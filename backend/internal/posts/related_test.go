package posts

import (
	"slices"
	"testing"
)

func TestRelated(t *testing.T) {
	p := Post{Slug: "passkeys", Category: "security", Tags: []string{"auth", "web"}}
	candidates := []Post{ // newest first
		{Slug: "passkeys"}, // the article itself
		{Slug: "same-category", Category: "security"},
		{Slug: "one-tag", Tags: []string{"web"}},
		{Slug: "unrelated", Category: "go", Tags: []string{"generics"}},
		{Slug: "two-tags", Tags: []string{"auth", "web"}},
		{Slug: "tag-and-category", Category: "security", Tags: []string{"auth"}},
		{Slug: "older-one-tag", Tags: []string{"auth"}},
	}
	slugs := func(ps []Post) []string {
		var s []string
		for _, p := range ps {
			s = append(s, p.Slug)
		}
		return s
	}

	// Two shared tags (4) beat a tag and the category (3), then one tag (2,
	// newest first), then only the category (1).
	want := []string{"two-tags", "tag-and-category", "one-tag", "older-one-tag", "same-category"}
	if got := slugs(Related(p, candidates, 10)); !slices.Equal(got, want) {
		t.Errorf("Related = %v, want %v", got, want)
	}
	if got := slugs(Related(p, candidates, 2)); !slices.Equal(got, want[:2]) {
		t.Errorf("Related(n=2) = %v, want %v", got, want[:2])
	}

	// An article without a category doesn't match others without one.
	if got := Related(Post{Slug: "x"}, []Post{{Slug: "y"}}, 5); len(got) != 0 {
		t.Errorf("Related with nothing in common = %v, want none", slugs(got))
	}
}
