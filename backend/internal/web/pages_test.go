package web

import (
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/auth"
	"github.com/michaelputong/blog/backend/internal/categories"
	"github.com/michaelputong/blog/backend/internal/media"
	"github.com/michaelputong/blog/backend/internal/posts"
	"github.com/michaelputong/blog/backend/internal/profile"
	"github.com/michaelputong/blog/backend/internal/ssr"
)

func newApp(t *testing.T, seed []posts.Post) *fiber.App {
	t.Helper()
	bundle, err := ServerBundle()
	if err != nil {
		t.Skipf("frontend not built (run npm run build in frontend/): %v", err)
	}
	r, err := ssr.New(bundle)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	Register(app, Config{Store: posts.NewMemoryStore(seed), Profiles: &profile.MemoryStore{}, Categories: &categories.MemoryStore{}, Media: &media.MemoryStore{}, Credentials: &auth.MemoryStore{}, Logf: func(string, ...any) {}, Renderer: r, Languages: []string{"en", "id"}})
	return app
}

func get(t *testing.T, app *fiber.App, path string, headers ...string) (int, string, map[string][]string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header
}

func TestPagesAreServerRendered(t *testing.T) {
	app := newApp(t, posts.SeedPosts())

	code, body, _ := get(t, app, "/en")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	for _, want := range []string{
		`<html lang="en">`,
		"<title>GoBlog.dev – Sharing tech</title>",
		`<a href="/en/posts/why-ssr">Why server-side rendering</a>`, // featured article
		"September 25, 2026",
		`window.__PAGE__ = {"page":"home"`,
		`<script src="/assets/app.js?v=`,
		fmt.Sprintf("goblog.dev © %d", time.Now().Year()),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/en missing %q", want)
		}
	}

	code, body, _ = get(t, app, "/id/posts/hello-world")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	for _, want := range []string{
		`<html lang="id">`,
		"<title>Halo, dunia – GoBlog.dev</title>",
		`<h2 id="cara-kerjanya">Cara kerjanya</h2>`,  // section heading
		`<a href="#cara-kerjanya">Cara kerjanya</a>`, // table of contents
		`<link rel="alternate" hreflang="en" href="/en/posts/hello-world">`,
		"20 September 2026",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/id/posts/hello-world missing %q", want)
		}
	}
}

func TestThemeCookie(t *testing.T) {
	app := newApp(t, posts.SeedPosts())
	cases := []struct{ cookie, htmlTag, theme string }{
		{"", `<html lang="en">`, "auto"},
		{"theme=dark", `<html lang="en" data-theme="dark">`, "dark"},
		{"theme=light", `<html lang="en" data-theme="light">`, "light"},
		{`theme="><script>`, `<html lang="en">`, "auto"}, // unknown values fall back to auto
	}
	for _, c := range cases {
		_, body, _ := get(t, app, "/en", "Cookie", c.cookie)
		for _, want := range []string{
			c.htmlTag,
			`checked="" value="` + c.theme + `"`, // Appearance menu selection
			`"theme":"` + c.theme + `"`,          // hydration data
		} {
			if !strings.Contains(body, want) {
				t.Errorf("cookie %q: missing %s", c.cookie, want)
			}
		}
	}
}

func TestSearchPage(t *testing.T) {
	app := newApp(t, posts.SeedPosts())
	cases := []struct{ path, want string }{
		{"/en/search", "Enter a word or phrase"},
		{"/en/search?q=crawlers", "1 result for “crawlers”"},
		{"/en/search?q=Server", `<mark>server</mark>-side`},
		{"/id/search?q=dunia", `<mark>dunia</mark>`},
		{"/en/search?q=zzzz", "There were no results matching “zzzz”."},
		{"/en/search?q=%3Cscript%3E", "&lt;script&gt;"}, // query is escaped
	}
	for _, c := range cases {
		code, body, _ := get(t, app, c.path)
		if code != 200 || !strings.Contains(body, c.want) {
			t.Errorf("%s: status %d, want body containing %q", c.path, code, c.want)
		}
	}
}

func TestHydrationDataIsEscaped(t *testing.T) {
	app := newApp(t, []posts.Post{{
		Slug: "xss", Lang: "en", Title: "</script><script>alert(1)</script>",
		PublishedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}})
	_, body, _ := get(t, app, "/en/posts/xss")
	if strings.Contains(body, "<script>alert(1)") {
		t.Fatalf("unescaped title in page:\n%s", body)
	}
}

func TestRootRedirectsToPreferredLanguage(t *testing.T) {
	app := newApp(t, posts.SeedPosts())
	cases := map[string]string{
		"":                        "/en",
		"id-ID,id;q=0.9,en;q=0.8": "/id",
		"fr,en;q=0.5":             "/en",
		"fr":                      "/en",
	}
	for header, want := range cases {
		code, _, h := get(t, app, "/", "Accept-Language", header)
		if code != 302 || h["Location"][0] != want {
			t.Errorf("Accept-Language %q: %d %v, want 302 %s", header, code, h["Location"], want)
		}
	}
}

func TestNotFoundPages(t *testing.T) {
	app := newApp(t, posts.SeedPosts())
	cases := map[string]string{
		"/id/posts/why-ssr":  "Halaman tidak ditemukan", // only exists in en
		"/id/unknown/path":   "Halaman tidak ditemukan",
		"/fr":                "Page not found",
		"/assets/missing.js": "Page not found",
	}
	for path, want := range cases {
		code, body, _ := get(t, app, path)
		if code != 404 || !strings.Contains(body, want) {
			t.Errorf("%s: status %d, want 404 containing %q", path, code, want)
		}
	}
}

func TestAssetsServedFromEmbed(t *testing.T) {
	app := newApp(t, posts.SeedPosts())
	code, body, h := get(t, app, "/assets/app.js")
	if code != 200 || len(body) == 0 {
		t.Fatalf("status %d, %d bytes", code, len(body))
	}
	if cc := h["Cache-Control"]; len(cc) == 0 || !strings.Contains(cc[0], "max-age=31536000") {
		t.Errorf("Cache-Control = %v", cc)
	}
}
