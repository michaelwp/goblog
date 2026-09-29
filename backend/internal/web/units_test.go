package web

// Unit tests for the web package's helpers. Handlers are covered by the
// request-level tests in the other _test.go files.

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/posts"
)

func TestNegotiate(t *testing.T) {
	supported := []string{"en", "id"}
	cases := map[string]string{
		"":                           "en", // no header: default language
		"id":                         "id",
		"id-ID,id;q=0.9,en;q=0.8":    "id",
		"en-US,en;q=0.9,id;q=0.8":    "en",
		"fr-FR,fr;q=0.9,id;q=0.5":    "id", // first supported by preference
		"fr,de":                      "en", // nothing supported: default
		"en;q=0.2,id;q=0.8":          "id", // q-values beat order
		"ID":                         "id", // case-insensitive
		"id;q=0":                     "en", // q=0 means "not acceptable"
		"  id-ID ;q=0.7 , en;q=0.6 ": "id",
	}
	for header, want := range cases {
		if got := negotiate(header, supported); got != want {
			t.Errorf("negotiate(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestSlugFromName(t *testing.T) {
	for name, want := range map[string]string{
		"Web Development":     "web-development",
		"  Web & Cloud!  ":    "web-cloud",
		"C++ / Go":            "c-go",
		"Keamanan Siber 2026": "keamanan-siber-2026",
		"日本語":                 "", // no ASCII letters: the form asks for a slug
	} {
		if got := slugFromName(name); got != want {
			t.Errorf("slugFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestNextStatusAndNotice(t *testing.T) {
	cases := []struct {
		action  string
		current posts.Status
		want    posts.Status
		notice  string
	}{
		{"", "", posts.Draft, "draft"}, // a new article defaults to draft
		{"draft", posts.Published, posts.Draft, "draft"},
		{"publish", posts.Draft, posts.Published, "published"},
		{"disable", posts.Published, posts.Disabled, "disabled"},
		{"save", posts.Disabled, posts.Disabled, "saved"},
		{"save", posts.Draft, posts.Draft, "draft"},
		{"revise", posts.Published, posts.Published, "saved"},
		{"bogus", posts.Published, posts.Published, "saved"}, // unknown actions keep the status
	}
	for _, c := range cases {
		got := nextStatus(c.action, c.current)
		if got != c.want {
			t.Errorf("nextStatus(%q, %q) = %q, want %q", c.action, c.current, got, c.want)
		}
		if n := editNotice(c.action, got); n != c.notice {
			t.Errorf("editNotice(%q, %q) = %q, want %q", c.action, got, n, c.notice)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	for _, c := range []struct{ password, confirm, want string }{
		{"Str0ng!Passw0rd", "Str0ng!Passw0rd", ""},
		{"Str0ng!Passw0rd", "Str0ng!Passw0rX", "The passwords don't match."},
		{"weak", "weak", "at least 12 characters"},
		{"NoDigitsHere!!", "NoDigitsHere!!", "a number"},
	} {
		got := validatePassword(c.password, c.confirm)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("validatePassword(%q) = %q, want %q", c.password, got, c.want)
		}
	}
}

func TestImageURLProblem(t *testing.T) {
	for url, wantOK := range map[string]bool{
		"/media/0123456789abcdef01234567.png":      true,
		"/media/0123456789abcdef01234567.svg":      false, // not an upload type
		"/media/../secret.png":                     false,
		"https://example.com/me.jpg":               true,
		"http://example.com/me.jpg":                true,
		"ftp://example.com/me.jpg":                 false,
		"https://drive.google.com/file/d/x/view":   false,
		"https://docs.google.com/uc?id=x":          false,
		"https://photos.app.goo.gl/abc":            false,
		"https://www.dropbox.com/s/x/me.jpg?dl=0":  false,
		"https://www.dropbox.com/s/x/me.jpg?raw=1": true,
		"javascript:alert(1)":                      false,
		"https://":                                 false,
	} {
		if got := imageURLProblem(url, "photo") == ""; got != wantOK {
			t.Errorf("imageURLProblem(%q) ok = %v, want %v (%s)", url, got, wantOK, imageURLProblem(url, "photo"))
		}
	}
	// Messages name the field's own upload button.
	if msg := imageURLProblem("https://drive.google.com/file/d/x/view", "image"); !strings.Contains(msg, "Use Upload image instead") {
		t.Errorf("cover message = %q, want it to name Upload image", msg)
	}
}

func TestSetupCodes(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	var logs []string
	s := &setupCodes{now: func() time.Time { return now }, logf: func(f string, a ...any) { logs = append(logs, f) }}

	code := s.current()
	if len(code) != 9 || code[4] != '-' || s.current() != code || len(logs) != 1 {
		t.Fatalf("code %q, logged %d times; want one stable NNNN-NNNN code logged once", code, len(logs))
	}
	for _, typed := range []string{code, strings.ReplaceAll(code, "-", ""), " " + code + " ", code[:4] + " " + code[5:]} {
		if !s.matches(typed) {
			t.Errorf("matches(%q) = false", typed)
		}
	}
	if s.matches("0000-0000") && code != "0000-0000" {
		t.Error("wrong code accepted")
	}

	now = now.Add(setupCodeTTL + time.Second) // expired: a new code, logged again
	if next := s.current(); next == "" || len(logs) != 2 {
		t.Errorf("after expiry: code %q, logged %d times", next, len(logs))
	}
	s.clear()
	if s.current(); len(logs) != 3 {
		t.Error("clear didn't force a new code")
	}
}

func TestGroupByCategory(t *testing.T) {
	p := func(slug, cat string) posts.Post { return posts.Post{Slug: slug, Category: cat} }
	nav := []navCategory{{Slug: "go", Name: "Go"}, {Slug: "web", Name: "Web"}}
	list := []posts.Post{p("a", "web"), p("b", "go"), p("c", ""), p("d", "web"), p("e", "gone"), p("f", "web"),
		p("g", "web"), p("h", "web"), p("i", "web")} // web has 6 posts: only 5 shown

	groups := groupByCategory(list, nav)
	if len(groups) != 3 {
		t.Fatalf("got %d groups, want go, web and other", len(groups))
	}
	if g := groups[0]; g.Category.Slug != "go" || g.Category.Count != 1 {
		t.Errorf("first group = %+v", g.Category)
	}
	if g := groups[1]; g.Category.Slug != "web" || g.Category.Count != 6 || len(g.Posts) != postsPerHomeGroup || g.Posts[0].Slug != "a" {
		t.Errorf("web group: count %d, %d posts, first %q", g.Category.Count, len(g.Posts), g.Posts[0].Slug)
	}
	// "Other" collects posts without a category and posts in unlisted ones.
	if g := groups[2]; g.Category.Slug != "" || g.Category.Count != 2 || g.Posts[0].Slug != "c" || g.Posts[1].Slug != "e" {
		t.Errorf("other group = %+v", g)
	}
	if got := groupByCategory(nil, nav); len(got) != 0 {
		t.Errorf("no posts: %d groups, want none", len(got))
	}
	if nav[0].Count != 0 {
		t.Error("groupByCategory changed its nav argument")
	}
}

func TestAssetVersion(t *testing.T) {
	a := fstest.MapFS{"app.js": {Data: []byte("one")}, "app.css": {Data: []byte("x")}}
	b := fstest.MapFS{"app.js": {Data: []byte("two")}, "app.css": {Data: []byte("x")}}
	va, vb := assetVersion(a), assetVersion(b)
	if len(va) != 12 || va == vb || va != assetVersion(a) {
		t.Errorf("assetVersion: %q vs %q; want stable 12-char hashes that change with content", va, vb)
	}
}

func TestBundleOf(t *testing.T) {
	for page, want := range map[string]string{"home": "app", "post": "app", "adminEdit": "admin", "adminLogin": "admin"} {
		if got := bundleOf(base{Page: page}); got != want {
			t.Errorf("bundleOf(%s) = %q, want %q", page, got, want)
		}
	}
	if bundleOf(struct{}{}) != "app" {
		t.Error("unknown page types should get app.js")
	}
}

func TestWithQuery(t *testing.T) {
	if got := withQuery("/admin", "a=1"); got != "/admin?a=1" {
		t.Errorf("got %q", got)
	}
	if got := withQuery("/admin?show=draft", "a=1"); got != "/admin?show=draft&a=1" {
		t.Errorf("got %q", got)
	}
}

// Helpers that read the request are exercised through a tiny Fiber app.
func TestRequestHelpers(t *testing.T) {
	app := fiber.New()
	app.Post("/origin", func(c *fiber.Ctx) error {
		if sameOrigin(c) {
			return c.SendString("same")
		}
		return c.SendString("cross")
	})
	app.Get("/bulk", func(c *fiber.Ctx) error { return c.JSON(bulkFromQuery(c)) })
	app.Get("/p/:v", func(c *fiber.Ctx) error { return c.SendString(pathParam(c, "v")) })
	call := func(method, path, origin string) string {
		req := httptest.NewRequest(method, "http://blog.test"+path, nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		b.Write(buf[:n])
		return b.String()
	}

	for origin, want := range map[string]string{
		"":                      "same", // older browsers omit Origin on same-origin posts
		"http://blog.test":      "same",
		"https://evil.example":  "cross",
		"null":                  "cross", // sandboxed/opaque origins
		"http://blog.test.evil": "cross",
	} {
		if got := call("POST", "/origin", origin); got != want {
			t.Errorf("sameOrigin(Origin: %q) = %s, want %s", origin, got, want)
		}
	}

	if got := call("GET", "/bulk?done=publish&articles=2&translations=3&skipped=1", ""); got != `{"verb":"publish","articles":2,"translations":3,"skipped":1}` {
		t.Errorf("bulkFromQuery = %s", got)
	}
	if got := call("GET", "/bulk?done=explode", ""); got != "null" {
		t.Errorf("unknown verb: %s, want null", got)
	}
	if got := call("GET", "/bulk?done=delete&articles=-4&translations=abc", ""); got != `{"verb":"delete","articles":0,"translations":0,"skipped":0}` {
		t.Errorf("bad numbers: %s", got)
	}
	if got := call("GET", "/p/c%23", ""); got != "c#" {
		t.Errorf("pathParam decoded %q, want c#", got)
	}
}

func TestAdminSetupAndPasswordPages(t *testing.T) {
	// No account: the setup page renders; with one, it redirects to login.
	app, _ := newAdminApp(t, "")
	if res := send(t, app, "GET", "/admin/setup", nil); res.code != 200 || !strings.Contains(res.body, "Set up admin") {
		t.Errorf("setup page: %d", res.code)
	}
	app, _ = newAdminApp(t, testPassword)
	if res := send(t, app, "GET", "/admin/setup", nil); res.code != 303 || res.location != "/admin/login" {
		t.Errorf("setup with an account: %d → %q", res.code, res.location)
	}
	// Signed in, the login page forwards to the admin, and the password page renders.
	session := login(t, app)
	if res := send(t, app, "GET", "/admin/login", nil, session); res.location != "/admin" {
		t.Errorf("login page while signed in → %q", res.location)
	}
	if res := send(t, app, "GET", "/admin/password", nil, session); res.code != 200 || !strings.Contains(res.body, "Change password") {
		t.Errorf("password page: %d", res.code)
	}
	if res := send(t, app, "GET", "/admin/password?notice=saved", nil, session); !strings.Contains(res.body, `"notice":"saved"`) {
		t.Error("password page ignores the saved notice")
	}
}
