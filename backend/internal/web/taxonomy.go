package web

import (
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/categories"
	"github.com/michaelputong/blog/backend/internal/posts"
)

// navCategory is a category as readers see it: its name in the page's
// language and how many published articles it has there.
type navCategory struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// publicBase is newBase plus the categories the page's menus link to.
// Categories without published articles in lang are left out.
func (h *pages) publicBase(c *fiber.Ctx, page, lang string) (base, error) {
	b := newBase(c, page, lang)
	nav, err := h.navCategories(c, lang)
	b.Categories = nav
	return b, err
}

func (h *pages) navCategories(c *fiber.Ctx, lang string) ([]navCategory, error) {
	cats, err := h.cfg.Categories.List(c.UserContext(), lang)
	if err != nil {
		return nil, err
	}
	counts, err := h.cfg.Store.CategoryCounts(c.UserContext(), lang)
	if err != nil {
		return nil, err
	}
	out := []navCategory{}
	for _, cat := range cats {
		if n := counts[cat.Slug]; n > 0 {
			out = append(out, navCategory{Slug: cat.Slug, Name: cat.Name(lang), Count: n})
		}
	}
	return out, nil
}

// homeGroup is one category section on the main page.
type homeGroup struct {
	Category navCategory  `json:"category"` // Slug "" is "Other" (no category)
	Posts    []posts.Post `json:"posts"`    // the newest few
}

const postsPerHomeGroup = 5

// groupByCategory splits posts (newest first) into sections in the order of
// nav, followed by articles without a category.
func groupByCategory(list []posts.Post, nav []navCategory) []homeGroup {
	listed := func(slug string) bool {
		return slices.ContainsFunc(nav, func(n navCategory) bool { return n.Slug == slug })
	}
	groups := []homeGroup{}
	for _, cat := range append(slices.Clone(nav), navCategory{Slug: ""}) {
		g := homeGroup{Category: cat, Posts: []posts.Post{}}
		g.Category.Count = 0
		for _, p := range list {
			// "Other" (slug "") collects posts whose category isn't listed.
			if p.Category == cat.Slug || (cat.Slug == "" && !listed(p.Category)) {
				g.Category.Count++
				if len(g.Posts) < postsPerHomeGroup {
					g.Posts = append(g.Posts, p)
				}
			}
		}
		if len(g.Posts) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}

// ---- Browse: search, category and tag pages -----------------------------

// searchPage lists articles matching any combination of text, tag and
// category. Scope says which URL was used, so the page can title itself
// ("Category: Security", "Tag: go" or "Search results").
type searchPage struct {
	base
	Scope    string           `json:"scope"` // "search", "category" or "tag"
	Query    string           `json:"query"`
	Tag      string           `json:"tag"`
	Category string           `json:"category"`
	Results  []posts.Post     `json:"results"`
	Tags     []posts.TagCount `json:"tags"` // every tag in use, for the filter
	Filtered bool             `json:"filtered"`
}

// GET /:lang/search?q=&tag=&category=
func (h *pages) search(c *fiber.Ctx) error {
	return h.browse(c, "search", c.Query("category"), c.Query("tag"))
}

// GET /:lang/categories/:slug (optionally ?q=&tag=)
func (h *pages) categoryPage(c *fiber.Ctx) error {
	return h.browse(c, "category", pathParam(c, "slug"), c.Query("tag"))
}

// GET /:lang/tags/:tag (optionally ?q=&category=)
func (h *pages) tagPage(c *fiber.Ctx) error {
	return h.browse(c, "tag", c.Query("category"), pathParam(c, "tag"))
}

// pathParam returns a decoded copy of a URL path parameter (tags like "c#"
// arrive percent-encoded; Fiber's strings must not outlive the request).
func pathParam(c *fiber.Ctx, name string) string {
	v, err := url.PathUnescape(c.Params(name))
	if err != nil {
		return ""
	}
	return strings.Clone(v)
}

func (h *pages) browse(c *fiber.Ctx, scope, category, tag string) error {
	lang := c.Params("lang")
	if !slices.Contains(h.cfg.Languages, lang) {
		return h.notFound(c)
	}
	query := strings.TrimSpace(strings.Clone(c.Query("q")))
	if r := []rune(query); len(r) > 200 {
		query = string(r[:200]) // cut on a character boundary, not mid-UTF-8
	}
	tag = posts.NormalizeTag(tag)
	category = strings.TrimSpace(strings.Clone(category))

	b, err := h.publicBase(c, "search", lang)
	if err != nil {
		return h.fail(c, err)
	}
	if category != "" {
		cat, err := h.cfg.Categories.Get(c.UserContext(), category)
		if errors.Is(err, categories.ErrNotFound) {
			if scope == "category" {
				return h.notFound(c)
			}
			category = "" // an unknown filter value is ignored on the search page
		} else if err != nil {
			return h.fail(c, err)
		} else if !slices.ContainsFunc(b.Categories, func(n navCategory) bool { return n.Slug == category }) {
			// No published articles in this language yet: name it anyway.
			b.Categories = append(b.Categories, navCategory{Slug: cat.Slug, Name: cat.Name(lang)})
		}
	}
	tags, err := h.cfg.Store.TagCounts(c.UserContext(), lang)
	if err != nil {
		return h.fail(c, err)
	}
	if scope == "tag" && !slices.ContainsFunc(tags, func(t posts.TagCount) bool { return t.Tag == tag }) {
		return h.notFound(c) // no published article uses this tag
	}

	page := searchPage{base: b, Scope: scope, Query: query, Tag: tag, Category: category, Tags: tags, Results: []posts.Post{}}
	page.Filtered = query != "" || tag != "" || category != ""
	if page.Filtered {
		if page.Results, err = h.cfg.Store.Find(c.UserContext(), posts.Filter{Lang: lang, Text: query, Tag: tag, Category: category}); err != nil {
			return h.fail(c, err)
		}
	}
	return h.render(c, fiber.StatusOK, lang, nil, page)
}
