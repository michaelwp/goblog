package posts

import (
	"context"
	"errors"
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
	Summary     string    `json:"summary" bson:"summary"`
	Body        string    `json:"body" bson:"body"`
	PublishedAt time.Time `json:"publishedAt" bson:"publishedAt"`
	Status      Status    `json:"status" bson:"status"`
	// Draft holds unpublished edits to a Published post. Readers keep seeing
	// the fields above until the draft is published (ApplyDraft). Only
	// Published posts carry one; drafts and disabled posts are edited in place.
	Draft *Revision `json:"draft,omitempty" bson:"draft,omitempty"`
}

// Revision is a pending edit of a published post.
type Revision struct {
	Title       string    `json:"title" bson:"title"`
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
	p.Title, p.Summary, p.Body, p.PublishedAt = p.Draft.Title, p.Draft.Summary, p.Draft.Body, p.Draft.PublishedAt
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
	// Search returns published posts in lang whose title, summary or body
	// contains query (case-insensitive), newest first, at most SearchLimit.
	Search(ctx context.Context, lang, query string) ([]Post, error)

	// All returns every post in every language and status, newest first.
	All(ctx context.Context) ([]Post, error)
	// Create adds p, or returns ErrExists if its slug and language are taken.
	Create(ctx context.Context, p Post) error
	// Update replaces the title, summary, body, publish date, status and
	// pending draft of the post with p's slug and language, or returns ErrNotFound.
	Update(ctx context.Context, p Post) error
	// Delete removes one translation, or returns ErrNotFound.
	Delete(ctx context.Context, slug, lang string) error
	// DeleteAll removes every translation of slug and reports how many.
	DeleteAll(ctx context.Context, slug string) (int, error)
}

const SearchLimit = 50

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

func (s *MemoryStore) Search(ctx context.Context, lang, query string) ([]Post, error) {
	all, _ := s.List(ctx, lang)
	q := strings.ToLower(query)
	out := []Post{}
	for _, p := range all {
		if len(out) == SearchLimit {
			break
		}
		for _, field := range []string{p.Title, p.Summary, p.Body} {
			if strings.Contains(strings.ToLower(field), q) {
				out = append(out, p)
				break
			}
		}
	}
	return out, nil
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
