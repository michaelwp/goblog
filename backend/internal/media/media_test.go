package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// pngBytes returns a small valid PNG.
func pngBytes(t testing.TB) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSniffRejectsNonImages(t *testing.T) {
	for name, data := range map[string][]byte{
		"svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"html": []byte(`<html><script>alert(1)</script></html>`),
		"text": []byte("hello"),
	} {
		if _, _, err := Sniff(data); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
	if ct, ext, err := Sniff(pngBytes(t)); err != nil || ct != "image/png" || ext != ".png" {
		t.Errorf("png: %q %q %v", ct, ext, err)
	}
}

func testStore(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	data := pngBytes(t)

	name, err := Upload(ctx, s, "red dot.png", data)
	if err != nil || !strings.HasSuffix(name, ".png") {
		t.Fatalf("Upload = %q, %v", name, err)
	}
	img, err := s.Get(ctx, strings.TrimSuffix(name, ".png"))
	if err != nil || img.ContentType != "image/png" || !bytes.Equal(img.Data, data) {
		t.Errorf("Get: %q, %d bytes, %v", img.ContentType, len(img.Data), err)
	}
	for _, id := range []string{"missing", "000000000000000000000000", "../../etc/passwd"} {
		if _, err := s.Get(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q): err = %v, want ErrNotFound", id, err)
		}
	}
	if _, err := Upload(ctx, s, "big.png", append(data, make([]byte, MaxSize)...)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversized upload: err = %v", err)
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
