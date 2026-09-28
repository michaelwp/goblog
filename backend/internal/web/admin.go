package web

import (
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/michaelputong/blog/backend/internal/auth"
	"github.com/michaelputong/blog/backend/internal/posts"
)

const sessionCookie = "admin_session"

// Admin page payloads. They must match the admin variants of PageData in
// frontend/src/types.ts.
type adminLoginPage struct {
	base
	Error string `json:"error"`
}

type adminListPage struct {
	base
	Posts     []posts.Post `json:"posts"` // bodies omitted
	Languages []string     `json:"languages"`
	Notice    string       `json:"notice"` // "deleted" or "deleted-all"
	Filter    string       `json:"filter"` // "", or a status to show only articles with it
	Bulk      *bulkResult  `json:"bulk"`   // result of the last list action, if any
}

type adminEditPage struct {
	base
	Mode              string            `json:"mode"`   // "new" or "edit"
	Status            string            `json:"status"` // current status when editing
	Notice            string            `json:"notice"` // "draft", "published", "saved" or "disabled"
	Form              adminForm         `json:"form"`
	Errors            map[string]string `json:"errors"`
	Languages         []string          `json:"languages"`
	OtherTranslations []string          `json:"otherTranslations"`
	// Status of each existing translation of this article, by language, for
	// the editor's language switcher.
	Translations map[string]string `json:"translations"`
	DraftSavedAt string            `json:"draftSavedAt"` // set when a published article has unpublished changes
}

