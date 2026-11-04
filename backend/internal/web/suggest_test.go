package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/ai"
	"github.com/michaelputong/blog/backend/internal/categories"
	"github.com/michaelputong/blog/backend/internal/posts"
)

// tinyPNG is a 1×1 PNG, as a generated image.
var tinyPNG, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

// fakeAI answers every chat completion with reply and every image request
// with tinyPNG, and records the last request's text.
func fakeAI(t *testing.T, reply string, imageModel ...string) (*ai.Client, *string) {
	t.Helper()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/images/generations" {
			var req struct{ Prompt string }
			_ = json.NewDecoder(r.Body).Decode(&req)
			got = req.Prompt
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(tinyPNG)}}})
			return
		}
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" || len(req.Messages) != 2 {
			http.Error(w, `{"error":{"message":"bad request"}}`, http.StatusBadRequest)
			return
		}
		got = req.Messages[1].Content
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	return ai.New("test-key", srv.URL, "test-model", imageModel...), &got
}

func newSuggestApp(t *testing.T, reply string, imageModel ...string) (*fiber.App, *posts.MemoryStore, *categories.MemoryStore, *string) {
	t.Helper()
	client, got := fakeAI(t, reply, imageModel...)
	cats := &categories.MemoryStore{}
	_ = cats.Create(t.Context(), categories.Category{Slug: "go", Names: map[string]string{"en": "Go", "id": "Go"}, CreatedAt: time.Now()})
	app, store, _ := newAdminAppWith(t, testPassword, func(c *Config) { c.AI, c.Categories = client, cats })
	return app, store, cats, got
}

func suggestJSON(t *testing.T, res response) suggestion {
	t.Helper()
	var out suggestion
	if res.code != 200 || json.Unmarshal([]byte(res.body), &out) != nil {
		t.Fatalf("suggest: %d %s", res.code, res.body)
	}
	return out
}

func TestSuggestPicksExistingCategory(t *testing.T) {
	app, _, _, got := newSuggestApp(t, "Sure!\n```json\n{\"title\": \" \\\"Generics,  explained\\\" \", \"summary\": \"How type parameters work.\", \"tags\": [\"Go\", \"Generics\", \"bad tag!!\", \"generics\"], \"category\": \"go\", \"newCategory\": null, \"cover\": \"/media/b.png\"}\n```")
	session := login(t, app)
	body := "Type parameters...\n\n![a diagram](/media/a.png)\n\n![](/media/b.png \"Generic stack\")\n\n![x](javascript:alert(1))"
	out := suggestJSON(t, send(t, app, "POST", "/admin/suggest", url.Values{"title": {"Generics in Go"}, "body": {body}, "lang": {"en"}}, session))
	if out.Category != "go" || out.NewCategory != nil || strings.Join(out.Tags, ",") != "go,generics" {
		t.Errorf("suggestion = %+v", out)
	}
	if out.Title != "Generics, explained" || out.Summary != "How type parameters work." || out.Cover != "/media/b.png" {
		t.Errorf("title/summary/cover = %q %q %q", out.Title, out.Summary, out.Cover)
	}
	if !strings.Contains(*got, `"caption": "Generic stack"`) || strings.Contains(*got, `"url": "javascript:`) {
		t.Errorf("prompt's images: %s", *got)
	}
	if !strings.Contains(*got, "Generics in Go") || !strings.Contains(*got, `"slug": "go"`) {
		t.Errorf("prompt lacks the article or categories: %s", *got)
	}
}

func TestSuggestNewCategory(t *testing.T) {
	app, _, _, _ := newSuggestApp(t, `{"tags": ["bali"], "category": "nope", "newCategory": {"en": "Travel", "id": ""}}`)
	session := login(t, app)
	out := suggestJSON(t, send(t, app, "POST", "/admin/suggest", url.Values{"title": {"A week in Bali"}, "lang": {"en"}}, session))
	if out.Category != "" || out.NewCategory["en"] != "Travel" || out.NewCategory["id"] != "Travel" {
		t.Errorf("suggestion = %+v", out)
	}

	// A "new" category that's really an existing one (by name) is picked instead.
	app, _, _, _ = newSuggestApp(t, `{"tags": [], "category": "", "newCategory": {"en": "go", "id": "Go"}}`)
	session = login(t, app)
	out = suggestJSON(t, send(t, app, "POST", "/admin/suggest", url.Values{"title": {"x"}, "lang": {"en"}}, session))
	if out.Category != "go" || out.NewCategory != nil {
		t.Errorf("existing by name: %+v", out)
	}
}

