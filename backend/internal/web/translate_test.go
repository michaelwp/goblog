package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/michaelputong/blog/backend/internal/ai"
)

func TestMarkdownChunks(t *testing.T) {
	md := "# One\n\npara a\n\n```go\nx := 1\n\ny := 2\n```\n\npara b\n\npara c"
	got := markdownChunks(md, 10)
	want := []string{"# One\n\npara a", "```go\nx := 1\n\ny := 2\n```", "para b\n\npara c"} // short blocks share a chunk
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("chunks = %q", got)
	}
	if strings.Join(markdownChunks(md, 1000), "") != md || markdownChunks("  ", 10) != nil {
		t.Error("small or empty input")
	}
}

func TestUnfence(t *testing.T) {
	for in, want := range map[string]string{
		"```markdown\n# Hi\n\ntext\n```":      "# Hi\n\ntext",
		"plain":                               "plain",
		"```go\na\n```\n\nb\n\n```go\nc\n```": "```go\na\n```\n\nb\n\n```go\nc\n```", // real code blocks stay
	} {
		if got := unfence(in); got != want {
			t.Errorf("unfence(%q) = %q", in, got)
		}
	}
}

func TestTranslate(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		system, user := req.Messages[0].Content, req.Messages[1].Content
		if !strings.Contains(system, "from English to Indonesian") {
			t.Errorf("system prompt: %s", system)
		}
		reply := "```markdown\n" + strings.ToUpper(user) + "\n```" // the "translation"
		if strings.Contains(system, "JSON object") {
			var in map[string]string
			_ = json.Unmarshal([]byte(user), &in)
			b, _ := json.Marshal(map[string]string{"title": "Judul " + in["title"], "summary": "Ringkasan"})
			reply = string(b)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	}))
	defer srv.Close()
	app, _, _ := newAdminAppWith(t, testPassword, func(c *Config) { c.AI = ai.New("test-key", srv.URL, "m") })
	session := login(t, app)

	res := send(t, app, "POST", "/admin/translate", url.Values{"slug": {"hello-world"}, "from": {"en"}, "to": {"id"}}, session)
	var out translation
	if res.code != 200 || json.Unmarshal([]byte(res.body), &out) != nil {
		t.Fatalf("translate: %d %s", res.code, res.body)
	}
	if !strings.HasPrefix(out.Title, "Judul ") || out.Summary == "" || out.Body == "" || out.Body != strings.ToUpper(out.Body) || strings.Contains(out.Body, "```markdown") {
		t.Errorf("translation = %+v", out)
	}

	// only=body: just the text, no title or summary call.
	before := calls
	res = send(t, app, "POST", "/admin/translate", url.Values{"slug": {"hello-world"}, "from": {"en"}, "to": {"id"}, "only": {"body"}}, session)
	out = translation{}
	if res.code != 200 || json.Unmarshal([]byte(res.body), &out) != nil || out.Title != "" || out.Summary != "" || out.Body == "" {
		t.Errorf("body only: %d %s", res.code, res.body)
	}
	if calls-before != len(markdownChunks(out.Body, maxTranslateChunk)) {
		t.Errorf("body only made %d calls", calls-before)
	}

	// only=title / only=summary: one short call, just that field.
	for _, field := range []string{"title", "summary"} {
		before = calls
		res = send(t, app, "POST", "/admin/translate", url.Values{"slug": {"hello-world"}, "from": {"en"}, "to": {"id"}, "only": {field}}, session)
		out = translation{}
		_ = json.Unmarshal([]byte(res.body), &out)
		got := map[string]string{"title": out.Title, "summary": out.Summary}
		if res.code != 200 || calls-before != 1 || got[field] == "" || out.Body != "" || (field == "title" && out.Summary != "") || (field == "summary" && out.Title != "") {
			t.Errorf("only %s: %d %s (%d calls)", field, res.code, res.body, calls-before)
		}
	}
	if res := send(t, app, "POST", "/admin/translate", url.Values{"slug": {"hello-world"}, "from": {"en"}, "to": {"id"}, "only": {"tags"}}, session); res.code != 400 {
		t.Errorf("only=tags: %d", res.code)
	}

	for _, bad := range []url.Values{
		{"slug": {"hello-world"}, "from": {"en"}, "to": {"en"}},
		{"slug": {"hello-world"}, "from": {"fr"}, "to": {"id"}},
		{"slug": {"nope"}, "from": {"en"}, "to": {"id"}},
	} {
		if res := send(t, app, "POST", "/admin/translate", bad, session); res.code < 400 {
			t.Errorf("%v: %d", bad, res.code)
		}
	}
	if res := send(t, app, "POST", "/admin/translate", url.Values{"slug": {"hello-world"}, "from": {"en"}, "to": {"id"}}); res.code == 200 {
		t.Error("translated without a session")
	}
}
