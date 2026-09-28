package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/michaelputong/blog/backend/internal/categories"
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

func TestTaxonomyEndpoints(t *testing.T) {
	store := posts.NewMemoryStore(posts.SeedPosts())
	cats := &categories.MemoryStore{}
	_ = cats.Create(t.Context(), categories.Category{Slug: "web", Names: map[string]string{"en": "Web", "id": "Web"}})
	_ = store.SetMeta(t.Context(), "why-ssr", "web", []string{"seo", "go"})
	_ = store.SetMeta(t.Context(), "hello-world", "", []string{"go"})
	app := New(Config{Store: store, Categories: cats, Languages: []string{"en", "id"}})
	get := func(path string, out any) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", path, resp.StatusCode, err)
		}
		_ = json.NewDecoder(resp.Body).Decode(out)
	}

	var list struct{ Posts []posts.Post }
	for path, want := range map[string]int{
		"/api/v1/en/posts?tag=go":              2,
		"/api/v1/en/posts?tag=Go":              2, // normalized
		"/api/v1/en/posts?category=web":        1,
		"/api/v1/en/posts?tag=go&category=web": 1,
		"/api/v1/en/posts?tag=seo&category=x":  0,
	} {
		list.Posts = nil
		get(path, &list)
		if len(list.Posts) != want {
			t.Errorf("%s: %d posts, want %d", path, len(list.Posts), want)
		}
	}

	var cs struct {
		Categories []struct {
			Slug, Name string
			Posts      int
		}
	}
	get("/api/v1/en/categories", &cs)
	if len(cs.Categories) != 1 || cs.Categories[0].Slug != "web" || cs.Categories[0].Posts != 1 {
		t.Errorf("categories: %+v", cs.Categories)
	}
	var ts struct{ Tags []posts.TagCount }
	get("/api/v1/en/tags", &ts)
	if len(ts.Tags) != 2 || ts.Tags[0] != (posts.TagCount{Tag: "go", Count: 2}) {
		t.Errorf("tags: %+v", ts.Tags)
	}
}
