package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"

	"github.com/michaelputong/blog/backend/internal/auth"
	"github.com/michaelputong/blog/backend/internal/media"
	"github.com/michaelputong/blog/backend/internal/posts"
	"github.com/michaelputong/blog/backend/internal/profile"
	"github.com/michaelputong/blog/backend/internal/ssr"
)

type Config struct {
	Store     posts.Repository
	Profiles  profile.Store
	Media     media.Store
	Renderer  *ssr.Renderer
	Languages []string // supported languages; the first is the default

	// Credentials holds the admin password hash and session key.
	Credentials auth.Store
	// BcryptCost for hashing admin passwords; 0 means auth.DefaultCost.
	BcryptCost int
	// Logf receives the admin setup code; nil means log.Printf.
	Logf func(format string, args ...any)
}

// Page payloads handed to the React renderer. They must match PageData in
// frontend/src/types.ts.
type base struct {
	Page  string `json:"page"`
	Lang  string `json:"lang"`
	Theme string `json:"theme"` // "auto", "light" or "dark"
	Year  int    `json:"year"`  // for the footer; from the server so SSR and hydration agree
}

type homePage struct {
	base
	Posts []posts.Post `json:"posts"`
}

type postPage struct {
	base
	Post               posts.Post `json:"post"`
	AvailableLanguages []string   `json:"availableLanguages"`
	Preview            bool       `json:"preview"`        // admin preview of an unpublished article
	PendingChanges     bool       `json:"pendingChanges"` // the preview shows unpublished edits to a live article
}

type searchPage struct {
	base
	Query   string       `json:"query"`
	Results []posts.Post `json:"results"`
}

type notFoundPage struct {
	base
}

// themeCookie holds the visitor's appearance choice. It is set in the browser
// by the Appearance menu and read here so the first paint uses the right theme.
const themeCookie = "theme"

func theme(c *fiber.Ctx) string {
	switch v := c.Cookies(themeCookie); v {
	case "light", "dark":
		return v
	default:
		return "auto"
	}
}

// bundle names the browser bundle that hydrates the page: admin pages get
// admin.js (with the editor), everything else the lighter app.js.
func (b base) bundle() string {
	if strings.HasPrefix(b.Page, "admin") {
		return "admin"
	}
	return "app"
}

func newBase(c *fiber.Ctx, page, lang string) base {
	return base{Page: page, Lang: lang, Theme: theme(c), Year: time.Now().Year()}
}

// Register mounts the embedded assets and the server-rendered pages. Call it
// after registering API routes: its catch-all renders the 404 page.
func Register(app *fiber.App, cfg Config) {
	assets := Assets()
	h := &pages{
		cfg:     cfg,
		version: assetVersion(assets),
		setup:   &setupCodes{now: time.Now, logf: cfg.Logf},
	}

	// Asset URLs carry ?v=<content hash>, so they can be cached indefinitely.
	app.Use("/assets", filesystem.New(filesystem.Config{
		Root:   http.FS(assets),
		MaxAge: int((365 * 24 * time.Hour).Seconds()),
	}))

	app.Get("/media/:file", h.media)
	h.registerAdmin(app) // before "/:lang", which would otherwise match "/admin"
	h.announceSetup()
	app.Get("/", h.root)
	app.Get("/:lang", h.home)
	app.Get("/:lang/search", h.search)
	app.Get("/:lang/about", h.about)
	app.Get("/:lang/posts/:slug", h.post)
	app.Use(h.notFound)
}

type pages struct {
	cfg     Config
	version string
	setup   *setupCodes
}

// announceSetup logs a setup code at startup when no admin exists yet, so
// the operator sees it without having to visit /admin/setup first.
func (h *pages) announceSetup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, ok, err := h.cfg.Credentials.Get(ctx); err == nil && !ok {
		h.setup.current()
	}
}

func (h *pages) root(c *fiber.Ctx) error {
	lang := negotiate(c.Get(fiber.HeaderAcceptLanguage), h.cfg.Languages)
	return c.Redirect("/"+lang, fiber.StatusFound)
}

func (h *pages) home(c *fiber.Ctx) error {
	lang := c.Params("lang")
	if !slices.Contains(h.cfg.Languages, lang) {
		return h.notFound(c)
	}
	list, err := h.cfg.Store.List(c.UserContext(), lang)
	if err != nil {
		return h.fail(c, err)
	}
	return h.render(c, fiber.StatusOK, lang, nil, homePage{base: newBase(c, "home", lang), Posts: list})
}

func (h *pages) search(c *fiber.Ctx) error {
	lang := c.Params("lang")
	if !slices.Contains(h.cfg.Languages, lang) {
		return h.notFound(c)
	}
	query := strings.TrimSpace(c.Query("q"))
	if r := []rune(query); len(r) > 200 {
		query = string(r[:200]) // cut on a character boundary, not mid-UTF-8
	}
	results := []posts.Post{}
	if query != "" {
		var err error
		if results, err = h.cfg.Store.Search(c.UserContext(), lang, query); err != nil {
			return h.fail(c, err)
		}
	}
	return h.render(c, fiber.StatusOK, lang, nil, searchPage{base: newBase(c, "search", lang), Query: query, Results: results})
}

