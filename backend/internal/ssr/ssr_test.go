package ssr_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/michaelputong/blog/backend/internal/ssr"
	"github.com/michaelputong/blog/backend/internal/web"
)

func newRenderer(t *testing.T) *ssr.Renderer {
	t.Helper()
	bundle, err := web.ServerBundle()
	if err != nil {
		t.Skipf("frontend not built (run npm run build in frontend/): %v", err)
	}
	r, err := ssr.New(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRenderPost(t *testing.T) {
	r := newRenderer(t)
	res, err := r.Render(map[string]any{
		"page": "post",
		"lang": "id",
		"post": map[string]any{
			"slug": "hello-world", "lang": "id", "title": "Halo <dunia>",
			"summary": "Ringkasan", "body": "Uno\n\nDos", "publishedAt": "2026-09-20T09:00:00Z",
		},
		"availableLanguages": []string{"en", "id"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`class="page-title">Halo &lt;dunia&gt;</h1>`, // React escapes content
		"<p>Uno</p><p>Dos</p>",
		`href="/en/posts/hello-world"`,
		"20 September 2026", // dictionary-based date formatting
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("html missing %q:\n%s", want, res.HTML)
		}
	}
	if res.Title != "Halo <dunia> – GoBlog.dev" || res.Description != "Ringkasan" {
		t.Errorf("head = %q / %q", res.Title, res.Description)
	}
}

func TestRenderConcurrent(t *testing.T) {
	r := newRenderer(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := r.Render(map[string]any{"page": "notFound", "lang": "en"})
			if err != nil || !strings.Contains(res.HTML, "Page not found") {
				t.Errorf("render: %v %q", err, res.HTML)
			}
		}()
	}
	wg.Wait()
}
