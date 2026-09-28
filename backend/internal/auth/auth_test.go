package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

func TestCredential(t *testing.T) {
	c, err := NewCredential("first password", bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(c.PasswordHash, []byte("first password")) {
		t.Fatal("password stored in plain text")
	}
	if !c.PasswordMatches("first password") || c.PasswordMatches("wrong") {
		t.Error("PasswordMatches")
	}
	changed, err := c.WithPassword("second password", bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Epoch != c.Epoch+1 || !bytes.Equal(changed.SessionKey, c.SessionKey) {
		t.Errorf("WithPassword: epoch %d→%d, key kept %v", c.Epoch, changed.Epoch, bytes.Equal(changed.SessionKey, c.SessionKey))
	}
	if changed.PasswordMatches("first password") || !changed.PasswordMatches("second password") {
		t.Error("password not changed")
	}
}

func testStore(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	if _, ok, err := s.Get(ctx); ok || err != nil {
		t.Fatalf("empty store: ok=%v err=%v", ok, err)
	}
	c, _ := NewCredential("password one", bcrypt.MinCost)
	if err := s.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, c); !errors.Is(err, ErrExists) {
		t.Errorf("second Create: err = %v, want ErrExists", err)
	}
	c2, _ := c.WithPassword("password two", bcrypt.MinCost)
	if err := s.Update(ctx, c2); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get(ctx)
	if err != nil || !ok || got.Epoch != 2 || !got.PasswordMatches("password two") || !bytes.Equal(got.SessionKey, c.SessionKey) {
		t.Errorf("after Update: %+v ok=%v err=%v", got, ok, err)
	}
	if err := s.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get(ctx); ok {
		t.Error("credential still present after Delete")
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
