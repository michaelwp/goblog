// Package media stores uploaded images. MongoStore keeps them in GridFS so the
// blog's data, text and images alike, lives in one database.
package media

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const MaxSize = 5 << 20 // 5 MiB

var (
	ErrNotFound    = errors.New("image not found")
	ErrTooLarge    = fmt.Errorf("images can be at most %d MB", MaxSize>>20)
	ErrUnsupported = errors.New("only PNG, JPEG, GIF and WebP images are supported")
)

// allowed maps sniffed content types to file extensions. SVG is excluded on
// purpose: it can carry scripts.
var allowed = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// Sniff returns the image's content type and extension, judged by its bytes
// rather than the uploaded filename or the client's Content-Type header.
func Sniff(data []byte) (contentType, ext string, err error) {
	ct := http.DetectContentType(data)
	ext, ok := allowed[ct]
	if !ok {
		return "", "", ErrUnsupported
	}
	return ct, ext, nil
}

type Image struct {
	ContentType string
	Data        []byte
}

type Store interface {
	// Save stores an already-validated image and returns its ID.
	Save(ctx context.Context, name string, img Image) (id string, err error)
	// Get returns the image with id, or ErrNotFound.
	Get(ctx context.Context, id string) (Image, error)
}

// Upload validates data and saves it, returning the public file name
// ("<id><ext>") to use in /media/ URLs.
func Upload(ctx context.Context, s Store, name string, data []byte) (string, error) {
	if len(data) > MaxSize {
		return "", ErrTooLarge
	}
	ct, ext, err := Sniff(data)
	if err != nil {
		return "", err
	}
	id, err := s.Save(ctx, name, Image{ContentType: ct, Data: data})
	if err != nil {
		return "", err
	}
	return id + ext, nil
}

// MongoStore keeps images in the "media" GridFS bucket.
type MongoStore struct {
	bucket *mongo.GridFSBucket
}

func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{bucket: db.GridFSBucket(options.GridFSBucket().SetName("media"))}
}

type metadata struct {
	ContentType string `bson:"contentType"`
}

func (s *MongoStore) Save(ctx context.Context, name string, img Image) (string, error) {
	id, err := s.bucket.UploadFromStream(ctx, name, bytes.NewReader(img.Data),
		options.GridFSUpload().SetMetadata(metadata{ContentType: img.ContentType}))
	if err != nil {
		return "", fmt.Errorf("save image: %w", err)
	}
	return id.Hex(), nil
}

func (s *MongoStore) Get(ctx context.Context, id string) (Image, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return Image{}, ErrNotFound
	}
	stream, err := s.bucket.OpenDownloadStream(ctx, oid)
	if errors.Is(err, mongo.ErrFileNotFound) {
		return Image{}, ErrNotFound
	}
	if err != nil {
		return Image{}, fmt.Errorf("open image: %w", err)
	}
	defer func() { _ = stream.Close() }() // read-only: a close error can't lose data

	var meta metadata
	if err := bson.Unmarshal(stream.GetFile().Metadata, &meta); err != nil {
		return Image{}, fmt.Errorf("image metadata: %w", err)
	}
	data, err := io.ReadAll(stream)
	if err != nil {
		return Image{}, fmt.Errorf("read image: %w", err)
	}
	return Image{ContentType: meta.ContentType, Data: data}, nil
}

// MemoryStore is an in-memory Store for tests.
type MemoryStore struct {
	mu     sync.RWMutex
	images map[string]Image
}

func (s *MemoryStore) Save(_ context.Context, _ string, img Image) (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.images == nil {
		s.images = map[string]Image{}
	}
	s.images[id] = img
	return id, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Image, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	img, ok := s.images[id]
	if !ok {
		return Image{}, ErrNotFound
	}
	return img, nil
}
