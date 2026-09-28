package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/michaelputong/blog/backend/internal/posts"
)

func do(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	app := New(Config{
		Store:     posts.NewMemoryStore(posts.SeedPosts()),
		Languages: []string{"en", "id"},
	})
	resp, err := app.Test(httptest.NewRequest("GET", path, nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body
}

func TestListPostsFiltersByLanguage(t *testing.T) {
	cases := map[string]int{"en": 2, "id": 1}
	for lang, want := range cases {
		code, body := do(t, "/api/v1/"+lang+"/posts")
		if code != 200 {
			t.Fatalf("%s: status %d", lang, code)
		}
		if got := len(body["posts"].([]any)); got != want {
			t.Errorf("%s: got %d posts, want %d", lang, got, want)
		}
	}
}

func TestListPostsNewestFirst(t *testing.T) {
	_, body := do(t, "/api/v1/en/posts")
	first := body["posts"].([]any)[0].(map[string]any)
	if first["slug"] != "why-ssr" {
		t.Errorf("first post = %v, want why-ssr", first["slug"])
	}
}

func TestGetPost(t *testing.T) {
	code, body := do(t, "/api/v1/id/posts/hello-world")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if title := body["post"].(map[string]any)["title"]; title != "Halo, dunia" {
		t.Errorf("title = %v", title)
	}
	if langs := body["availableLanguages"].([]any); len(langs) != 2 {
		t.Errorf("availableLanguages = %v", langs)
	}
}

func TestNotFound(t *testing.T) {
	for _, path := range []string{
		"/api/v1/id/posts/why-ssr", // exists only in en
		"/api/v1/en/posts/missing",
		"/api/v1/fr/posts", // unsupported language
		"/api/v1/nope/nope/nope/nope",
	} {
		code, body := do(t, path)
		if code != 404 {
			t.Errorf("%s: status %d, want 404", path, code)
		}
		if body["error"] == nil {
			t.Errorf("%s: missing error field", path)
		}
	}
}

func TestUnpublishedPostsAreHidden(t *testing.T) {
	store := posts.NewMemoryStore(posts.SeedPosts())
	for _, status := range []posts.Status{posts.Draft, posts.Disabled} {
		p, _ := store.Get(t.Context(), "why-ssr", "en")
		p.Status = status
		_ = store.Update(t.Context(), p)
		app := New(Config{Store: store, Languages: []string{"en", "id"}})

		resp, _ := app.Test(httptest.NewRequest("GET", "/api/v1/en/posts/why-ssr", nil))
		if resp.StatusCode != 404 {
			t.Errorf("%s: get status %d, want 404", status, resp.StatusCode)
		}
		resp, _ = app.Test(httptest.NewRequest("GET", "/api/v1/en/posts", nil))
		var body struct{ Posts []posts.Post }
		json.NewDecoder(resp.Body).Decode(&body)
		for _, p := range body.Posts {
			if p.Slug == "why-ssr" {
				t.Errorf("%s: post appears in list", status)
			}
		}
	}
}
