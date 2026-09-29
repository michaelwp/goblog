package posts

import (
	"slices"
	"sort"
)

// RelatedLimit is how many suggestions an article page shows.
const RelatedLimit = 5

// Related picks up to n of candidates to suggest alongside p: those sharing
// the most tags with it, where each shared tag counts twice as much as being
// in the same category. Candidates with nothing in common are left out, as is
// p itself. Ties keep the candidates' order, so pass them newest first.
func Related(p Post, candidates []Post, n int) []Post {
	type scored struct {
		post  Post
		score int
	}
	var matches []scored
	for _, c := range candidates {
		if c.Slug == p.Slug {
			continue
		}
		score := 0
		for _, tag := range c.Tags {
			if slices.Contains(p.Tags, tag) {
				score += 2
			}
		}
		if p.Category != "" && c.Category == p.Category {
			score++
		}
		if score > 0 {
			matches = append(matches, scored{c, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })

	out := make([]Post, 0, min(n, len(matches)))
	for _, m := range matches[:min(n, len(matches))] {
		out = append(out, m.post)
	}
	return out
}
