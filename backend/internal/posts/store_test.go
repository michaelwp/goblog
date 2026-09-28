package posts

import (
	"context"
	"testing"
)

func TestMemoryStoreSearch(t *testing.T) {
	s := NewMemoryStore(SeedPosts())
	cases := []struct {
		lang, query string
		want        int
	}{
		{"en", "SERVER", 2}, // case-insensitive, matches title and body
		{"en", "dunia", 0},  // other language's content isn't searched
		{"id", "dunia", 1},
		{"en", "a.b(", 0}, // regex metacharacters are literal
	}
	for _, c := range cases {
		got, err := s.Search(context.Background(), c.lang, c.query)
		if err != nil || len(got) != c.want {
			t.Errorf("Search(%s, %q) = %d posts, %v; want %d", c.lang, c.query, len(got), err, c.want)
		}
	}
}
