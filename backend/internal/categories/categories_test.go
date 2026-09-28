package categories

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func cat(slug, en, id string) Category {
	return Category{Slug: slug, Names: map[string]string{"en": en, "id": id}, CreatedAt: time.Now().UTC().Truncate(time.Millisecond)}
}

func testStore(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	for _, c := range []Category{cat("security", "Security", "Keamanan"), cat("web", "Web", "Web"), cat("go", "Go", "Go")} {
		if err := s.Create(ctx, c); err != nil {
			t.Fatalf("Create %s: %v", c.Slug, err)
		}
	}

	// Unique slug, and unique names per language ignoring case.
	if err := s.Create(ctx, cat("security", "Safety", "Aman")); !errors.Is(err, ErrSlugTaken) { // new names, taken slug
		t.Errorf("duplicate slug: err = %v", err)
	}
	// Same slug and same name: the name clash is what's reported.
	var sameName *NameTakenError
	if err := s.Create(ctx, cat("security", "security", "Aman")); !errors.As(err, &sameName) {
		t.Errorf("duplicate slug and name: err = %v, want *NameTakenError", err)
	}
	var taken *NameTakenError
	if err := s.Create(ctx, cat("sec", "SECURITY", "Lain")); !errors.As(err, &taken) || taken.Lang != "en" || taken.By != "security" {
		t.Errorf("duplicate English name: err = %v", err)
	}
	if err := s.Create(ctx, cat("safety", "Safety", "keamanan")); !errors.As(err, &taken) || taken.Lang != "id" {
		t.Errorf("duplicate Indonesian name: err = %v", err)
	}
	// The same name in different languages is fine ("Web" / "Web").
	if c, _ := s.Get(ctx, "web"); c.Names["id"] != "Web" {
		t.Errorf("Get(web) = %+v", c)
	}

	// Listing is sorted by name in the requested language.
	list, _ := s.List(ctx, "id")
	if len(list) != 3 || list[0].Slug != "go" || list[1].Slug != "security" || list[2].Slug != "web" {
		t.Errorf("List(id) order: %v", []string{list[0].Slug, list[1].Slug, list[2].Slug})
	}

	// Renaming checks uniqueness against the others, but not itself.
	if err := s.Rename(ctx, cat("security", "Security", "Keamanan Siber")); err != nil {
		t.Errorf("rename: %v", err)
	}
	if err := s.Rename(ctx, cat("web", "Go", "Web")); !errors.As(err, &taken) {
		t.Errorf("rename to a taken name: err = %v", err)
	}
	if err := s.Rename(ctx, cat("missing", "X", "Y")); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename missing: err = %v", err)
	}

	if err := s.Delete(ctx, "go"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "go"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: err = %v", err)
	}
}

func TestMemoryStore(t *testing.T) { testStore(t, &MemoryStore{}) }

func TestMongoStore(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("MONGODB_TEST_URI not set")
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database(fmt.Sprintf("blog_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	s := NewMongoStore(db)
	if err := s.EnsureIndexes(context.Background(), []string{"en", "id"}); err != nil {
		t.Fatal(err)
	}
	testStore(t, s)

	// The database itself refuses a case-insensitive duplicate name, even if
	// the application check is bypassed.
	_, err = db.Collection("categories").InsertOne(context.Background(), cat("dup", "sEcUrItY", "Beda"))
	if !mongo.IsDuplicateKeyError(err) {
		t.Errorf("direct insert of a duplicate name: err = %v, want duplicate key", err)
	}
}