func TestSuggestNeedsKeyAndSession(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	if res := send(t, app, "POST", "/admin/suggest", url.Values{"title": {"x"}}, session); res.code != 404 {
		t.Errorf("without AI configured: %d", res.code)
	}
	app, _, _, _ = newSuggestApp(t, `{}`)
	if res := send(t, app, "POST", "/admin/suggest", url.Values{"title": {"x"}}); res.code == 200 {
		t.Errorf("without a session: %d", res.code)
	}
}

func TestSavingCreatesNewCategory(t *testing.T) {
	app, store, cats, _ := newSuggestApp(t, `{}`)
	session := login(t, app)
	form := url.Values{
		"slug": {"bali"}, "lang": {"en"}, "title": {"Bali"}, "date": {"2026-11-04"}, "action": {"save"},
		"category": {newCategoryValue}, "newCategory_en": {" Travel  Notes "}, "newCategory_id": {"Catatan Perjalanan"},
	}
	if res := send(t, app, "POST", "/admin/new", form, session); res.code != 303 {
		t.Fatalf("create: %d %s", res.code, res.body)
	}
	cat, err := cats.Get(t.Context(), "travel-notes")
	if err != nil || cat.Names["en"] != "Travel Notes" || cat.Names["id"] != "Catatan Perjalanan" {
		t.Fatalf("category = %+v, %v", cat, err)
	}
	if p, _ := store.Get(t.Context(), "bali", "en"); p.Category != "travel-notes" {
		t.Errorf("article category = %q", p.Category)
	}

	// Autosave reuses it by name rather than making a duplicate, and says which it picked.
	form.Set("slug", "lombok")
	res := send(t, app, "POST", "/admin/new/autosave", form, session)
	if res.code != 200 || !strings.Contains(res.body, `"category":"travel-notes"`) {
		t.Errorf("autosave: %d %s", res.code, res.body)
	}

	// No name: the form explains, and nothing is created.
	form.Set("slug", "nameless")
	form.Del("newCategory_en")
	form.Del("newCategory_id")
	if res := send(t, app, "POST", "/admin/new", form, session); res.code != 422 || !strings.Contains(res.body, "Name the new category") {
		t.Errorf("unnamed: %d", res.code)
	}
	// An invalid article doesn't leave a category behind.
	form.Set("newCategory_en", "Orphan")
	form.Set("title", "")
	send(t, app, "POST", "/admin/new", form, session)
	if _, err := cats.Get(t.Context(), "orphan"); err == nil {
		t.Error("category created for an article that wasn't saved")
	}
}

func TestDetectProvider(t *testing.T) {
	for key, want := range map[string]string{
		"sk-proj-abc": "OpenAI", "sk-abc": "OpenAI", "sk-ant-api03-x": "Anthropic", "AIzaSyX": "Google Gemini",
		"gsk_x": "Groq", "sk-or-v1-x": "OpenRouter", "xai-x": "xAI", "whatever": "OpenAI",
	} {
		if got := ai.Detect(key).Name; got != want {
			t.Errorf("Detect(%q) = %s, want %s", key, got, want)
		}
	}
	if c := ai.New("sk-proj-x", "", ""); !c.CanGenerateImages() {
		t.Error("OpenAI keys should generate images")
	}
	if c := ai.New("sk-ant-x", "", ""); c.CanGenerateImages() {
		t.Error("Anthropic has no image model")
	}
	if c := ai.New("sk-x", "http://localhost:11434/v1", ""); c.CanGenerateImages() {
		t.Error("a custom endpoint shouldn't assume an image model")
	}
	if ai.New("  ", "", "") != nil {
		t.Error("an empty key should turn AI off")
	}
	if c := ai.New("sk-ant-x", "http://localhost:11434/v1/", "llama3"); c.Provider.BaseURL != "http://localhost:11434/v1" || c.Provider.Model != "llama3" {
		t.Errorf("overrides: %+v", c.Provider)
	}
}

