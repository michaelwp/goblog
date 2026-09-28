package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/posts"
)

// withCategories creates two categories through the admin form.
func withCategories(t *testing.T, app *fiber.App, session *http.Cookie) {
	t.Helper()
	for _, form := range []url.Values{
		{"name_en": {"Web Development"}, "name_id": {"Pengembangan Web"}}, // slug from the English name
		{"name_en": {"Security"}, "name_id": {"Keamanan"}, "slug": {"security"}},
	} {
		res := send(t, app, "POST", "/admin/categories", form, session)
		if res.location != "/admin/categories?notice=created" {
			t.Fatalf("create category %v: %d → %q\n%s", form, res.code, res.location, res.body)
		}
	}
}

func TestCategoryAdmin(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	withCategories(t, app, session)

	page := send(t, app, "GET", "/admin/categories", nil, session).body
	if !strings.Contains(page, `"slug":"web-development"`) || !strings.Contains(page, `"slug":"security"`) {
		t.Fatal("categories page doesn't list both categories")
	}

	// Unique: slug and names (ignoring case) in each language.
	for want, form := range map[string]url.Values{
		"already exists":                 {"name_en": {"Other"}, "name_id": {"Lain"}, "slug": {"security"}},
		"“security” is already used":     {"name_en": {"security"}, "name_id": {"Baru"}}, // the name clash is reported, not the derived slug
		"“KEAMANAN” is already used":     {"name_en": {"Safety"}, "name_id": {"KEAMANAN"}},
		"Enter a name.":                  {"name_en": {"Only English"}},
		"Use lowercase letters, numbers": {"name_en": {"X"}, "name_id": {"Y"}, "slug": {"Bad Slug"}},
	} {
		res := send(t, app, "POST", "/admin/categories", form, session)
		if res.code != 422 || !strings.Contains(res.body, want) {
			t.Errorf("%v: status %d, want 422 containing %q", form, res.code, want)
		}
	}

	// Rename: allowed, but not onto another category's name.
	res := send(t, app, "POST", "/admin/categories/security", url.Values{"name_en": {"Cybersecurity"}, "name_id": {"Keamanan Siber"}}, session)
	if res.location != "/admin/categories?notice=renamed" {
		t.Errorf("rename: %d → %q", res.code, res.location)
	}
	res = send(t, app, "POST", "/admin/categories/security", url.Values{"name_en": {"web development"}, "name_id": {"X"}}, session)
	if res.code != 422 || !strings.Contains(res.body, "already used") {
		t.Errorf("rename onto a taken name: status %d", res.code)
	}

	// Signed out: nothing changes.
	send(t, app, "POST", "/admin/categories", url.Values{"name_en": {"Spam"}, "name_id": {"Spam"}})
	if strings.Contains(send(t, app, "GET", "/admin/categories", nil, session).body, `"slug":"spam"`) {
		t.Error("signed-out request created a category")
	}
}

func TestArticleCategoryAndTags(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	withCategories(t, app, session)
	ctx := t.Context()

	// Saving one translation sets category and tags on all of them.
	res := send(t, app, "POST", "/admin/posts/hello-world/en", url.Values{
		"title": {"Hello, world"}, "body": {"Hi."}, "date": {"2026-09-20"}, "action": {"revise"},
		"category": {"security"}, "tags": {"Go, Node JS, go, #Web"},
	}, session)
	if res.code != 303 {
		t.Fatalf("save: %d\n%s", res.code, res.body)
	}
	for _, lang := range []string{"en", "id"} {
		p, _ := store.Get(ctx, "hello-world", lang)
		if p.Category != "security" || !slices.Equal(p.Tags, []string{"go", "node-js", "web"}) {
			t.Errorf("hello-world/%s: category %q tags %v", lang, p.Category, p.Tags)
		}
	}
	// Category and tags apply right away, even though the text went to a draft copy.
	if p, _ := store.Get(ctx, "hello-world", "en"); p.Draft == nil || p.Category != "security" {
		t.Errorf("draft copy %v, live category %q", p.Draft != nil, p.Category)
	}

	// Invalid input is refused.
	for want, form := range map[string]url.Values{
		"Choose a category from the list.": {"category": {"nope"}},
		"Tags can use":                     {"tags": {"bad/tag"}},
	} {
		form.Set("title", "x")
		form.Set("body", "y")
		form.Set("date", "2026-09-25")
		if res := send(t, app, "POST", "/admin/posts/why-ssr/en", form, session); res.code != 422 || !strings.Contains(res.body, want) {
			t.Errorf("%v: status %d, want 422 with %q", form, res.code, want)
		}
	}

	// A new translation starts with the article's category and tags.
	send(t, app, "POST", "/admin/posts/why-ssr/en", url.Values{"title": {"Why server-side rendering"}, "body": {"b"}, "date": {"2026-09-25"}, "action": {"publish"}, "category": {"web-development"}, "tags": {"seo"}}, session)
	newPage := send(t, app, "GET", "/admin/new?slug=why-ssr&lang=id&from=en", nil, session).body
	if !strings.Contains(newPage, `"category":"web-development"`) || !strings.Contains(newPage, `"tags":"seo"`) {
		t.Error("new translation isn't prefilled with category and tags")
	}

	// Deleting a category keeps its articles, uncategorized.
	send(t, app, "POST", "/admin/categories/security/delete", url.Values{}, session)
	if p, _ := store.Get(ctx, "hello-world", "id"); p.Category != "" {
		t.Errorf("after deleting its category, article category = %q", p.Category)
	}
}

