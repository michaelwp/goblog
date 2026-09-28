package web

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/michaelputong/blog/backend/internal/auth"
	"github.com/michaelputong/blog/backend/internal/categories"
	"github.com/michaelputong/blog/backend/internal/media"
	"github.com/michaelputong/blog/backend/internal/posts"
	"github.com/michaelputong/blog/backend/internal/profile"
	"github.com/michaelputong/blog/backend/internal/ssr"
)

const testPassword = "Correct-Horse-7-Staple"

// newAdminApp returns an app whose admin password is testPassword, or with
// no admin account at all when password is "".
func newAdminApp(t *testing.T, password string) (*fiber.App, *posts.MemoryStore) {
	app, store, _ := newAdminAppWithLog(t, password)
	return app, store
}

func newAdminAppWithLog(t *testing.T, password string) (*fiber.App, *posts.MemoryStore, *[]string) {
	t.Helper()
	bundle, err := ServerBundle()
	if err != nil {
		t.Skipf("frontend not built (run npm run build in frontend/): %v", err)
	}
	r, err := ssr.New(bundle)
	if err != nil {
		t.Fatal(err)
	}
	store := posts.NewMemoryStore(posts.SeedPosts())
	creds := &auth.MemoryStore{}
	if password != "" {
		c, err := auth.NewCredential(password, bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		_ = creds.Create(t.Context(), c)
	}
	var logs []string
	app := fiber.New()
	Register(app, Config{
		Store: store, Profiles: &profile.MemoryStore{}, Categories: &categories.MemoryStore{}, Media: &media.MemoryStore{}, Credentials: creds, BcryptCost: bcrypt.MinCost,
		Renderer: r, Languages: []string{"en", "id"},
		Logf: func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) },
	})
	return app, store, &logs
}

type response struct {
	code     int
	body     string
	location string
	cookies  []*http.Cookie
}

func send(t *testing.T, app *fiber.App, method, path string, form url.Values, cookies ...*http.Cookie) response {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return response{resp.StatusCode, string(b), resp.Header.Get("Location"), resp.Cookies()}
}

func login(t *testing.T, app *fiber.App) *http.Cookie {
	t.Helper()
	res := send(t, app, "POST", "/admin/login", url.Values{"password": {testPassword}})
	if res.code != 303 || res.location != "/admin" {
		t.Fatalf("login: %d → %q", res.code, res.location)
	}
	for _, c := range res.cookies {
		if c.Name == sessionCookie {
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/admin" {
				t.Errorf("session cookie attributes: %+v", c)
			}
			return c
		}
	}
	t.Fatal("login set no session cookie")
	return nil
}

func TestAdminWithoutAccountGoesToSetup(t *testing.T) {
	app, _, logs := newAdminAppWithLog(t, "")
	for _, path := range []string{"/admin", "/admin/login", "/admin/profile"} {
		if res := send(t, app, "GET", path, nil); res.code != 303 || res.location != "/admin/setup" {
			t.Errorf("%s: %d → %q, want redirect to setup", path, res.code, res.location)
		}
	}
	// The setup code is logged at startup so the operator can find it.
	if len(*logs) == 0 || !strings.Contains((*logs)[0], "Admin setup code:") {
		t.Errorf("no setup code logged: %v", *logs)
	}
}

var setupCodePattern = regexp.MustCompile(`\d{4}-\d{4}`)

func TestAdminSetup(t *testing.T) {
	app, _, logs := newAdminAppWithLog(t, "")
	code := setupCodePattern.FindString((*logs)[0])

	cases := []struct {
		form url.Values
		want string
	}{
		{url.Values{"code": {"0000-0000"}, "password": {testPassword}, "confirm": {testPassword}}, "setup code isn"},
		{url.Values{"code": {code}, "password": {"alllowercase-and-long"}, "confirm": {"alllowercase-and-long"}}, "needs an uppercase letter and a number"},
		{url.Values{"code": {code}, "password": {"NoSpecialChars123"}, "confirm": {"NoSpecialChars123"}}, "needs a special character"},
		{url.Values{"code": {code}, "password": {testPassword}, "confirm": {testPassword + "x"}}, "don&#x27;t match"},
	}
	// Stays under the 5-failures-per-minute limit; auth.TestPasswordProblems covers each rule.
	for _, c := range cases {
		res := send(t, app, "POST", "/admin/setup", c.form)
		if res.code < 400 || !strings.Contains(res.body, c.want) {
			t.Errorf("setup %v: status %d, want error containing %q", c.form, res.code, c.want)
		}
	}

	// Correct code (typed without the dash) creates the account and signs in.
	res := send(t, app, "POST", "/admin/setup", url.Values{
		"code": {strings.ReplaceAll(code, "-", "")}, "password": {testPassword}, "confirm": {testPassword},
	})
	if res.code != 303 || res.location != "/admin" || len(res.cookies) == 0 {
		t.Fatalf("setup: %d → %q", res.code, res.location)
	}
	if res := send(t, app, "GET", "/admin", nil, res.cookies[0]); res.code != 200 {
		t.Errorf("after setup, /admin: status %d", res.code)
	}

	// Setup can't be repeated to take over the account.
	res = send(t, app, "POST", "/admin/setup", url.Values{"code": {code}, "password": {"attacker password"}, "confirm": {"attacker password"}})
	if res.code != 303 || res.location != "/admin/login" {
		t.Errorf("second setup: %d → %q, want redirect to login", res.code, res.location)
	}
	login(t, app) // the original password still works
}

