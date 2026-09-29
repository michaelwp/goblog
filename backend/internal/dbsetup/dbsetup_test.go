package dbsetup

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"github.com/michaelputong/blog/backend/internal/auth"
	"github.com/michaelputong/blog/backend/internal/categories"
	"github.com/michaelputong/blog/backend/internal/media"
	"github.com/michaelputong/blog/backend/internal/posts"
	"github.com/michaelputong/blog/backend/internal/profile"
)

// Runs against a real MongoDB when MONGODB_TEST_URI is set. It checks that the
// validators accept everything the stores write and reject malformed documents.
func TestMongoSetup(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("MONGODB_TEST_URI not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database(fmt.Sprintf("blog_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })

	languages := []string{"en", "id"}
	first, err := Setup(ctx, db, languages)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range first {
		if !r.Created {
			t.Errorf("first run: %s not created", r.Collection)
		}
	}
	// A second run updates the validators in place.
	second, err := Setup(ctx, db, languages)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	for _, r := range second {
		if r.Created {
			t.Errorf("second run: %s created again", r.Collection)
		}
	}

	names, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"posts", "categories", "profile", "admin", "media.files", "media.chunks"} {
		if !slices.Contains(names, want) {
			t.Errorf("collection %s missing (have %v)", want, names)
		}
	}

	// Everything the stores write must pass validation.
	ps := posts.NewMongoStore(db)
	if err := ps.SeedIfEmpty(ctx, posts.SeedPosts()); err != nil {
		t.Fatalf("seed posts: %v", err)
	}
	draft := posts.Post{Slug: "new", Lang: "en", Title: "New", PublishedAt: time.Now(), Status: posts.Draft}
	if err := ps.Create(ctx, draft); err != nil {
		t.Fatalf("create post with nil tags: %v", err)
	}
	if err := ps.SetMeta(ctx, "new", "go", []string{"a", "b"}); err != nil {
		t.Fatalf("set meta: %v", err)
	}
	draft.Status = posts.Published
	draft.Draft = &posts.Revision{Title: "Edit", PublishedAt: time.Now(), SavedAt: time.Now()}
	if err := ps.Update(ctx, draft); err != nil {
		t.Fatalf("update post with draft: %v", err)
	}

	cs := categories.NewMongoStore(db)
	if err := cs.Create(ctx, categories.Category{Slug: "go", Names: map[string]string{"en": "Go"}, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("create category: %v", err)
	}

	pr := profile.NewMongoStore(db)
	if err := pr.Save(ctx, profile.Profile{Name: "Owner", UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("save empty profile: %v", err)
	}
	if err := pr.Save(ctx, profile.Profile{
		Name: "Owner", Links: []profile.Link{{Label: "GitHub", URL: "https://github.com"}},
		Texts: map[string]profile.Text{"en": {Headline: "Hi", Bio: "Bio"}}, UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save full profile: %v", err)
	}

	cred, err := auth.NewCredential("a strong password", bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	as := auth.NewMongoStore(db)
	if err := as.Create(ctx, cred); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if cred, err = cred.WithPassword("another password", bcrypt.MinCost); err != nil {
		t.Fatal(err)
	}
	if err := as.Update(ctx, cred); err != nil {
		t.Fatalf("update admin: %v", err)
	}

	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	if _, err := media.Upload(ctx, media.NewMongoStore(db), "a.png", png); err != nil {
		t.Fatalf("upload image: %v", err)
	}

	// Malformed documents are refused.
	bad := map[string]bson.D{
		"posts":      {{Key: "slug", Value: "x"}, {Key: "lang", Value: "fr"}},
		"categories": {{Key: "slug", Value: 42}},
		"admin":      {{Key: "_id", Value: "admin2"}},
	}
	for coll, doc := range bad {
		if _, err := db.Collection(coll).InsertOne(ctx, doc); err == nil {
			t.Errorf("%s accepted invalid document %v", coll, doc)
		}
	}
}
