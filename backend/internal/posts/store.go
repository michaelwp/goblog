package posts

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("post not found")
	ErrExists   = errors.New("a post with this slug and language already exists")
)

// Post is a single translation of a blog post. Translations of the same
// article share a Slug and differ by Lang.
type Post struct {
	Slug        string    `json:"slug" bson:"slug"`
	Lang        string    `json:"lang" bson:"lang"`
	Title       string    `json:"title" bson:"title"`
	Subtitle    string    `json:"subtitle" bson:"subtitle,omitempty"` // optional line under the title
	Summary     string    `json:"summary" bson:"summary"`
	Body        string    `json:"body" bson:"body"`
	PublishedAt time.Time `json:"publishedAt" bson:"publishedAt"`
	Status      Status    `json:"status" bson:"status"`
	// Category and Tags describe the whole article: they are kept the same
	// on every translation (see SetMeta) and are not part of Draft.
	Category string   `json:"category" bson:"category"` // category slug, or "" for none
	Tags     []string `json:"tags" bson:"tags"`
	// Cover is the article's optional cover image: an uploaded /media/ path
	// or an https:// address. Like Category it is shared by every
	// translation (see SetCover) and applies right away, not via Draft.
	Cover string `json:"cover" bson:"cover,omitempty"`
	// Draft holds unpublished edits to a Published post. Readers keep seeing
	// the fields above until the draft is published (ApplyDraft). Only
	// Published posts carry one; drafts and disabled posts are edited in place.
	Draft *Revision `json:"draft,omitempty" bson:"draft,omitempty"`
}

// Revision is a pending edit of a published post.
type Revision struct {
	Title       string    `json:"title" bson:"title"`
	Subtitle    string    `json:"subtitle" bson:"subtitle,omitempty"`
	Summary     string    `json:"summary" bson:"summary"`
	Body        string    `json:"body" bson:"body"`
	PublishedAt time.Time `json:"publishedAt" bson:"publishedAt"`
	SavedAt     time.Time `json:"savedAt" bson:"savedAt"`
}

// ApplyDraft copies pending edits into the post itself and clears them.
func (p *Post) ApplyDraft() {
	if p.Draft == nil {
		return
	}
	p.Title, p.Subtitle, p.Summary, p.Body, p.PublishedAt = p.Draft.Title, p.Draft.Subtitle, p.Draft.Summary, p.Draft.Body, p.Draft.PublishedAt
	p.Draft = nil
}

// Editing returns the version an editor works on: the pending draft when
// there is one, otherwise the post itself.
func (p Post) Editing() Post {
	if p.Draft != nil {
		p.ApplyDraft()
	}
	return p
}

// Status controls whether a translation is visible on the public site.
type Status string

const (
	Draft     Status = "draft"     // not yet published; admin only
	Published Status = "published" // live
	Disabled  Status = "disabled"  // taken down; admin only, can be republished
)

func (s Status) Valid() bool { return s == Draft || s == Published || s == Disabled }

// Repository is the storage the API depends on. MongoStore is the production
// implementation; MemoryStore backs the tests.
//
// List, Languages and Search only see Published posts, since they feed the
// public site, and their results never include pending Drafts. Get and All return posts of any status; public callers must
// check Status themselves.
type Repository interface {
	// List returns the published posts in lang, newest first.
	List(ctx context.Context, lang string) ([]Post, error)
	// Get returns the post with slug in lang, whatever its status, or ErrNotFound.
	Get(ctx context.Context, slug, lang string) (Post, error)
	// Languages returns the languages slug is published in, sorted.
	Languages(ctx context.Context, slug string) ([]string, error)
	// Find returns published posts matching every set field of f, newest
	// first, at most SearchLimit.
	Find(ctx context.Context, f Filter) ([]Post, error)
	// TagCounts returns how many published posts in lang use each tag,
	// most used first.
	TagCounts(ctx context.Context, lang string) ([]TagCount, error)
	// CategoryCounts returns how many published posts in lang each
	// category has ("" counts posts without one).
	CategoryCounts(ctx context.Context, lang string) (map[string]int, error)

	// All returns every post in every language and status, newest first.
	All(ctx context.Context) ([]Post, error)
	// Create adds p, or returns ErrExists if its slug and language are taken.
	Create(ctx context.Context, p Post) error
	// Update replaces the title, subtitle, summary, body, publish date, status and
	// pending draft of the post with p's slug and language, or returns ErrNotFound.
	Update(ctx context.Context, p Post) error
	// Delete removes one translation, or returns ErrNotFound.
	Delete(ctx context.Context, slug, lang string) error
	// DeleteAll removes every translation of slug and reports how many.
	DeleteAll(ctx context.Context, slug string) (int, error)
	// SetMeta sets the category and tags of every translation of slug.
	SetMeta(ctx context.Context, slug, category string, tags []string) error
	// SetCover sets the cover image of every translation of slug ("" for none).
	SetCover(ctx context.Context, slug, cover string) error
	// ReassignCategory moves every post in category from to category to
	// ("" for none) and reports how many translations changed.
	ReassignCategory(ctx context.Context, from, to string) (int, error)
}

