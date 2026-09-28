package posts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Runs against a real MongoDB when MONGODB_TEST_URI is set, e.g.
// MONGODB_TEST_URI=mongodb://localhost:27017 go test ./...
func TestMongoStore(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("MONGODB_TEST_URI not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	db := client.Database(fmt.Sprintf("blog_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })

	s := NewMongoStore(db)
	if err := s.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedIfEmpty(ctx, SeedPosts()); err != nil {
		t.Fatal(err)
	}
	// A second seed must not duplicate documents.
	if err := s.SeedIfEmpty(ctx, SeedPosts()); err != nil {
		t.Fatal(err)
	}

	list, err := s.List(ctx, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Slug != "why-ssr" {
		t.Errorf("List(en) = %+v, want 2 posts newest first", list)
	}

	p, err := s.Get(ctx, "hello-world", "id")
	if err != nil || p.Title != "Halo, dunia" {
		t.Errorf("Get(hello-world, id) = %+v, %v", p, err)
	}
	if _, err := s.Get(ctx, "why-ssr", "id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(why-ssr, id) err = %v, want ErrNotFound", err)
	}

	for query, want := range map[string]int{"SERVER": 2, "a.b(": 0, "dunia": 0} {
		got, err := s.Find(ctx, Filter{Lang: "en", Text: query})
		if err != nil || len(got) != want {
			t.Errorf("Search(en, %q) = %d posts, %v; want %d", query, len(got), err, want)
		}
	}

	langs, err := s.Languages(ctx, "hello-world")
	if err != nil || !slices.Equal(langs, []string{"en", "id"}) {
		t.Errorf("Languages = %v, %v", langs, err)
	}

	testWrites(t, s)
	testStatuses(t, s)
	testDraftRevision(t, s)

	// Documents written before statuses existed become published.
	if _, err := s.coll.InsertOne(ctx, bson.D{{Key: "slug", Value: "legacy"}, {Key: "lang", Value: "en"}, {Key: "title", Value: "Old"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Get(ctx, "legacy", "en"); p.Status != Published {
		t.Errorf("legacy post status after Migrate = %q", p.Status)
	}
}

// Runs on its own freshly seeded database: the tests above change the seed.
func TestMongoStoreTaxonomy(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("MONGODB_TEST_URI not set")
	}
	ctx := context.Background()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database(fmt.Sprintf("blog_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	s := NewMongoStore(db)
	if err := s.SeedIfEmpty(ctx, SeedPosts()); err != nil {
		t.Fatal(err)
	}
	testTaxonomy(t, s)
}