func TestAdminChangePassword(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	oldSession := login(t, app)
	otherDevice := login(t, app)

	res := send(t, app, "POST", "/admin/password", url.Values{"current": {"wrong"}, "password": {"New-Password-123!"}, "confirm": {"New-Password-123!"}}, oldSession)
	if res.code != 422 || !strings.Contains(res.body, "current password") {
		t.Errorf("wrong current password: status %d", res.code)
	}

	res = send(t, app, "POST", "/admin/password", url.Values{"current": {testPassword}, "password": {"New-Password-123!"}, "confirm": {"New-Password-123!"}}, oldSession)
	if res.code != 303 || res.location != "/admin/password?notice=saved" {
		t.Fatalf("change: %d → %q", res.code, res.location)
	}
	// The browser that changed it gets a fresh session; others are signed out.
	var fresh *http.Cookie
	for _, c := range res.cookies {
		if c.Name == sessionCookie {
			fresh = c
		}
	}
	if fresh == nil || send(t, app, "GET", "/admin", nil, fresh).code != 200 {
		t.Error("changing browser lost its session")
	}
	if send(t, app, "GET", "/admin", nil, otherDevice).code != 303 {
		t.Error("other session still valid after password change")
	}
	// Only the new password works now.
	if send(t, app, "POST", "/admin/login", url.Values{"password": {testPassword}}).code != 401 {
		t.Error("old password still accepted")
	}
	if send(t, app, "POST", "/admin/login", url.Values{"password": {"New-Password-123!"}}).code != 303 {
		t.Error("new password rejected")
	}
}

func TestAdminRequiresLogin(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	for _, path := range []string{"/admin", "/admin/new", "/admin/posts/hello-world/en"} {
		res := send(t, app, "GET", path, nil)
		if res.code != 303 || res.location != "/admin/login" {
			t.Errorf("GET %s: %d → %q, want redirect to login", path, res.code, res.location)
		}
	}
	// Writes are refused too, and nothing changes.
	send(t, app, "POST", "/admin/posts/hello-world/en/delete", url.Values{})
	if _, err := store.Get(t.Context(), "hello-world", "en"); err != nil {
		t.Errorf("unauthenticated delete removed the post: %v", err)
	}
	// A forged or expired cookie doesn't work.
	for _, v := range []string{"9999999999.forged", "1.x", "garbage"} {
		res := send(t, app, "GET", "/admin", nil, &http.Cookie{Name: sessionCookie, Value: v})
		if res.code != 303 {
			t.Errorf("cookie %q: status %d, want redirect", v, res.code)
		}
	}
}

func TestAdminLogin(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)

	res := send(t, app, "POST", "/admin/login", url.Values{"password": {"wrong"}})
	if res.code != 401 || !strings.Contains(res.body, "That password isn") {
		t.Errorf("wrong password: status %d", res.code)
	}

	session := login(t, app)
	res = send(t, app, "GET", "/admin", nil, session)
	if res.code != 200 || !strings.Contains(res.body, "Hello, world") || res.body == "" {
		t.Fatalf("list after login: status %d", res.code)
	}

	res = send(t, app, "POST", "/admin/logout", url.Values{}, session)
	if res.code != 303 || res.location != "/admin/login" {
		t.Errorf("logout: %d → %q", res.code, res.location)
	}
}

func TestAdminLoginRateLimited(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	var last response
	for range 6 {
		last = send(t, app, "POST", "/admin/login", url.Values{"password": {"wrong"}})
	}
	if last.code != 429 {
		t.Errorf("6th attempt: status %d, want 429", last.code)
	}
}

