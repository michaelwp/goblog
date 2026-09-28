package web

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/michaelputong/blog/backend/internal/posts"
)

func TestEditingPublishedArticleKeepsLiveVersion(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	ctx := t.Context()
	edit := url.Values{"title": {"Why SSR, revised"}, "summary": {"New summary"}, "body": {"secret-draft-text"}, "date": {"2026-09-25"}}

	// Save draft on a published article: live text unchanged, draft stored.
	edit.Set("action", "revise")
	res := send(t, app, "POST", "/admin/posts/why-ssr/en", edit, session)
	if res.location != "/admin/posts/why-ssr/en?notice=revised" {
		t.Fatalf("save draft: → %q", res.location)
	}
	p, _ := store.Get(ctx, "why-ssr", "en")
	if p.Status != posts.Published || p.Title != "Why server-side rendering" || p.Draft == nil || p.Draft.Title != "Why SSR, revised" {
		t.Fatalf("after save draft: status %q title %q draft %+v", p.Status, p.Title, p.Draft)
	}

	// Readers see nothing of the draft anywhere.
	for _, path := range []string{"/en/posts/why-ssr", "/en", "/en/search?q=server"} {
		body := send(t, app, "GET", path, nil).body
		if strings.Contains(body, "secret-draft-text") || strings.Contains(body, "Why SSR, revised") {
			t.Errorf("%s leaks the draft", path)
		}
	}

	// The editor and the preview show the draft.
	editor := send(t, app, "GET", "/admin/posts/why-ssr/en", nil, session).body
	if !strings.Contains(editor, "Why SSR, revised") || !strings.Contains(editor, `"draftSavedAt":"20`) {
		t.Error("editor doesn't load the pending draft")
	}
	preview := send(t, app, "GET", "/admin/posts/why-ssr/en/preview", nil, session).body
	if !strings.Contains(preview, "secret-draft-text") || !strings.Contains(preview, `"pendingChanges":true`) {
		t.Error("preview doesn't show the pending draft")
	}

	// Publish replaces the live version with the draft.
	edit.Set("action", "publish")
	send(t, app, "POST", "/admin/posts/why-ssr/en", edit, session)
	p, _ = store.Get(ctx, "why-ssr", "en")
	if p.Title != "Why SSR, revised" || p.Draft != nil || p.Status != posts.Published {
		t.Errorf("after publish: title %q draft %+v", p.Title, p.Draft)
	}
	if !strings.Contains(send(t, app, "GET", "/en/posts/why-ssr", nil).body, "secret-draft-text") {
		t.Error("published changes aren't live")
	}
}

func TestDiscardDraftChanges(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	send(t, app, "POST", "/admin/posts/why-ssr/en", url.Values{"title": {"Temp"}, "body": {"x"}, "date": {"2026-09-25"}, "action": {"revise"}}, session)
	res := send(t, app, "POST", "/admin/posts/why-ssr/en/discard", url.Values{}, session)
	p, _ := store.Get(t.Context(), "why-ssr", "en")
	if res.location != "/admin/posts/why-ssr/en?notice=discarded" || p.Draft != nil || p.Title != "Why server-side rendering" {
		t.Errorf("discard: → %q, draft %+v, title %q", res.location, p.Draft, p.Title)
	}
}

func TestDisableFoldsInPendingDraft(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	send(t, app, "POST", "/admin/posts/why-ssr/en", url.Values{"title": {"Edited"}, "body": {"x"}, "date": {"2026-09-25"}, "action": {"revise"}}, session)
	send(t, app, "POST", "/admin/bulk", url.Values{"action": {"disable:why-ssr"}}, session)
	p, _ := store.Get(t.Context(), "why-ssr", "en")
	if p.Status != posts.Disabled || p.Draft != nil || p.Title != "Edited" {
		t.Errorf("disable with pending draft: status %q title %q draft %+v", p.Status, p.Title, p.Draft)
	}
}