func (h *pages) post(c *fiber.Ctx) error {
	lang, slug := c.Params("lang"), c.Params("slug")
	if !slices.Contains(h.cfg.Languages, lang) {
		return h.notFound(c)
	}
	p, err := h.cfg.Store.Get(c.UserContext(), slug, lang)
	if errors.Is(err, posts.ErrNotFound) || (err == nil && p.Status != posts.Published) {
		return h.notFound(c) // drafts and disabled articles don't exist publicly
	}
	if err != nil {
		return h.fail(c, err)
	}
	return h.renderPost(c, p, false)
}

// renderPost renders an article page. preview marks an admin preview of a
// draft or disabled article, which search engines must not index.
func (h *pages) renderPost(c *fiber.Ctx, p posts.Post, preview bool) error {
	pending := preview && p.Draft != nil
	if preview {
		p = p.Editing() // previews show pending changes; the public page never does
	} else {
		p.Draft = nil // never send unpublished edits to readers
	}
	langs, err := h.cfg.Store.Languages(c.UserContext(), p.Slug)
	if err != nil {
		return h.fail(c, err)
	}
	var alternates []alternate
	if !preview {
		for _, l := range langs {
			alternates = append(alternates, alternate{Lang: l, Href: "/" + l + "/posts/" + p.Slug})
		}
	}
	return h.render(c, fiber.StatusOK, p.Lang, alternates,
		postPage{base: newBase(c, "post", p.Lang), Post: p, AvailableLanguages: langs, Preview: preview, PendingChanges: pending})
}

func (h *pages) notFound(c *fiber.Ctx) error {
	// Use the language from the URL prefix when it's a supported one.
	lang := h.cfg.Languages[0]
	if first, _, _ := strings.Cut(strings.TrimPrefix(c.Path(), "/"), "/"); slices.Contains(h.cfg.Languages, first) {
		lang = first
	}
	return h.render(c, fiber.StatusNotFound, lang, nil, notFoundPage{base: newBase(c, "notFound", lang)})
}

func (h *pages) fail(c *fiber.Ctx, err error) error {
	log.Printf("%s %s: %v", c.Method(), c.Path(), err)
	return c.Status(fiber.StatusInternalServerError).SendString("Something went wrong. Please try again later.")
}

type alternate struct{ Lang, Href string }

func (h *pages) render(c *fiber.Ctx, status int, lang string, alternates []alternate, page any) error {
	res, err := h.cfg.Renderer.Render(page)
	if err != nil {
		return h.fail(c, err)
	}
	var buf bytes.Buffer
	err = shell.Execute(&buf, map[string]any{
		"Lang":        lang,
		"Theme":       theme(c),
		"Title":       res.Title,
		"Description": res.Description,
		"Alternates":  alternates,
		"Version":     h.version,
		"Bundle":      bundleOf(page),
		"HTML":        template.HTML(res.HTML), // produced by React, which escapes content
		"Page":        page,                    // JSON-encoded by html/template in the <script>
	})
	if err != nil {
		return h.fail(c, err)
	}
	c.Type("html", "utf-8")
	return c.Status(status).Send(buf.Bytes())
}

func bundleOf(page any) string {
	if p, ok := page.(interface{ bundle() string }); ok {
		return p.bundle()
	}
	return "app"
}

var shell = template.Must(template.New("shell").Parse(`<!doctype html>
<html lang="{{.Lang}}"{{if ne .Theme "auto"}} data-theme="{{.Theme}}"{{end}}>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<meta name="description" content="{{.Description}}">
{{range .Alternates}}<link rel="alternate" hreflang="{{.Lang}}" href="{{.Href}}">
{{end}}<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 48 48'%3E%3Ccircle cx='24' cy='24' r='22' fill='%23fff' stroke='%23202122' stroke-width='3'/%3E%3Ctext x='24' y='33' text-anchor='middle' font-family='Georgia,serif' font-size='26' font-weight='700' fill='%23202122'%3EG%3C/text%3E%3C/svg%3E">
<link rel="stylesheet" href="/assets/{{.Bundle}}.css?v={{.Version}}">
</head>
<body>
<div id="root">{{.HTML}}</div>
<script>window.__PAGE__ = {{.Page}};</script>
<script src="/assets/{{.Bundle}}.js?v={{.Version}}" defer></script>
</body>
</html>
`))

// assetVersion hashes the asset files so URLs change whenever a build does.
func assetVersion(assets fs.FS) string {
	sum := sha256.New()
	_ = fs.WalkDir(assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		sum.Write([]byte(path))
		sum.Write(b)
		return nil
	})
	return hex.EncodeToString(sum.Sum(nil))[:12]
}

// negotiate picks the best supported language from an Accept-Language
// header, matching on the primary subtag ("id-ID" matches "id").
func negotiate(header string, supported []string) string {
	best, bestQ := supported[0], 0.0
	for _, part := range strings.Split(header, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				q = f
			}
		}
		base, _, _ := strings.Cut(strings.ToLower(tag), "-")
		if q > bestQ && slices.Contains(supported, base) {
			best, bestQ = base, q
		}
	}
	return best
}