func TestSuggestCoverIgnoresImagesNotInArticle(t *testing.T) {
	app, _, _, _ := newSuggestApp(t, `{"title": "", "cover": "https://evil.example/x.png"}`)
	session := login(t, app)
	out := suggestJSON(t, send(t, app, "POST", "/admin/suggest", url.Values{"title": {"x"}, "body": {"![a](/media/a.png)"}}, session))
	if out.Cover != "" {
		t.Errorf("cover = %q", out.Cover)
	}
}

func TestGenerateCover(t *testing.T) {
	app, _, _, got := newSuggestApp(t, `{}`, "test-image-model")
	session := login(t, app)
	res := send(t, app, "POST", "/admin/suggest/cover", url.Values{"title": {"A week in Bali"}, "summary": {"Rice terraces and temples."}}, session)
	var out struct {
		Cover     string `json:"cover"`
		Generated bool   `json:"generated"`
	}
	if res.code != 200 || json.Unmarshal([]byte(res.body), &out) != nil || !out.Generated || !strings.HasPrefix(out.Cover, "/media/") {
		t.Fatalf("generate: %d %s", res.code, res.body)
	}
	if !strings.Contains(*got, "A week in Bali") || !strings.Contains(*got, "no text") {
		t.Errorf("image prompt: %s", *got)
	}
	if r := send(t, app, "GET", out.Cover, nil); r.code != 200 {
		t.Errorf("generated cover isn't served: %d", r.code)
	}
}

func TestPickCoverWithoutImageModel(t *testing.T) {
	app, _, _, _ := newSuggestApp(t, `{"cover": "/media/b.png"}`)
	session := login(t, app)
	res := send(t, app, "POST", "/admin/suggest/cover", url.Values{"title": {"x"}, "body": {"![a](/media/a.png) ![b](/media/b.png)"}}, session)
	if res.code != 200 || !strings.Contains(res.body, `"cover":"/media/b.png"`) || !strings.Contains(res.body, `"generated":false`) {
		t.Errorf("pick: %d %s", res.code, res.body)
	}
	res = send(t, app, "POST", "/admin/suggest/cover", url.Values{"title": {"x"}, "body": {"no images"}}, session)
	if res.code != 422 || !strings.Contains(res.body, "can't create images") {
		t.Errorf("nothing to pick: %d %s", res.code, res.body)
	}
}