func TestPublicCategoryAndTagPages(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	withCategories(t, app, session)
	ctx := t.Context()
	_ = store.SetMeta(ctx, "hello-world", "web-development", []string{"go", "welcome"})
	_ = store.SetMeta(ctx, "why-ssr", "security", []string{"go", "c#"})

	// Main page: grouped by category.
	home := send(t, app, "GET", "/en", nil).body
	for _, want := range []string{`"groups":[`, `href="/en/categories/security"`, "Browse by category", `"name":"Web Development","count":1`} {
		if !strings.Contains(home, want) {
			t.Errorf("home missing %s", want)
		}
	}

	// Category page (Indonesian name on the Indonesian site).
	res := send(t, app, "GET", "/id/categories/web-development", nil)
	if res.code != 200 || !strings.Contains(res.body, "<title>Pengembangan Web – GoBlog.dev</title>") || !strings.Contains(res.body, "Halo, dunia") {
		t.Errorf("category page: %d", res.code)
	}
	// A category without articles in a language still shows its own name.
	_ = store.SetMeta(ctx, "why-ssr", "security", []string{"go", "c#"}) // why-ssr has no Indonesian version
	if res := send(t, app, "GET", "/id/categories/security", nil); res.code != 200 || !strings.Contains(res.body, "<title>Keamanan – GoBlog.dev</title>") {
		t.Errorf("empty category page: %d", res.code)
	}
	if send(t, app, "GET", "/en/categories/nope", nil).code != 404 {
		t.Error("unknown category isn't 404")
	}

	// Tag page, including a tag that needs URL encoding.
	res = send(t, app, "GET", "/en/tags/c%23", nil)
	if res.code != 200 || !strings.Contains(res.body, "Why server-side rendering") || strings.Contains(res.body, "result-title\" href=\"/en/posts/hello-world") {
		t.Errorf("tag page c#: %d", res.code)
	}
	if send(t, app, "GET", "/en/tags/unused", nil).code != 404 {
		t.Error("unused tag isn't 404")
	}

	// Search by tag, by category, or both.
	for q, want := range map[string][]string{
		"tag=go":                            {"why-ssr", "hello-world"},
		"category=security":                 {"why-ssr"},
		"tag=go&category=web-development":   {"hello-world"},
		"tag=welcome&category=security":     {},
		"q=crawlers&tag=go":                 {"why-ssr"},
		"q=server&tag=go&category=security": {"why-ssr"},
	} {
		body := send(t, app, "GET", "/en/search?"+q, nil).body
		for _, slug := range []string{"why-ssr", "hello-world"} {
			found := strings.Contains(body, `class="result-title" href="/en/posts/`+slug+`"`)
			if found != slices.Contains(want, slug) {
				t.Errorf("search %s: %s found=%v, want %v", q, slug, found, slices.Contains(want, slug))
			}
		}
	}

	// Article page links its category and tags.
	art := send(t, app, "GET", "/en/posts/why-ssr", nil).body
	for _, want := range []string{`<a href="/en/categories/security">Security</a>`, `href="/en/tags/c%23"`, `c#</a>`} {
		if !strings.Contains(art, want) {
			t.Errorf("article missing %s", want)
		}
	}

	// Drafts don't count: a draft in a category doesn't show on the site.
	p, _ := store.Get(ctx, "why-ssr", "en")
	p.Status = posts.Draft
	_ = store.Update(ctx, p)
	if strings.Contains(send(t, app, "GET", "/en/search?category=security", nil).body, `href="/en/posts/why-ssr"`) {
		t.Error("draft appears in category results")
	}
}
