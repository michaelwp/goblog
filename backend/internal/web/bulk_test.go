package web

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/michaelputong/blog/backend/internal/posts"
)

func TestBulkActions(t *testing.T) {
	app, store := newAdminApp(t, testPassword)
	session := login(t, app)
	ctx := t.Context()
	status := func(slug, lang string) posts.Status { p, _ := store.Get(ctx, slug, lang); return p.Status }
	bulk := func(form url.Values) response {
		t.Helper()
		return send(t, app, "POST", "/admin/bulk", form, session)
	}

	// Disable two articles at once: every translation changes.
	res := bulk(url.Values{"action": {"disable"}, "slug": {"hello-world", "why-ssr"}})
	if !strings.Contains(res.location, "done=disable") || !strings.Contains(res.location, "articles=2") || !strings.Contains(res.location, "translations=3") {
		t.Fatalf("disable: → %q", res.location)
	}
	for _, k := range [][2]string{{"hello-world", "en"}, {"hello-world", "id"}, {"why-ssr", "en"}} {
		if status(k[0], k[1]) != posts.Disabled {
			t.Errorf("%v not disabled", k)
		}
	}
	if send(t, app, "GET", "/en/posts/why-ssr", nil).code != 404 {
		t.Error("disabled article still public")
	}

	// A row's own menu acts on just that article, ignoring checked boxes.
	bulk(url.Values{"action": {"publish:why-ssr"}, "slug": {"hello-world", "why-ssr"}})
	if status("why-ssr", "en") != posts.Published || status("hello-world", "en") != posts.Disabled {
		t.Errorf("row publish: why-ssr %q, hello-world %q", status("why-ssr", "en"), status("hello-world", "en"))
	}

	// Publishing skips empty drafts (a body is required) and says so.
	_ = store.Create(ctx, posts.Post{Slug: "empty", Lang: "en", Title: "Empty", Status: posts.Draft, PublishedAt: time.Now()})
	res = bulk(url.Values{"action": {"publish"}, "slug": {"empty", "hello-world"}})
	if !strings.Contains(res.location, "skipped=1") || status("empty", "en") != posts.Draft || status("hello-world", "id") != posts.Published {
		t.Errorf("publish with empty draft: → %q, empty %q", res.location, status("empty", "en"))
	}
	list := send(t, app, "GET", res.location, nil, session).body
	if !strings.Contains(list, "1 empty draft skipped") {
		t.Error("list doesn't report the skipped draft")
	}

	// Delete several articles in every language; the filter is kept.
	res = bulk(url.Values{"action": {"delete"}, "slug": {"hello-world", "empty"}, "show": {"draft"}})
	if !strings.HasPrefix(res.location, "/admin?show=draft&") || !strings.Contains(res.location, "translations=3") {
		t.Errorf("delete: → %q", res.location)
	}
	if _, err := store.Get(ctx, "hello-world", "id"); err == nil {
		t.Error("hello-world survived")
	}
	if _, err := store.Get(ctx, "why-ssr", "en"); err != nil {
		t.Error("unselected article was deleted")
	}

	// Nothing selected, unknown actions and signed-out requests change nothing.
	if res := bulk(url.Values{"action": {"delete"}}); !strings.Contains(res.location, "notice=none-selected") {
		t.Errorf("empty selection: → %q", res.location)
	}
	bulk(url.Values{"action": {"explode"}, "slug": {"why-ssr"}})
	send(t, app, "POST", "/admin/bulk", url.Values{"action": {"delete"}, "slug": {"why-ssr"}}) // no session
	if _, err := store.Get(ctx, "why-ssr", "en"); err != nil {
		t.Error("why-ssr deleted by an invalid or signed-out request")
	}
}

func TestBulkListRendersControls(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	body := send(t, app, "GET", "/admin", nil, session).body
	for _, want := range []string{
		`<form action="/admin/bulk" method="post">`,
		`name="slug" value="hello-world"`,
		`value="delete:hello-world"`,
		`value="publish"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %s", want)
		}
	}
}
