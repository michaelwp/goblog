package web

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/posts"
)

// bulkResult reports what a list action did; it travels in the redirect's
// query string so the list can show it after the POST.
type bulkResult struct {
	Verb         string `json:"verb"` // "publish", "disable", "draft" or "delete"
	Articles     int    `json:"articles"`
	Translations int    `json:"translations"`
	Skipped      int    `json:"skipped"` // empty drafts that couldn't be published
}

const maxBulk = 500

var bulkVerbs = []string{"publish", "disable", "draft", "delete"}

// adminBulk applies one action to whole articles (every translation) from
// the article list. The form sends action=<verb> with the checked slugs, or
// action=<verb>:<slug> from a row's own menu.
func (h *pages) adminBulk(c *fiber.Ctx) error {
	verb, only, _ := strings.Cut(c.FormValue("action"), ":")
	back := "/admin"
	if show := posts.Status(c.FormValue("show")); show.Valid() {
		back += "?show=" + string(show)
	}
	if !slices.Contains(bulkVerbs, verb) {
		return c.Redirect(back, fiber.StatusSeeOther)
	}

	var slugs []string
	if only != "" {
		slugs = []string{strings.Clone(only)}
	} else {
		for _, s := range c.Request().PostArgs().PeekMulti("slug") {
			if !slices.Contains(slugs, string(s)) { // string(s) copies Fiber's buffer
				slugs = append(slugs, string(s))
			}
		}
	}
	if len(slugs) == 0 {
		return c.Redirect(withQuery(back, "notice=none-selected"), fiber.StatusSeeOther)
	}
	if len(slugs) > maxBulk {
		slugs = slugs[:maxBulk]
	}

	res := bulkResult{Verb: verb}
	ctx := c.UserContext()
	if verb == "delete" {
		for _, slug := range slugs {
			n, err := h.cfg.Store.DeleteAll(ctx, slug)
			if err != nil {
				return h.fail(c, err)
			}
			if n > 0 {
				res.Articles++
				res.Translations += n
			}
		}
	} else {
		status := map[string]posts.Status{"publish": posts.Published, "disable": posts.Disabled, "draft": posts.Draft}[verb]
		all, err := h.cfg.Store.All(ctx)
		if err != nil {
			return h.fail(c, err)
		}
		touched := map[string]bool{}
		for _, p := range all {
			if !slices.Contains(slugs, p.Slug) {
				continue
			}
			if status == posts.Published && strings.TrimSpace(p.Editing().Body) == "" {
				res.Skipped++ // publishing needs a body, same as in the editor
				continue
			}
			// Publishing applies pending edits; other status changes fold them in.
			if p.Status != status || p.Draft != nil {
				p.ApplyDraft()
				p.Status = status
				if err := h.cfg.Store.Update(ctx, p); err != nil {
					return h.fail(c, err)
				}
			}
			res.Translations++
			touched[p.Slug] = true
		}
		res.Articles = len(touched)
	}

	q := url.Values{
		"done": {verb}, "articles": {strconv.Itoa(res.Articles)},
		"translations": {strconv.Itoa(res.Translations)}, "skipped": {strconv.Itoa(res.Skipped)},
	}
	return c.Redirect(withQuery(back, q.Encode()), fiber.StatusSeeOther)
}

// bulkFromQuery reads a bulkResult back from the list page's query string.
func bulkFromQuery(c *fiber.Ctx) *bulkResult {
	verb := c.Query("done")
	if !slices.Contains(bulkVerbs, verb) {
		return nil
	}
	n := func(key string) int {
		v, err := strconv.Atoi(c.Query(key))
		if err != nil || v < 0 || v > maxBulk*10 {
			return 0
		}
		return v
	}
	return &bulkResult{Verb: verb, Articles: n("articles"), Translations: n("translations"), Skipped: n("skipped")}
}

func withQuery(path, query string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return fmt.Sprint(path, sep, query)
}