// Filter selects published posts. Lang is required; Text matches title,
// summary or body (case-insensitive); Tag and Category must match exactly.
type Filter struct {
	Lang, Text, Tag, Category string
}

func (f Filter) matches(p Post) bool {
	if p.Lang != f.Lang || p.Status != Published {
		return false
	}
	if f.Category != "" && p.Category != f.Category {
		return false
	}
	if f.Tag != "" && !slices.Contains(p.Tags, f.Tag) {
		return false
	}
	if f.Text == "" {
		return true
	}
	q := strings.ToLower(f.Text)
	for _, field := range []string{p.Title, p.Summary, p.Body} {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	return false
}

type TagCount struct {
	Tag   string `json:"tag" bson:"_id"`
	Count int    `json:"count" bson:"count"`
}

const SearchLimit = 100

// MemoryStore is a concurrency-safe in-memory Repository.
type MemoryStore struct {
	mu    sync.RWMutex
	posts map[string]map[string]Post // slug -> lang -> post
}

func NewMemoryStore(seed []Post) *MemoryStore {
	s := &MemoryStore{posts: make(map[string]map[string]Post)}
	for _, p := range seed {
		if p.Status == "" {
			p.Status = Published
		}
		if s.posts[p.Slug] == nil {
			s.posts[p.Slug] = make(map[string]Post)
		}
		s.posts[p.Slug][p.Lang] = p
	}
	return s
}

func (s *MemoryStore) List(_ context.Context, lang string) ([]Post, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := []Post{}
	for _, translations := range s.posts {
		if p, ok := translations[lang]; ok && p.Status == Published {
			p.Draft = nil
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PublishedAt.After(out[j].PublishedAt)
	})
	return out, nil
}

func (s *MemoryStore) Get(_ context.Context, slug, lang string) (Post, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.posts[slug][lang]
	if !ok {
		return Post{}, ErrNotFound
	}
	return p, nil
}

func (s *MemoryStore) Languages(_ context.Context, slug string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	langs := []string{}
	for lang, p := range s.posts[slug] {
		if p.Status == Published {
			langs = append(langs, lang)
		}
	}
	sort.Strings(langs)
	return langs, nil
}

func (s *MemoryStore) Find(ctx context.Context, f Filter) ([]Post, error) {
	all, _ := s.List(ctx, f.Lang)
	out := []Post{}
	for _, p := range all {
		if len(out) == SearchLimit {
			break
		}
		if f.matches(p) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *MemoryStore) TagCounts(ctx context.Context, lang string) ([]TagCount, error) {
	all, _ := s.List(ctx, lang)
	counts := map[string]int{}
	for _, p := range all {
		for _, t := range p.Tags {
			counts[t]++
		}
	}
	return sortTagCounts(counts), nil
}

func sortTagCounts(counts map[string]int) []TagCount {
	out := []TagCount{}
	for t, n := range counts {
		out = append(out, TagCount{Tag: t, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

func (s *MemoryStore) CategoryCounts(ctx context.Context, lang string) (map[string]int, error) {
	all, _ := s.List(ctx, lang)
	counts := map[string]int{}
	for _, p := range all {
		counts[p.Category]++
	}
	return counts, nil
}

func (s *MemoryStore) SetMeta(_ context.Context, slug, category string, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for lang, p := range s.posts[slug] {
		p.Category, p.Tags = category, slices.Clone(tags)
		s.posts[slug][lang] = p
	}
	return nil
}

func (s *MemoryStore) SetCover(_ context.Context, slug, cover string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for lang, p := range s.posts[slug] {
		p.Cover = cover
		s.posts[slug][lang] = p
	}
	return nil
}

func (s *MemoryStore) ReassignCategory(_ context.Context, from, to string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for slug, translations := range s.posts {
		for lang, p := range translations {
			if p.Category == from {
				p.Category = to
				s.posts[slug][lang] = p
				n++
			}
		}
	}
	return n, nil
}

func (s *MemoryStore) All(_ context.Context) ([]Post, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := []Post{}
	for _, translations := range s.posts {
		for _, p := range translations {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].PublishedAt.Equal(out[j].PublishedAt) {
			return out[i].PublishedAt.After(out[j].PublishedAt)
		}
		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}
		return out[i].Lang < out[j].Lang
	})
	return out, nil
}

func (s *MemoryStore) Create(_ context.Context, p Post) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.posts[p.Slug][p.Lang]; ok {
		return ErrExists
	}
	if s.posts[p.Slug] == nil {
		s.posts[p.Slug] = make(map[string]Post)
	}
	s.posts[p.Slug][p.Lang] = p
	return nil
}

func (s *MemoryStore) Update(_ context.Context, p Post) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.posts[p.Slug][p.Lang]; !ok {
		return ErrNotFound
	}
	s.posts[p.Slug][p.Lang] = p
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, slug, lang string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.posts[slug][lang]; !ok {
		return ErrNotFound
	}
	delete(s.posts[slug], lang)
	if len(s.posts[slug]) == 0 {
		delete(s.posts, slug)
	}
	return nil
}

func (s *MemoryStore) DeleteAll(_ context.Context, slug string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := len(s.posts[slug])
	delete(s.posts, slug)
	return n, nil
}