type adminForm struct {
	Slug    string `json:"slug"`
	Lang    string `json:"lang"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
	Date    string `json:"date"` // YYYY-MM-DD
}

const dateLayout = "2006-01-02"

// registerAdmin mounts /admin. Until an admin password has been created in
// the database, every admin page leads to the one-time setup form.
func (h *pages) registerAdmin(app *fiber.App) {
	admin := app.Group("/admin", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		c.Set("X-Robots-Tag", "noindex, nofollow")
		c.Set("X-Frame-Options", "DENY")
		if c.Method() == fiber.MethodPost && !sameOrigin(c) {
			return c.Status(fiber.StatusForbidden).SendString("Cross-site request rejected.")
		}
		return c.Next()
	})

	attempts := func(render func(*fiber.Ctx, int, string) error) fiber.Handler {
		return limiter.New(limiter.Config{
			Max:                    5, // failed attempts per minute per IP
			Expiration:             time.Minute,
			SkipSuccessfulRequests: true,
			LimitReached: func(c *fiber.Ctx) error {
				return render(c, fiber.StatusTooManyRequests, "Too many attempts. Wait a minute and try again.")
			},
		})
	}

	admin.Get("/setup", h.adminSetupForm)
	admin.Post("/setup", attempts(h.renderSetup), h.adminSetup)
	admin.Get("/login", h.adminLoginForm)
	admin.Post("/login", attempts(h.renderLogin), h.adminLogin)
	admin.Post("/logout", h.adminLogout)

	admin.Use(h.requireAdmin)
	admin.Get("/", h.adminList)
	admin.Get("/new", h.adminNewForm)
	admin.Post("/new", h.adminCreate)
	admin.Get("/posts/:slug/:lang", h.adminEditForm)
	admin.Post("/posts/:slug/:lang", h.adminUpdate)
	admin.Post("/posts/:slug/:lang/delete", h.adminDelete)
	admin.Get("/posts/:slug/:lang/preview", h.adminPreview)
	admin.Post("/posts/:slug/:lang/autosave", h.adminAutosave)
	admin.Post("/posts/:slug/:lang/discard", h.adminDiscard)
	admin.Post("/new/autosave", h.adminAutosaveNew)
	admin.Post("/articles/:slug/delete", h.adminDeleteAll)
	admin.Post("/bulk", h.adminBulk)
	admin.Get("/profile", h.adminProfileForm)
	admin.Post("/profile", h.adminProfileSave)
	admin.Post("/media", h.adminUpload)
	admin.Get("/password", h.adminPasswordForm)
	admin.Post("/password", h.adminPasswordSave)
	admin.Use(h.notFound)
}

// sameOrigin rejects cross-site form posts. The SameSite=Strict cookie
// already blocks them; this is defense in depth for older browsers.
func sameOrigin(c *fiber.Ctx) bool {
	origin := c.Get(fiber.HeaderOrigin)
	if origin == "" || origin == "null" {
		return origin == "" // no Origin header: same-origin navigation in older browsers
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == string(c.Request().Host())
}

func (h *pages) credential(c *fiber.Ctx) (auth.Credential, bool, error) {
	return h.cfg.Credentials.Get(c.UserContext())
}

// requireAdmin lets a request through only with a valid session, and stores
// the credential in Locals for handlers that need it.
func (h *pages) requireAdmin(c *fiber.Ctx) error {
	cred, ok, err := h.credential(c)
	if err != nil {
		return h.fail(c, err)
	}
	if !ok {
		return c.Redirect("/admin/setup", fiber.StatusSeeOther)
	}
	if !validToken(c.Cookies(sessionCookie), cred, time.Now()) {
		return c.Redirect("/admin/login", fiber.StatusSeeOther)
	}
	c.Locals("credential", cred)
	return c.Next()
}

func (h *pages) adminBase(c *fiber.Ctx, page string) base {
	return newBase(c, page, h.cfg.Languages[0])
}

func (h *pages) startSession(c *fiber.Ctx, cred auth.Credential) {
	token, expires := issueToken(cred, time.Now())
	c.Cookie(&fiber.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/admin",
		Expires:  expires,
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: fiber.CookieSameSiteStrictMode,
	})
}

// validatePassword applies the rules shared by setup and password changes.
func validatePassword(password, confirm string) string {
	if msg := auth.DescribePasswordProblems(auth.PasswordProblems(password)); msg != "" {
		return msg
	}
	if password != confirm {
		return "The passwords don't match."
	}
	return ""
}

// ---- First-time setup -------------------------------------------------

func (h *pages) adminSetupForm(c *fiber.Ctx) error {
	if _, ok, err := h.credential(c); err != nil {
		return h.fail(c, err)
	} else if ok {
		return c.Redirect("/admin/login", fiber.StatusSeeOther)
	}
	h.setup.current() // make sure a code exists and has been logged
	return h.renderSetup(c, fiber.StatusOK, "")
}

func (h *pages) renderSetup(c *fiber.Ctx, status int, msg string) error {
	return h.render(c, status, h.cfg.Languages[0], nil, adminLoginPage{base: h.adminBase(c, "adminSetup"), Error: msg})
}

func (h *pages) adminSetup(c *fiber.Ctx) error {
	if _, ok, err := h.credential(c); err != nil {
		return h.fail(c, err)
	} else if ok {
		return c.Redirect("/admin/login", fiber.StatusSeeOther)
	}
	if !h.setup.matches(c.FormValue("code")) {
		return h.renderSetup(c, fiber.StatusUnauthorized, "That setup code isn't right. Find the current one in the server log.")
	}
	password := strings.Clone(c.FormValue("password"))
	if msg := validatePassword(password, c.FormValue("confirm")); msg != "" {
		return h.renderSetup(c, fiber.StatusUnprocessableEntity, msg)
	}
	cred, err := auth.NewCredential(password, h.bcryptCost())
	if err != nil {
		return h.fail(c, err)
	}
	if err := h.cfg.Credentials.Create(c.UserContext(), cred); errors.Is(err, auth.ErrExists) {
		return c.Redirect("/admin/login", fiber.StatusSeeOther) // someone else finished setup first
	} else if err != nil {
		return h.fail(c, err)
	}
	h.setup.clear()
	h.startSession(c, cred)
	return c.Redirect("/admin", fiber.StatusSeeOther)
}

// ---- Login ------------------------------------------------------------

func (h *pages) adminLoginForm(c *fiber.Ctx) error {
	cred, ok, err := h.credential(c)
	if err != nil {
		return h.fail(c, err)
	}
	if !ok {
		return c.Redirect("/admin/setup", fiber.StatusSeeOther)
	}
	if validToken(c.Cookies(sessionCookie), cred, time.Now()) {
		return c.Redirect("/admin", fiber.StatusSeeOther)
	}
	return h.renderLogin(c, fiber.StatusOK, "")
}

func (h *pages) renderLogin(c *fiber.Ctx, status int, msg string) error {
	return h.render(c, status, h.cfg.Languages[0], nil, adminLoginPage{base: h.adminBase(c, "adminLogin"), Error: msg})
}

func (h *pages) adminLogin(c *fiber.Ctx) error {
	cred, ok, err := h.credential(c)
	if err != nil {
		return h.fail(c, err)
	}
	if !ok {
		return c.Redirect("/admin/setup", fiber.StatusSeeOther)
	}
	if !cred.PasswordMatches(c.FormValue("password")) {
		return h.renderLogin(c, fiber.StatusUnauthorized, "That password isn't right.")
	}
	h.startSession(c, cred)
	return c.Redirect("/admin", fiber.StatusSeeOther)
}

func (h *pages) adminLogout(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     sessionCookie,
		Path:     "/admin",
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteStrictMode,
	})
	return c.Redirect("/admin/login", fiber.StatusSeeOther)
}

// ---- Change password --------------------------------------------------

type adminPasswordPage struct {
	base
	Errors map[string]string `json:"errors"`
	Notice string            `json:"notice"`
}

func (h *pages) adminPasswordForm(c *fiber.Ctx) error {
	notice := ""
	if c.Query("notice") == "saved" {
		notice = "saved"
	}
	return h.renderPassword(c, fiber.StatusOK, nil, notice)
}

func (h *pages) renderPassword(c *fiber.Ctx, status int, errs map[string]string, notice string) error {
	if errs == nil {
		errs = map[string]string{}
	}
	return h.render(c, status, h.cfg.Languages[0], nil, adminPasswordPage{base: h.adminBase(c, "adminPassword"), Errors: errs, Notice: notice})
}

func (h *pages) adminPasswordSave(c *fiber.Ctx) error {
	cred := c.Locals("credential").(auth.Credential)
	if !cred.PasswordMatches(c.FormValue("current")) {
		return h.renderPassword(c, fiber.StatusUnprocessableEntity, map[string]string{"current": "That isn't your current password."}, "")
	}
	password := strings.Clone(c.FormValue("password"))
	if msg := validatePassword(password, c.FormValue("confirm")); msg != "" {
		return h.renderPassword(c, fiber.StatusUnprocessableEntity, map[string]string{"password": msg}, "")
	}
	updated, err := cred.WithPassword(password, h.bcryptCost())
	if err != nil {
		return h.fail(c, err)
	}
	if err := h.cfg.Credentials.Update(c.UserContext(), updated); err != nil {
		return h.fail(c, err)
	}
	h.startSession(c, updated) // other sessions were signed out by the new epoch
	return c.Redirect("/admin/password?notice=saved", fiber.StatusSeeOther)
}

func (h *pages) bcryptCost() int {
	if h.cfg.BcryptCost > 0 {
		return h.cfg.BcryptCost
	}
	return auth.DefaultCost
}

// ---- List -------------------------------------------------------------

func (h *pages) adminList(c *fiber.Ctx) error {
	all, err := h.cfg.Store.All(c.UserContext())
	if err != nil {
		return h.fail(c, err)
	}
	for i := range all {
		all[i].Body = "" // not shown; keeps the page payload small
		if d := all[i].Draft; d != nil {
			copied := *d // don't mutate the store's revision
			copied.Body = ""
			all[i].Draft = &copied
		}
	}
	notice := c.Query("notice")
	if !slices.Contains([]string{"deleted", "deleted-all", "none-selected"}, notice) {
		notice = ""
	}
	filter := posts.Status(c.Query("show"))
	if !filter.Valid() {
		filter = ""
	}
	return h.render(c, fiber.StatusOK, h.cfg.Languages[0], nil, adminListPage{
		base: h.adminBase(c, "adminList"), Posts: all, Languages: h.cfg.Languages, Notice: notice, Filter: string(filter),
		Bulk: bulkFromQuery(c),
	})
}

// ---- Status actions ---------------------------------------------------

// The editor's buttons submit action=draft|publish|disable|save. "save"
// (and a missing action, e.g. pressing Enter) keeps the current status; a new
// article without an explicit action is saved as a draft.
func nextStatus(action string, current posts.Status) posts.Status {
	switch action {
	case "draft":
		return posts.Draft
	case "publish":
		return posts.Published
	case "disable":
		return posts.Disabled
	}
	if current == "" {
		return posts.Draft
	}
	return current
}

// Notices shown on the edit page after an action, keyed by the new status.
func editNotice(action string, status posts.Status) string {
	switch {
	case action == "publish":
		return "published"
	case action == "disable":
		return "disabled"
	case action == "draft":
		return "draft"
	case status == posts.Draft:
		return "draft"
	}
	return "saved"
}

func editURL(p posts.Post, notice string) string {
	return "/admin/posts/" + p.Slug + "/" + p.Lang + "?notice=" + notice
}

// ---- Create -----------------------------------------------------------

func (h *pages) adminNewForm(c *fiber.Ctx) error {
	// ?slug=&lang=&from= pre-fills a translation of an existing article.
	form := adminForm{Slug: c.Query("slug"), Lang: c.Query("lang"), Date: time.Now().UTC().Format(dateLayout)}
	if !slices.Contains(h.cfg.Languages, form.Lang) {
		form.Lang = h.cfg.Languages[0]
	}
	if from := c.Query("from"); form.Slug != "" && from != "" {
		if src, err := h.cfg.Store.Get(c.UserContext(), form.Slug, from); err == nil {
			form.Date = src.PublishedAt.UTC().Format(dateLayout)
		}
	}
	return h.renderEdit(c, fiber.StatusOK, editState{mode: "new", form: form})
}

func (h *pages) adminCreate(c *fiber.Ctx) error {
	form := readForm(c)
	action := c.FormValue("action")
	status := nextStatus(action, "")
	p, errs := h.validate(form, "new", status)
	if len(errs) == 0 {
		err := h.cfg.Store.Create(c.UserContext(), p)
		if errors.Is(err, posts.ErrExists) {
			errs = map[string]string{"slug": "An article with this slug already exists in this language."}
		} else if err != nil {
			return h.fail(c, err)
		}
	}
	if len(errs) > 0 {
		return h.renderEdit(c, fiber.StatusUnprocessableEntity, editState{mode: "new", form: form, errs: errs})
	}
	return c.Redirect(editURL(p, editNotice(action, status)), fiber.StatusSeeOther)
}

// ---- Edit -------------------------------------------------------------

func (h *pages) adminEditForm(c *fiber.Ctx) error {
	p, err := h.cfg.Store.Get(c.UserContext(), c.Params("slug"), c.Params("lang"))
	if errors.Is(err, posts.ErrNotFound) {
		return h.notFound(c)
	}
	if err != nil {
		return h.fail(c, err)
	}
	notice := c.Query("notice")
	if !slices.Contains([]string{"draft", "published", "saved", "disabled", "revised", "discarded"}, notice) {
		notice = ""
	}
	e := p.Editing() // the pending draft of a published article, if any
	st := editState{
		mode: "edit", status: p.Status, notice: notice,
		form: adminForm{
			Slug: e.Slug, Lang: e.Lang, Title: e.Title, Summary: e.Summary, Body: e.Body,
			Date: e.PublishedAt.UTC().Format(dateLayout),
		},
	}
	if p.Draft != nil {
		st.draftSavedAt = p.Draft.SavedAt.UTC().Format(time.RFC3339)
	}
	return h.renderEdit(c, fiber.StatusOK, st)
}

// saveEdit applies an edit form to an existing article for action, which is
// one of: "" or "save" (keep the status), "revise" (Save draft), "publish",
// "disable" or "draft" (move to drafts).
//
// Edits to a Published article that aren't "publish" never touch the live
// text: they're stored as its pending Draft, which readers don't see.
// Publishing applies the draft; other status changes fold it in first.
func (h *pages) saveEdit(existing posts.Post, form adminForm, action string) (posts.Post, map[string]string) {
	status := nextStatus(action, existing.Status)
	revise := existing.Status == posts.Published && status == posts.Published && action != "publish"

	check := status
	if revise {
		check = posts.Draft // a draft copy may be unfinished
	}
	p, errs := h.validate(form, "edit", check)
	if len(errs) > 0 {
		return posts.Post{}, errs
	}
	// Keep the original time of day when the date wasn't changed.
	if prev := existing.Editing().PublishedAt; prev.UTC().Format(dateLayout) == form.Date {
		p.PublishedAt = prev
	}
	if !revise {
		p.Status = status
		return p, nil // p.Draft is nil: any pending draft is replaced by the form
	}
	existing.Draft = &posts.Revision{
		Title: p.Title, Summary: p.Summary, Body: p.Body, PublishedAt: p.PublishedAt, SavedAt: time.Now().UTC(),
	}
	return existing, nil
}

func (h *pages) adminUpdate(c *fiber.Ctx) error {
	existing, err := h.cfg.Store.Get(c.UserContext(), c.Params("slug"), c.Params("lang"))
	if errors.Is(err, posts.ErrNotFound) {
		return h.notFound(c)
	}
	if err != nil {
		return h.fail(c, err)
	}

	form := readForm(c)
	form.Slug, form.Lang = existing.Slug, existing.Lang // identity comes from the URL, not the form
	action := c.FormValue("action")
	p, errs := h.saveEdit(existing, form, action)
	if len(errs) > 0 {
		st := editState{mode: "edit", status: existing.Status, form: form, errs: errs}
		if existing.Draft != nil {
			st.draftSavedAt = existing.Draft.SavedAt.UTC().Format(time.RFC3339)
		}
		return h.renderEdit(c, fiber.StatusUnprocessableEntity, st)
	}
	if err := h.cfg.Store.Update(c.UserContext(), p); err != nil {
		return h.fail(c, err)
	}
	notice := editNotice(action, p.Status)
	if p.Draft != nil {
		notice = "revised"
	}
	return c.Redirect(editURL(p, notice), fiber.StatusSeeOther)
}

// adminDiscard drops the pending draft of a published article.
func (h *pages) adminDiscard(c *fiber.Ctx) error {
	p, err := h.cfg.Store.Get(c.UserContext(), c.Params("slug"), c.Params("lang"))
	if errors.Is(err, posts.ErrNotFound) {
		return h.notFound(c)
	}
	if err != nil {
		return h.fail(c, err)
	}
	p.Draft = nil
	if err := h.cfg.Store.Update(c.UserContext(), p); err != nil {
		return h.fail(c, err)
	}
	return c.Redirect(editURL(p, "discarded"), fiber.StatusSeeOther)
}

// ---- Autosave ---------------------------------------------------------

// The editor posts its form here about once a minute while there are unsaved
// changes. Autosave never publishes or changes a status: new articles are
// created as drafts, drafts are updated, and published articles get their
// pending draft updated. It answers JSON: {"savedAt": "..."} (plus
// "editUrl" when it created the article) or {"error": "..."}.

func autosaveError(c *fiber.Ctx, status int, errs map[string]string) error {
	msg := "Autosave paused: fix the highlighted fields."
	for _, key := range []string{"slug", "lang", "title", "date", "summary", "body"} {
		if m, ok := errs[key]; ok {
			msg = "Autosave paused: " + m
			break
		}
	}
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

func (h *pages) adminAutosaveNew(c *fiber.Ctx) error {
	form := readForm(c)
	p, errs := h.validate(form, "new", posts.Draft)
	if len(errs) > 0 {
		return autosaveError(c, fiber.StatusUnprocessableEntity, errs)
	}
	err := h.cfg.Store.Create(c.UserContext(), p)
	if errors.Is(err, posts.ErrExists) {
		return autosaveError(c, fiber.StatusConflict, map[string]string{"slug": "an article with this slug already exists in this language."})
	}
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{
		"savedAt": time.Now().UTC().Format(time.RFC3339),
		"editUrl": "/admin/posts/" + p.Slug + "/" + p.Lang,
	})
}

func (h *pages) adminAutosave(c *fiber.Ctx) error {
	existing, err := h.cfg.Store.Get(c.UserContext(), c.Params("slug"), c.Params("lang"))
	if errors.Is(err, posts.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "This article no longer exists."})
	}
	if err != nil {
		return h.fail(c, err)
	}
	form := readForm(c)
	form.Slug, form.Lang = existing.Slug, existing.Lang
	p, errs := h.saveEdit(existing, form, "save")
	if len(errs) > 0 {
		return autosaveError(c, fiber.StatusUnprocessableEntity, errs)
	}
	if err := h.cfg.Store.Update(c.UserContext(), p); err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"savedAt": time.Now().UTC().Format(time.RFC3339)})
}

func (h *pages) adminPreview(c *fiber.Ctx) error {
	p, err := h.cfg.Store.Get(c.UserContext(), c.Params("slug"), c.Params("lang"))
	if errors.Is(err, posts.ErrNotFound) {
		return h.notFound(c)
	}
	if err != nil {
		return h.fail(c, err)
	}
	return h.renderPost(c, p, true) // renderPost shows the pending draft in previews
}

// ---- Delete -----------------------------------------------------------

func (h *pages) adminDelete(c *fiber.Ctx) error {
	err := h.cfg.Store.Delete(c.UserContext(), c.Params("slug"), c.Params("lang"))
	if err != nil && !errors.Is(err, posts.ErrNotFound) {
		return h.fail(c, err)
	}
	return c.Redirect("/admin?notice=deleted", fiber.StatusSeeOther)
}

func (h *pages) adminDeleteAll(c *fiber.Ctx) error {
	if _, err := h.cfg.Store.DeleteAll(c.UserContext(), c.Params("slug")); err != nil {
		return h.fail(c, err)
	}
	return c.Redirect("/admin?notice=deleted-all", fiber.StatusSeeOther)
}

type editState struct {
	mode         string // "new" or "edit"
	status       posts.Status
	draftSavedAt string // when a published article's pending draft was saved
	notice       string
	form         adminForm
	errs         map[string]string
}

func (h *pages) renderEdit(c *fiber.Ctx, code int, st editState) error {
	if st.errs == nil {
		st.errs = map[string]string{}
	}
	// Other translations of this article, for the "delete all" option and
	// the language switcher. A new translation (?slug=) has siblings too.
	var others []string
	translations := map[string]string{}
	if st.form.Slug != "" {
		all, err := h.cfg.Store.All(c.UserContext())
		if err != nil {
			return h.fail(c, err)
		}
		for _, p := range all {
			if p.Slug != st.form.Slug {
				continue
			}
			status := string(p.Status)
			if p.Status == posts.Published && p.Draft != nil {
				status = "edited" // published, with unpublished changes
			}
			translations[p.Lang] = status
			if p.Lang != st.form.Lang {
				others = append(others, p.Lang)
			}
		}
	}
	if others == nil {
		others = []string{}
	}
	return h.render(c, code, h.cfg.Languages[0], nil, adminEditPage{
		base: h.adminBase(c, "adminEdit"), Mode: st.mode, Status: string(st.status), Notice: st.notice,
		DraftSavedAt: st.draftSavedAt, Translations: translations,
		Form: st.form, Errors: st.errs, Languages: h.cfg.Languages, OtherTranslations: others,
	})
}

// ---- Validation -------------------------------------------------------

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func readForm(c *fiber.Ctx) adminForm {
	// Fiber's FormValue strings point into a buffer that is reused after the
	// request, so copy them before they can end up stored anywhere.
	v := func(key string) string { return strings.TrimSpace(strings.Clone(c.FormValue(key))) }
	return adminForm{
		Slug:    strings.ToLower(v("slug")),
		Lang:    v("lang"),
		Title:   v("title"),
		Summary: v("summary"),
		// Normalize Windows line endings from textareas so "\n\n" splits paragraphs.
		Body: strings.ReplaceAll(v("body"), "\r\n", "\n"),
		Date: v("date"),
	}
}

// validate checks the form for saving with status. Drafts may be unfinished,
// so only a published article needs a body.
func (h *pages) validate(f adminForm, mode string, status posts.Status) (posts.Post, map[string]string) {
	errs := map[string]string{}
	if mode == "new" {
		switch {
		case f.Slug == "":
			errs["slug"] = "Enter a slug."
		case len(f.Slug) > 100 || !slugPattern.MatchString(f.Slug):
			errs["slug"] = "Use lowercase letters, numbers and single hyphens, up to 100 characters (e.g. my-first-post)."
		}
		if !slices.Contains(h.cfg.Languages, f.Lang) {
			errs["lang"] = "Choose a language."
		}
	}
	switch n := utf8.RuneCountInString(f.Title); {
	case n == 0:
		errs["title"] = "Enter a title."
	case n > 200:
		errs["title"] = "Keep the title under 200 characters."
	}
	if utf8.RuneCountInString(f.Summary) > 300 {
		errs["summary"] = "Keep the summary under 300 characters."
	}
	if f.Body == "" && status == posts.Published {
		errs["body"] = "Write the article body before publishing (or save it as a draft)."
	}
	date, err := time.Parse(dateLayout, f.Date)
	if err != nil {
		errs["date"] = "Enter a valid date."
	}
	return posts.Post{
		Slug: f.Slug, Lang: f.Lang, Title: f.Title, Summary: f.Summary, Body: f.Body, PublishedAt: date, Status: status,
	}, errs
}
