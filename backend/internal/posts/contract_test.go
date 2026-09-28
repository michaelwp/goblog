package posts

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// testWrites checks the write methods against any Repository that starts
// with SeedPosts, so MemoryStore stays faithful to MongoStore.
func testWrites(t *testing.T, r Repository) {
	t.Helper()
	ctx := context.Background()
	p := Post{
		Slug: "new-post", Lang: "id", Title: "Tulisan baru", Summary: "Ringkasan",
		Body: "Isi", PublishedAt: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Status: Published,
	}

	if err := r.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.Create(ctx, p); !errors.Is(err, ErrExists) {
		t.Errorf("Create duplicate: err = %v, want ErrExists", err)
	}

	all, err := r.All(ctx)
	if err != nil || len(all) != 4 || all[0].Slug != "new-post" {
		t.Errorf("All = %d posts (first %q), %v; want 4 with new-post first", len(all), first(all), err)
	}

	p.Title = "Judul diperbarui"
	if err := r.Update(ctx, p); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, _ := r.Get(ctx, "new-post", "id"); got.Title != "Judul diperbarui" {
		t.Errorf("after Update, title = %q", got.Title)
	}
	if err := r.Update(ctx, Post{Slug: "missing", Lang: "en"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update missing: err = %v, want ErrNotFound", err)
	}

	if err := r.Delete(ctx, "new-post", "id"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, "new-post", "id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after Delete, Get err = %v, want ErrNotFound", err)
	}
	if err := r.Delete(ctx, "new-post", "id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete twice: err = %v, want ErrNotFound", err)
	}
	// Deleting one translation leaves the others.
	if err := r.Delete(ctx, "hello-world", "id"); err != nil {
		t.Fatal(err)
	}
	if langs, _ := r.Languages(ctx, "hello-world"); len(langs) != 1 || langs[0] != "en" {
		t.Errorf("after deleting id translation, languages = %v", langs)
	}
}

// testStatuses checks that drafts and disabled posts never reach the public
// queries (List, Languages, Search) but stay visible to Get and All.
func testStatuses(t *testing.T, r Repository) {
	t.Helper()
	ctx := context.Background()
	p := Post{
		Slug: "status-post", Lang: "en", Title: "Status test", Body: "unique-zebra-word",
		PublishedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Status: Draft,
	}
	if err := r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	visible := func() bool {
		list, _ := r.List(ctx, "en")
		found, _ := r.Search(ctx, "en", "unique-zebra-word")
		langs, _ := r.Languages(ctx, "status-post")
		inList := len(list) > 0 && list[0].Slug == "status-post"
		if inList != (len(found) == 1) || inList != (len(langs) == 1) {
			t.Errorf("List/Search/Languages disagree: %v %d %v", inList, len(found), langs)
		}
		return inList
	}
	check := func(status Status, wantVisible bool) {
		t.Helper()
		p.Status = status
		if err := r.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		if got := visible(); got != wantVisible {
			t.Errorf("%s: publicly visible = %v, want %v", status, got, wantVisible)
		}
		if got, err := r.Get(ctx, "status-post", "en"); err != nil || got.Status != status {
			t.Errorf("%s: Get = %q, %v", status, got.Status, err)
		}
	}
	if visible() {
		t.Error("new draft is publicly visible")
	}
	check(Published, true)
	check(Disabled, false)
	check(Draft, false)
	if all, _ := r.All(ctx); !slices.ContainsFunc(all, func(x Post) bool { return x.Slug == "status-post" }) {
		t.Error("All omits the draft")
	}

	// DeleteAll removes every translation of an article.
	p.Lang, p.Status = "id", Published
	if err := r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if n, err := r.DeleteAll(ctx, "status-post"); err != nil || n != 2 {
		t.Errorf("DeleteAll = %d, %v; want 2", n, err)
	}
	if _, err := r.Get(ctx, "status-post", "id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after DeleteAll, Get err = %v", err)
	}
}

// testDraftRevision checks that pending edits round-trip through Update and
// never leak into what the public queries return.
func testDraftRevision(t *testing.T, r Repository) {
	t.Helper()
	ctx := context.Background()
	p, err := r.Get(ctx, "why-ssr", "en")
	if err != nil {
		t.Fatal(err)
	}
	live := p.Title
	p.Draft = &Revision{Title: "Edited title", Body: "zebra-draft-word", PublishedAt: p.PublishedAt, SavedAt: time.Now().UTC().Truncate(time.Millisecond)}
	if err := r.Update(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, "why-ssr", "en")
	if got.Draft == nil || got.Draft.Title != "Edited title" || got.Title != live {
		t.Fatalf("after saving a draft: title %q, draft %+v", got.Title, got.Draft)
	}
	if found, _ := r.Search(ctx, "en", "zebra-draft-word"); len(found) != 0 {
		t.Error("search finds unpublished draft text")
	}
	list, _ := r.List(ctx, "en")
	for _, x := range list {
		if x.Draft != nil || x.Title == "Edited title" {
			t.Error("List exposes the pending draft")
		}
	}

	got.ApplyDraft()
	if err := r.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	final, _ := r.Get(ctx, "why-ssr", "en")
	if final.Draft != nil || final.Title != "Edited title" || final.Body != "zebra-draft-word" {
		t.Errorf("after publishing the draft: %q, draft %+v", final.Title, final.Draft)
	}
}

func TestMemoryStoreDraftRevision(t *testing.T) {
	testDraftRevision(t, NewMemoryStore(SeedPosts()))
}

func first(ps []Post) string {
	if len(ps) == 0 {
		return ""
	}
	return ps[0].Slug
}

func TestMemoryStoreWrites(t *testing.T) {
	testWrites(t, NewMemoryStore(SeedPosts()))
}

func TestMemoryStoreStatuses(t *testing.T) {
	testStatuses(t, NewMemoryStore(SeedPosts()))
}
