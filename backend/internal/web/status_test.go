package web

import (
	"net/url"
	"strings"
	"testing"

	"github.com/michaelputong/blog/backend/internal/posts"
)

func TestArticleLifecycle(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	ctx := t.Context()
	public := func() int { return send(t, app, "GET", "/en/posts/lifecycle", nil).code }
	inList := func() bool { return strings.Contains(send(t, app, "GET", "/en", nil).body, "Lifecycle title") }
	status := func() posts.Status { p, _ := store.Get(ctx, "lifecycle", "en"); return p.Status }
	act := func(action string, wantNotice string) {
		t.Helper()
		res := send(t, app, "POST", "/admin/posts/lifecycle/en", url.Values{
			"title": {"Lifecycle title"}, "body": {"Body text."}, "date": {"2026-09-29"}, "action": {action},
		}, session)
		if want := "/admin/posts/lifecycle/en?notice=" + wantNotice; res.code != 303 || res.location != want {
			t.Fatalf("%s: %d → %q, want %q", action, res.code, res.location, want)
		}
	}

	// A draft may be saved without a body, and isn't public.
	res := send(t, app, "POST", "/admin/new", url.Values{
		"slug": {"lifecycle"}, "lang": {"en"}, "title": {"Lifecycle title"}, "body": {""}, "date": {"2026-09-29"}, "action": {"draft"},
	}, session)
	if res.code != 303 || res.location != "/admin/posts/lifecycle/en?notice=draft" {
		t.Fatalf("save draft: %d → %q\n%s", res.code, res.location, res.body)
	}
	if status() != posts.Draft || public() != 404 || inList() {
		t.Errorf("draft: status %q, public %d, in list %v", status(), public(), inList())
	}
	// The API doesn't reveal it either.
	if code := send(t, app, "GET", "/api/v1/en/posts/lifecycle", nil).code; code != 404 && code != 303 {
		t.Errorf("API exposes draft: %d", code)
	}

	// The admin can preview it, marked as a preview.
	res = send(t, app, "GET", "/admin/posts/lifecycle/en/preview", nil, session)
	if res.code != 200 || !strings.Contains(res.body, "preview-banner") {
		t.Errorf("preview: status %d", res.code)
	}
	if send(t, app, "GET", "/admin/posts/lifecycle/en/preview", nil).code != 303 {
		t.Error("preview works without signing in")
	}

	act("publish", "published")
	if status() != posts.Published || public() != 200 || !inList() {
		t.Errorf("published: status %q, public %d, in list %v", status(), public(), inList())
	}
	act("save", "revised") // saving a published article keeps it live and stores a draft copy
	if status() != posts.Published {
		t.Errorf("save changed status to %q", status())
	}
	act("disable", "disabled")
	if status() != posts.Disabled || public() != 404 || inList() {
		t.Errorf("disabled: status %q, public %d, in list %v", status(), public(), inList())
	}
	act("publish", "published")
	act("draft", "draft")
	if status() != posts.Draft || public() != 404 {
		t.Errorf("moved to drafts: status %q, public %d", status(), public())
	}

	// Publishing still requires a body.
	res = send(t, app, "POST", "/admin/posts/lifecycle/en", url.Values{"title": {"x"}, "body": {""}, "date": {"2026-09-29"}, "action": {"publish"}}, session)
	if res.code != 422 || status() != posts.Draft {
		t.Errorf("publish without body: status %d, post status %q", res.code, status())
	}
}

func TestDeleteWholeArticle(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)

	// hello-world exists in en and id.
	res := send(t, app, "GET", "/admin/posts/hello-world/en", nil, session)
	if !strings.Contains(res.body, "Delete the whole article") {
		t.Error("edit page doesn't offer deleting all translations")
	}
	res = send(t, app, "POST", "/admin/articles/hello-world/delete", url.Values{}, session)
	if res.code != 303 || res.location != "/admin?notice=deleted-all" {
		t.Fatalf("delete all: %d → %q", res.code, res.location)
	}
	for _, lang := range []string{"en", "id"} {
		if _, err := store.Get(t.Context(), "hello-world", lang); err == nil {
			t.Errorf("%s translation survived", lang)
		}
	}
	if send(t, app, "POST", "/admin/articles/why-ssr/delete", url.Values{}).code != 303 {
		t.Error("signed-out delete wasn't redirected to login")
	}
	if _, err := store.Get(t.Context(), "why-ssr", "en"); err != nil {
		t.Error("signed-out delete removed the article")
	}
}

func TestAdminListFilter(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	p, _ := store.Get(t.Context(), "why-ssr", "en")
	p.Status = posts.Disabled
	_ = store.Update(t.Context(), p)

	res := send(t, app, "GET", "/admin?show=disabled", nil, session)
	if !strings.Contains(res.body, "Why server-side rendering") || strings.Contains(res.body, ">Hello, world<") {
		t.Error("disabled filter shows the wrong articles")
	}
	if !strings.Contains(res.body, "chip-disabled") {
		t.Error("disabled translation isn't marked in the list")
	}
}
