// Package categories stores the blog's article categories: a unique slug
// plus a unique name in each language ("security" → Security / Keamanan).
package categories

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	ErrNotFound  = errors.New("category not found")
	ErrSlugTaken = errors.New("a category with this slug already exists")
)

// NameTakenError reports a name already used by another category.
type NameTakenError struct{ Lang, Name, By string }

func (e *NameTakenError) Error() string {
	return fmt.Sprintf("the %s name %q is already used by category %q", e.Lang, e.Name, e.By)
}

type Category struct {
	Slug      string            `json:"slug" bson:"slug"`
	Names     map[string]string `json:"names" bson:"names"` // by language
	CreatedAt time.Time         `json:"createdAt" bson:"createdAt"`
}

// Name returns the category's name in lang, falling back to any name, then
// the slug.
func (c Category) Name(lang string) string {
	if n := c.Names[lang]; n != "" {
		return n
	}
	for _, n := range c.Names {
		if n != "" {
			return n
		}
	}
	return c.Slug
}

type Store interface {
	// List returns every category, sorted by name in lang.
	List(ctx context.Context, lang string) ([]Category, error)
	Get(ctx context.Context, slug string) (Category, error)
	// Create adds c; it fails with *NameTakenError or ErrSlugTaken (names are
	// checked first).
	Create(ctx context.Context, c Category) error
	// Rename replaces the names of the category with c.Slug; it fails with
	// ErrNotFound or *NameTakenError.
	Rename(ctx context.Context, c Category) error
	Delete(ctx context.Context, slug string) error
}

// nameConflict finds a category that already uses one of c's names in the
// same language, ignoring case. When renaming, self is c's own slug, so a
// category may keep its current name; when creating, self is "".
func nameConflict(existing []Category, c Category, self string) error {
	for _, other := range existing {
		if self != "" && other.Slug == self {
			continue
		}
		for lang, name := range c.Names {
			if name != "" && strings.EqualFold(strings.TrimSpace(other.Names[lang]), strings.TrimSpace(name)) {
				return &NameTakenError{Lang: lang, Name: name, By: other.Slug}
			}
		}
	}
	return nil
}

func sortByName(cs []Category, lang string) {
	sort.Slice(cs, func(i, j int) bool {
		a, b := strings.ToLower(cs[i].Name(lang)), strings.ToLower(cs[j].Name(lang))
		if a != b {
			return a < b
		}
		return cs[i].Slug < cs[j].Slug
	})
}

// ---- MongoDB ---------------------------------------------------------------

type MongoStore struct {
	coll *mongo.Collection
	mu   sync.Mutex // serializes the check-then-write of name uniqueness
}

func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{coll: db.Collection("categories")}
}

// EnsureIndexes makes slugs unique, and names unique per language ignoring
// case, so duplicates are refused even by concurrent writers.
func (s *MongoStore) EnsureIndexes(ctx context.Context, languages []string) error {
	models := []mongo.IndexModel{{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)}}
	for _, lang := range languages {
		models = append(models, mongo.IndexModel{
			Keys: bson.D{{Key: "names." + lang, Value: 1}},
			Options: options.Index().SetUnique(true).
				// Strength 2 ignores case; the "simple" locale would ignore strength.
				SetCollation(&options.Collation{Locale: "en", Strength: 2}).
				SetPartialFilterExpression(bson.D{{Key: "names." + lang, Value: bson.D{{Key: "$type", Value: "string"}, {Key: "$gt", Value: ""}}}}),
		})
	}
	_, err := s.coll.Indexes().CreateMany(ctx, models)
	return err
}

func (s *MongoStore) List(ctx context.Context, lang string) ([]Category, error) {
	cur, err := s.coll.Find(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	out := []Category{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode categories: %w", err)
	}
	sortByName(out, lang)
	return out, nil
}

func (s *MongoStore) Get(ctx context.Context, slug string) (Category, error) {
	var c Category
	err := s.coll.FindOne(ctx, bson.D{{Key: "slug", Value: slug}}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Category{}, ErrNotFound
	}
	if err != nil {
		return Category{}, fmt.Errorf("get category: %w", err)
	}
	return c, nil
}

func (s *MongoStore) checkNames(ctx context.Context, c Category, self string) error {
	all, err := s.List(ctx, "")
	if err != nil {
		return err
	}
	return nameConflict(all, c, self)
}

func (s *MongoStore) Create(ctx context.Context, c Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Names first: "that name is taken" explains more than a clashing slug
	// (which is usually derived from the name).
	if err := s.checkNames(ctx, c, ""); err != nil {
		return err
	}
	if _, err := s.Get(ctx, c.Slug); err == nil {
		return ErrSlugTaken
	}
	_, err := s.coll.InsertOne(ctx, c)
	if mongo.IsDuplicateKeyError(err) {
		return ErrSlugTaken // raced with another writer
	}
	if err != nil {
		return fmt.Errorf("create category: %w", err)
	}
	return nil
}

func (s *MongoStore) Rename(ctx context.Context, c Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkNames(ctx, c, c.Slug); err != nil {
		return err
	}
	res, err := s.coll.UpdateOne(ctx, bson.D{{Key: "slug", Value: c.Slug}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "names", Value: c.Names}}}})
	if err != nil {
		return fmt.Errorf("rename category: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MongoStore) Delete(ctx context.Context, slug string) error {
	res, err := s.coll.DeleteOne(ctx, bson.D{{Key: "slug", Value: slug}})
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Memory ----------------------------------------------------------------

// MemoryStore is an in-memory Store for tests.
type MemoryStore struct {
	mu   sync.Mutex
	cats map[string]Category
}

func (s *MemoryStore) List(_ context.Context, lang string) ([]Category, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Category{}
	for _, c := range s.cats {
		out = append(out, c)
	}
	sortByName(out, lang)
	return out, nil
}

func (s *MemoryStore) Get(_ context.Context, slug string) (Category, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cats[slug]
	if !ok {
		return Category{}, ErrNotFound
	}
	return c, nil
}

func (s *MemoryStore) all() []Category {
	out := []Category{}
	for _, c := range s.cats {
		out = append(out, c)
	}
	return out
}

func (s *MemoryStore) Create(_ context.Context, c Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := nameConflict(s.all(), c, ""); err != nil {
		return err
	}
	if _, ok := s.cats[c.Slug]; ok {
		return ErrSlugTaken
	}
	if s.cats == nil {
		s.cats = map[string]Category{}
	}
	s.cats[c.Slug] = c
	return nil
}

func (s *MemoryStore) Rename(_ context.Context, c Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.cats[c.Slug]
	if !ok {
		return ErrNotFound
	}
	if err := nameConflict(s.all(), c, c.Slug); err != nil {
		return err
	}
	existing.Names = c.Names
	s.cats[c.Slug] = existing
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cats[slug]; !ok {
		return ErrNotFound
	}
	delete(s.cats, slug)
	return nil
}
