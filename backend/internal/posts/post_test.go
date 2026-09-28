package posts

import (
	"testing"
	"time"
)

func TestStatusValid(t *testing.T) {
	for s, want := range map[Status]bool{Draft: true, Published: true, Disabled: true, "": false, "archived": false} {
		if s.Valid() != want {
			t.Errorf("%q.Valid() = %v", s, !want)
		}
	}
}

func TestApplyDraftAndEditing(t *testing.T) {
	live := Post{Title: "Live", Summary: "S", Body: "B", PublishedAt: time.Unix(1, 0), Status: Published, Category: "go", Tags: []string{"x"}}

	// Without a draft, both are no-ops.
	if e := live.Editing(); e.Title != "Live" || e.Draft != nil {
		t.Errorf("Editing without draft = %+v", e)
	}
	p := live
	p.ApplyDraft()
	if p.Title != "Live" {
		t.Error("ApplyDraft without draft changed the post")
	}

	live.Draft = &Revision{Title: "New", Summary: "S2", Body: "B2", PublishedAt: time.Unix(2, 0)}
	e := live.Editing()
	if e.Title != "New" || e.Body != "B2" || !e.PublishedAt.Equal(time.Unix(2, 0)) || e.Draft != nil {
		t.Errorf("Editing = %+v", e)
	}
	if e.Status != Published || e.Category != "go" || len(e.Tags) != 1 {
		t.Error("Editing must keep status, category and tags")
	}
	if live.Title != "Live" || live.Draft == nil {
		t.Error("Editing must not change the original")
	}
	live.ApplyDraft()
	if live.Title != "New" || live.Draft != nil {
		t.Errorf("after ApplyDraft: %+v", live)
	}
}

func TestNormalizeTag(t *testing.T) {
	for in, want := range map[string]string{
		" Go ":               "go",
		"#Security":          "security",
		"Node   JS":          "node-js",
		"--a--b--":           "a-b",
		"C#":                 "c#",
		"Jaringan  Komputer": "jaringan-komputer",
		"   ":                "",
	} {
		if got := NormalizeTag(in); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", in, got, want)
		}
	}
}