// Review fixes: a failed save leaves no category, the taken slug is named,
// renames during autosave don't duplicate, and only the default name matches.
func TestNewCategoryEdgeCases(t *testing.T) {
	app, _, cats, _ := newSuggestApp(t, `{}`)
	session := login(t, app)
	ctx := t.Context()
	newCat := func(v url.Values, names ...string) url.Values {
		v.Set("category", newCategoryValue)
		for i, lang := range []string{"en", "id"} {
			if i < len(names) {
				v.Set("newCategory_"+lang, names[i])
			}
		}
		return v
	}

	// The slug is taken: nothing is saved, no category is left behind, and
	// the error names the slug.
	res := send(t, app, "POST", "/admin/new", newCat(url.Values{"slug": {"hello-world"}, "lang": {"en"}, "title": {"Hello, World"}, "date": {"2026-11-04"}, "action": {"save"}}, "Travel", "Perjalanan"), session)
	if res.code != 422 || !strings.Contains(res.body, "“hello-world” already exists") {
		t.Errorf("taken slug: %d", res.code)
	}
	if _, err := cats.Get(ctx, "travel"); err == nil {
		t.Error("a failed save left its new category behind")
	}

	// Autosave creates the category; renamed while saving, the next save
	// renames it instead of making a second one.
	res = send(t, app, "POST", "/admin/new/autosave", newCat(url.Values{"slug": {"trip"}, "lang": {"en"}, "title": {"Trip"}, "date": {"2026-11-04"}}, "Travel", "Perjalanan"), session)
	if res.code != 200 || !strings.Contains(res.body, `"category":"travel"`) {
		t.Fatalf("autosave: %d %s", res.code, res.body)
	}
	form := newCat(url.Values{"title": {"Trip"}, "date": {"2026-11-04"}, "newCategoryReplaces": {"travel"}}, "Trips", "Wisata")
	res = send(t, app, "POST", "/admin/posts/trip/en/autosave", form, session)
	if c, err := cats.Get(ctx, "travel"); res.code != 200 || err != nil || c.Names["en"] != "Trips" || c.Names["id"] != "Wisata" {
		t.Errorf("rename: %d %s %+v", res.code, res.body, c)
	}
	if all, _ := cats.List(ctx, "en"); len(all) != 2 { // go + travel
		t.Errorf("categories = %+v", all)
	}

	// Only the default-language name finds an existing category: a copied
	// name clashing in another language asks for that name instead.
	_ = cats.Create(ctx, categories.Category{Slug: "ui", Names: map[string]string{"en": "UI/UX", "id": "Design"}, CreatedAt: time.Now()})
	res = send(t, app, "POST", "/admin/new", newCat(url.Values{"slug": {"d"}, "lang": {"en"}, "title": {"D"}, "date": {"2026-11-04"}, "action": {"save"}}, "Design"), session)
	if res.code != 422 || !strings.Contains(res.body, "enter this category&#39;s id name") && !strings.Contains(res.body, "enter this category's id name") {
		t.Errorf("copied name clash: %d", res.code)
	}
}

func TestSlugFromNameMatchesTitles(t *testing.T) {
	if got := slugFromName("Café Culture"); got != "cafe-culture" || got != slugify("Café Culture") {
		t.Errorf("slugFromName = %q", got)
	}
}

func TestCompactImage(t *testing.T) {
	if out := compactImage(tinyPNG); http.DetectContentType(out) != "image/jpeg" {
		t.Errorf("PNG not converted: %s", http.DetectContentType(out))
	}
	if out := compactImage([]byte("not an image")); string(out) != "not an image" {
		t.Error("non-images should pass through")
	}
}

func TestSuggestField(t *testing.T) {
	for _, tc := range []struct{ field, reply, want string }{
		{"title", `{"value": " \"A Week in Bali\" "}`, `"value":"A Week in Bali"`},
		{"summary", `{"value": "Rice terraces, temples and food."}`, `"value":"Rice terraces, temples and food."`},
		{"slug", `{"value": "Seminggu di Bali!"}`, `"value":"seminggu-di-bali"`},
		{"tags", `{"value": ["Bali", "travel tips", "bali"]}`, `"value":["bali","travel-tips"]`},
	} {
		app, _, _, got := newSuggestApp(t, tc.reply)
		session := login(t, app)
		res := send(t, app, "POST", "/admin/suggest/field", url.Values{"field": {tc.field}, "title": {"Bali"}, "body": {"A week in Bali."}, "lang": {"en"}}, session)
		if res.code != 200 || !strings.Contains(res.body, tc.want) {
			t.Errorf("%s: %d %s", tc.field, res.code, res.body)
		}
		if tc.field == "tags" && !strings.Contains(*got, "existingTags") {
			t.Errorf("tags prompt lacks existing tags: %s", *got)
		}
	}
	app, _, _, _ := newSuggestApp(t, `{"value": ""}`)
	session := login(t, app)
	if res := send(t, app, "POST", "/admin/suggest/field", url.Values{"field": {"title"}, "title": {"x"}}, session); res.code != 502 {
		t.Errorf("empty reply: %d", res.code)
	}
	if res := send(t, app, "POST", "/admin/suggest/field", url.Values{"field": {"body"}, "title": {"x"}}, session); res.code != 400 {
		t.Errorf("unknown field: %d", res.code)
	}
}
