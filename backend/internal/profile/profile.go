// Package profile stores the blog owner's "About me" profile: one document
// with shared details plus a headline and bio per language.
package profile

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Link struct {
	Label string `json:"label" bson:"label"`
	URL   string `json:"url" bson:"url"`
}

// Text is the translatable part of the profile.
type Text struct {
	Headline string `json:"headline" bson:"headline"`
	Bio      string `json:"bio" bson:"bio"` // same plain-text format as post bodies
}

type Profile struct {
	Name      string          `json:"name" bson:"name"`
	PhotoURL  string          `json:"photoUrl" bson:"photoUrl"`
	Location  string          `json:"location" bson:"location"`
	Country   string          `json:"country" bson:"country"` // ISO 3166-1 alpha-2, shown as a flag
	Email     string          `json:"email" bson:"email"`
	Links     []Link          `json:"links" bson:"links"`
	Texts     map[string]Text `json:"texts" bson:"texts"` // by language
	UpdatedAt time.Time       `json:"updatedAt" bson:"updatedAt"`
}

// Store reads and writes the single profile. Get returns an empty Profile
// (not an error) when none has been saved yet.
type Store interface {
	Get(ctx context.Context) (Profile, error)
	Save(ctx context.Context, p Profile) error
}

// MongoStore keeps the profile as one document in the "profile" collection.
type MongoStore struct {
	coll *mongo.Collection
}

func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{coll: db.Collection("profile")}
}

const docID = "owner"

func (s *MongoStore) Get(ctx context.Context) (Profile, error) {
	var p Profile
	err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: docID}}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Profile{}, nil
	}
	if err != nil {
		return Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return p, nil
}

func (s *MongoStore) Save(ctx context.Context, p Profile) error {
	_, err := s.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: docID}}, p, options.Replace().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	return nil
}

// MemoryStore is an in-memory Store for tests.
type MemoryStore struct {
	mu sync.RWMutex
	p  Profile
}

func (s *MemoryStore) Get(context.Context) (Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.p, nil
}

func (s *MemoryStore) Save(_ context.Context, p Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p = p
	return nil
}
