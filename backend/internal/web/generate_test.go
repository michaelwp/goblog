package web

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestParseGenerated(t *testing.T) {
	out, err := parseGenerated("Sure!\n{\"title\": \"T\", \"tags\": [\"a\"]}\n" + bodyMarker + "\n```markdown\n## One\n\nText\n```\n")
	if err != nil || out.Title != "T" || out.Body != "## One\n\nText" {
		t.Errorf("marker form: %+v %v", out, err)
	}
	out, err = parseGenerated(`{"title": "T", "body": "Inline body"}`)
	if err != nil || out.Body != "Inline body" {
		t.Errorf("body in JSON: %+v %v", out, err)
	}
	if _, err := parseGenerated("no json here"); err == nil {
		t.Error("expected an error")
	}
}

func TestGenerateArticle(t *testing.T) {
	reply := `{"reply": "Here's a guide to Go errors.", "title": "Handling Errors in Go", "slug": "Go Errors Guide!", "summary": "How to wrap and check errors.", "tags": ["Go", "errors"], "category": "go", "newCategory": null}
` + bodyMarker + `
## Why errors are values

Go returns errors as values.`
	app, _, _, got := newSuggestApp(t, reply)
	session := login(t, app)

	res := send(t, app, "POST", "/admin/generate", url.Values{"prompt": {"Write about Go errors"}, "lang": {"en"}}, session)
	var out generatedArticle
	if res.code != 200 || json.Unmarshal([]byte(res.body), &out) != nil {
		t.Fatalf("generate: %d %s", res.code, res.body)
	}
	if out.Title != "Handling Errors in Go" || out.Slug != "go-errors-guide" || out.Category != "go" || strings.Join(out.Tags, ",") != "go,errors" ||
		!strings.HasPrefix(out.Body, "## Why errors") || out.Reply == "" {
		t.Errorf("article = %+v", out)
	}
	if !strings.Contains(*got, `"request": "Write about Go errors"`) || strings.Contains(*got, "currentArticle") {
		t.Errorf("first prompt: %s", *got)
	}

	// A follow-up revises the draft in the form, with the earlier requests.
	follow := url.Values{"prompt": {"Make it shorter"}, "history": {"Write about Go errors"}, "lang": {"en"}, "title": {out.Title}, "body": {out.Body}, "tags": {"go, errors"}}
	if res := send(t, app, "POST", "/admin/generate", follow, session); res.code != 200 {
		t.Fatalf("follow-up: %d %s", res.code, res.body)
	}
	if !strings.Contains(*got, "currentArticle") || !strings.Contains(*got, `"earlierRequests"`) || !strings.Contains(*got, "Make it shorter") {
		t.Errorf("follow-up prompt: %s", *got)
	}

	if res := send(t, app, "POST", "/admin/generate", url.Values{"prompt": {"  "}}, session); res.code != 422 {
		t.Errorf("empty prompt: %d", res.code)
	}
	if res := send(t, app, "POST", "/admin/generate", url.Values{"prompt": {"x"}}); res.code == 200 {
		t.Error("generated without a session")
	}
}

func TestGenerateArticleNewCategory(t *testing.T) {
	app, _, _, _ := newSuggestApp(t, `{"title": "Bali", "slug": "", "summary": "", "tags": [], "category": "", "newCategory": {"en": "Travel", "id": "Perjalanan"}}`+"\n"+bodyMarker+"\nText.")
	session := login(t, app)
	res := send(t, app, "POST", "/admin/generate", url.Values{"prompt": {"Bali trip"}, "lang": {"en"}}, session)
	if res.code != 200 || !strings.Contains(res.body, `"newCategory":{"en":"Travel","id":"Perjalanan"}`) || !strings.Contains(res.body, `"slug":"bali"`) {
		t.Errorf("new category / slug from title: %d %s", res.code, res.body)
	}
}

// The article is written in the chosen language, whatever the request's.
func TestGenerateInChosenLanguage(t *testing.T) {
	client, got := fakeAI(t, `{"title": "Seminggu di Bali", "slug": "seminggu-di-bali"}`+"\n"+bodyMarker+"\nTeks.")
	app, _, _ := newAdminAppWith(t, testPassword, func(c *Config) { c.AI = client })
	session := login(t, app)
	if res := send(t, app, "POST", "/admin/generate", url.Values{"prompt": {"A week in Bali"}, "lang": {"id"}}, session); res.code != 200 {
		t.Fatalf("generate: %d %s", res.code, res.body)
	}
	if !strings.Contains(*got, `"articleLanguage": "Indonesian (Bahasa Indonesia)"`) {
		t.Errorf("prompt doesn't name the language: %s", *got)
	}
}