func TestAdminRejectsCrossSitePost(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	req := httptest.NewRequest("POST", "/admin/login", strings.NewReader("password="+url.QueryEscape(testPassword)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	resp, _ := app.Test(req)
	if resp.StatusCode != 403 {
		t.Errorf("cross-site login: status %d, want 403", resp.StatusCode)
	}
}

func TestAdminCreateEditDelete(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	ctx := t.Context()

	form := url.Values{
		"slug": {"my-trip"}, "lang": {"id"}, "title": {"Perjalanan saya"},
		"summary": {"Ringkasan"}, "body": {"Paragraf satu.\r\n\r\n## Bagian\r\n\r\nParagraf dua."}, "date": {"2026-09-28"},
		"action": {"publish"},
	}
	res := send(t, app, "POST", "/admin/new", form, session)
	if res.code != 303 || res.location != "/admin/posts/my-trip/id?notice=published" {
		t.Fatalf("create: %d → %q\n%s", res.code, res.location, res.body)
	}
	p, err := store.Get(ctx, "my-trip", "id")
	if err != nil || p.Title != "Perjalanan saya" || strings.Contains(p.Body, "\r") {
		t.Fatalf("created post = %+v, %v", p, err)
	}
	// The new article is live on the public site.
	if res := send(t, app, "GET", "/id/posts/my-trip", nil); res.code != 200 || !strings.Contains(res.body, `id="bagian"`) {
		t.Errorf("public page: status %d", res.code)
	}

	// Creating the same slug+language again is a validation error.
	res = send(t, app, "POST", "/admin/new", form, session)
	if res.code != 422 || !strings.Contains(res.body, "already exists") {
		t.Errorf("duplicate create: status %d", res.code)
	}

	// Edit: slug and lang come from the URL; form values for them are ignored.
	form.Set("title", "Judul baru")
	form.Set("slug", "hijacked")
	form.Set("action", "save")
	res = send(t, app, "POST", "/admin/posts/my-trip/id", form, session)
	// It's published, so the edit is saved as a draft copy; the live title stays.
	if res.code != 303 || res.location != "/admin/posts/my-trip/id?notice=revised" {
		t.Fatalf("update: %d → %q", res.code, res.location)
	}
	if p, _ := store.Get(ctx, "my-trip", "id"); p.Title != "Perjalanan saya" || p.Editing().Title != "Judul baru" {
		t.Errorf("after update, live title %q, draft title %q", p.Title, p.Editing().Title)
	}
	if _, err := store.Get(ctx, "hijacked", "id"); err == nil {
		t.Error("update created a post under the form's slug")
	}

	res = send(t, app, "POST", "/admin/posts/my-trip/id/delete", url.Values{}, session)
	if res.code != 303 || res.location != "/admin?notice=deleted" {
		t.Fatalf("delete: %d → %q", res.code, res.location)
	}
	if _, err := store.Get(ctx, "my-trip", "id"); err == nil {
		t.Error("post still exists after delete")
	}
}

func TestAdminKeepsTimeOfDayWhenDateUnchanged(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	before, _ := store.Get(t.Context(), "hello-world", "en") // seeded at 09:00 UTC

	form := url.Values{"title": {"Hello again"}, "body": {"Text"}, "date": {before.PublishedAt.Format("2006-01-02")}}
	send(t, app, "POST", "/admin/posts/hello-world/en", form, session)
	after, _ := store.Get(t.Context(), "hello-world", "en")
	if !after.PublishedAt.Equal(before.PublishedAt) {
		t.Errorf("publishedAt changed from %v to %v", before.PublishedAt, after.PublishedAt)
	}
}

func TestAdminValidation(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	res := send(t, app, "POST", "/admin/new", url.Values{
		"slug": {"Bad Slug!"}, "lang": {"fr"}, "title": {""}, "body": {""}, "date": {"not-a-date"}, "action": {"publish"},
	}, session)
	if res.code != 422 {
		t.Fatalf("status %d, want 422", res.code)
	}
	for _, want := range []string{"Use lowercase letters", "Choose a language.", "Enter a title.", "Write the article body before publishing", "Enter a valid date."} {
		if !strings.Contains(res.body, want) {
			t.Errorf("missing error %q", want)
		}
	}
}

func TestSessionTokens(t *testing.T) {
	cred, _ := auth.NewCredential(testPassword, bcrypt.MinCost)
	now := time.Unix(1_000_000, 0)
	token, _ := issueToken(cred, now)
	if !validToken(token, cred, now) {
		t.Fatal("fresh token invalid")
	}
	if validToken(token, cred, now.Add(sessionTTL+time.Second)) {
		t.Error("expired token still valid")
	}
	other, _ := auth.NewCredential(testPassword, bcrypt.MinCost) // different key
	if validToken(token, other, now) {
		t.Error("token valid under a different key")
	}
	changed, _ := cred.WithPassword("another password", bcrypt.MinCost)
	if validToken(token, changed, now) {
		t.Error("token valid after password change")
	}
	for _, bad := range []string{"", "garbage", "1.1", token + "x"} {
		if validToken(bad, cred, now) {
			t.Errorf("malformed token %q accepted", bad)
		}
	}
}

func TestAdminPagesLoadAdminBundle(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	admin := send(t, app, "GET", "/admin/posts/hello-world/en", nil, session).body
	public := send(t, app, "GET", "/en/posts/hello-world", nil).body
	if !strings.Contains(admin, `src="/assets/admin.js?v=`) || strings.Contains(admin, "/assets/app.js") {
		t.Error("admin page doesn't load admin.js")
	}
	if !strings.Contains(public, `src="/assets/app.js?v=`) || strings.Contains(public, "admin.js") {
		t.Error("public page doesn't load app.js")
	}
	// Before JavaScript runs, the editor is a Markdown textarea that submits the body.
	if !strings.Contains(admin, `<textarea name="body" class="rich-source"`) {
		t.Error("editor fallback textarea missing from server-rendered page")
	}
}