func TestAutosave(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	ctx := t.Context()
	call := func(path string, form url.Values) (int, map[string]string) {
		t.Helper()
		res := send(t, app, "POST", path, form, session)
		var out map[string]string
		_ = json.Unmarshal([]byte(res.body), &out)
		return res.code, out
	}

	// A new article isn't saved until it has a valid slug and title.
	code, out := call("/admin/new/autosave", url.Values{"slug": {""}, "lang": {"en"}, "title": {"x"}, "date": {"2026-09-28"}})
	if code != 422 || !strings.HasPrefix(out["error"], "Autosave paused: Enter a slug") {
		t.Errorf("autosave without slug: %d %v", code, out)
	}
	// Then it's created as a draft, and the editor learns its new address.
	code, out = call("/admin/new/autosave", url.Values{"slug": {"auto-post"}, "lang": {"en"}, "title": {"Autosaved"}, "body": {""}, "date": {"2026-09-28"}})
	if code != 200 || out["editUrl"] != "/admin/posts/auto-post/en" || out["savedAt"] == "" {
		t.Fatalf("autosave new: %d %v", code, out)
	}
	if p, _ := store.Get(ctx, "auto-post", "en"); p.Status != posts.Draft {
		t.Errorf("autosaved article status %q", p.Status)
	}
	if code, _ := call("/admin/new/autosave", url.Values{"slug": {"auto-post"}, "lang": {"en"}, "title": {"Again"}, "date": {"2026-09-28"}}); code != 409 {
		t.Errorf("autosaving a taken slug: %d, want 409", code)
	}

	// Existing draft: updated in place, still a draft.
	call("/admin/posts/auto-post/en/autosave", url.Values{"title": {"Autosaved v2"}, "body": {"more"}, "date": {"2026-09-28"}})
	if p, _ := store.Get(ctx, "auto-post", "en"); p.Title != "Autosaved v2" || p.Status != posts.Draft {
		t.Errorf("autosave draft: %q %q", p.Title, p.Status)
	}

	// Published article: autosave only touches its pending draft.
	code, _ = call("/admin/posts/why-ssr/en/autosave", url.Values{"title": {"Auto edit"}, "body": {"y"}, "date": {"2026-09-25"}})
	p, _ := store.Get(ctx, "why-ssr", "en")
	if code != 200 || p.Title != "Why server-side rendering" || p.Draft == nil || p.Draft.Title != "Auto edit" || p.Status != posts.Published {
		t.Errorf("autosave published: %d title %q draft %+v", code, p.Title, p.Draft)
	}

	// Signed out: nothing is saved.
	res := send(t, app, "POST", "/admin/posts/auto-post/en/autosave", url.Values{"title": {"Hacked"}, "date": {"2026-09-28"}})
	if res.code != 303 {
		t.Errorf("signed-out autosave: %d", res.code)
	}
	if p, _ := store.Get(ctx, "auto-post", "en"); p.Title == "Hacked" {
		t.Error("signed-out autosave changed the article")
	}
}

func TestEditorActionsAtTopAndBottom(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	body := send(t, app, "GET", "/admin/posts/why-ssr/en", nil, session).body
	top, bottom := strings.Index(body, "admin-actions-top"), strings.Index(body, "admin-actions-bottom")
	if top < 0 || bottom < 0 || strings.Count(body, `value="publish"`) != 2 {
		t.Fatalf("action bars: top %d, bottom %d, publish buttons %d", top, bottom, strings.Count(body, `value="publish"`))
	}
	// Enter submits the first button in the form: it must be the safe "Save draft", not Publish.
	first := body[strings.Index(body, `<form class="admin-card admin-form"`):]
	first = first[strings.Index(first, `type="submit"`):]
	if !strings.Contains(first[:120], `value="revise"`) {
		t.Errorf("first submit button isn't Save draft: %s", first[:120])
	}
}

func TestEditorKnowsTranslations(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	send(t, app, "POST", "/admin/posts/hello-world/id", url.Values{"title": {"Halo lagi"}, "body": {"x"}, "date": {"2026-09-20"}, "action": {"revise"}}, session)

	body := send(t, app, "GET", "/admin/posts/hello-world/en", nil, session).body
	if !strings.Contains(body, `"translations":{"en":"published","id":"edited"}`) {
		t.Errorf("edit page translations missing or wrong")
	}
	// why-ssr has no Indonesian translation: the switcher offers to start it.
	body = send(t, app, "GET", "/admin/posts/why-ssr/en", nil, session).body
	if !strings.Contains(body, `"translations":{"en":"published"}`) || !strings.Contains(body, "Not translated yet (start it)") {
		t.Errorf("switcher doesn't offer the missing translation")
	}
}
