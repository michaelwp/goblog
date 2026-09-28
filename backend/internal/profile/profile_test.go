package profile

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func sample() Profile {
	return Profile{
		Name: "Ada", PhotoURL: "https://example.com/ada.jpg", Location: "Jakarta", Email: "ada@example.com",
		Links: []Link{{Label: "GitHub", URL: "https://github.com/ada"}},
		Texts: map[string]Text{
			"en": {Headline: "Writes about Go.", Bio: "Hello."},
			"id": {Headline: "Menulis tentang Go.", Bio: "Halo."},
		},
		UpdatedAt: time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC),
	}
}

func testStore(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	if p, err := s.Get(ctx); err != nil || p.Name != "" {
		t.Fatalf("empty store: Get = %+v, %v; want zero profile", p, err)
	}
	want := sample()
	if err := s.Save(ctx, want); err != nil {
		t.Fatal(err)
	}
	want.Name = "Ada Lovelace" // saving again replaces
	if err := s.Save(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Get = %+v, %v\nwant %+v", got, err, want)
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
	testStore(t, NewMongoStore(db))
}
