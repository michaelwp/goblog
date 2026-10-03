// Package auth stores the admin credential in the database: a bcrypt hash of
// the password, the key that signs session cookies, and an epoch that is
// bumped on every password change to sign out existing sessions.
package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
)

var ErrExists = errors.New("an admin account already exists")

type Credential struct {
	PasswordHash []byte    `bson:"passwordHash"`
	SessionKey   []byte    `bson:"sessionKey"`
	Epoch        int64     `bson:"epoch"`
	UpdatedAt    time.Time `bson:"updatedAt"`
	// UserID is the admin's WebAuthn user handle, created with the first
	// passkey; Passkeys can be used to sign in instead of the password.
	UserID   []byte    `bson:"userId,omitempty"`
	Passkeys []Passkey `bson:"passkeys,omitempty"`
}

const (
	MinPasswordLength = 12
	MaxPasswordBytes  = 72 // bcrypt ignores anything longer
	DefaultCost       = 12
)

// NewCredential hashes password and generates a fresh session key.
func NewCredential(password string, cost int) (Credential, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return Credential{}, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return Credential{}, err
	}
	return Credential{PasswordHash: hash, SessionKey: key, Epoch: 1, UpdatedAt: time.Now().UTC()}, nil
}

// WithPassword returns c with a new password hash and a bumped epoch, which
// invalidates every session issued before the change.
func (c Credential) WithPassword(password string, cost int) (Credential, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return Credential{}, err
	}
	c.PasswordHash, c.Epoch, c.UpdatedAt = hash, c.Epoch+1, time.Now().UTC()
	return c, nil
}

func (c Credential) PasswordMatches(password string) bool {
	return bcrypt.CompareHashAndPassword(c.PasswordHash, []byte(password)) == nil
}

// Store holds the single admin credential. Get reports ok=false when no
// admin has been set up yet.
type Store interface {
	Get(ctx context.Context) (c Credential, ok bool, err error)
	// Create stores c only if no credential exists yet; otherwise ErrExists.
	// This makes first-time setup safe against two concurrent requests.
	Create(ctx context.Context, c Credential) error
	Update(ctx context.Context, c Credential) error
	Delete(ctx context.Context) error
}

// MongoStore keeps the credential as one document in the "admin" collection.
type MongoStore struct {
	coll *mongo.Collection
}

func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{coll: db.Collection("admin")}
}

const docID = "admin"

type doc struct {
	ID         string `bson:"_id"`
	Credential `bson:",inline"`
}

func (s *MongoStore) Get(ctx context.Context) (Credential, bool, error) {
	var d doc
	err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: docID}}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Credential{}, false, nil
	}
	if err != nil {
		return Credential{}, false, fmt.Errorf("get admin credential: %w", err)
	}
	return d.Credential, true, nil
}

func (s *MongoStore) Create(ctx context.Context, c Credential) error {
	_, err := s.coll.InsertOne(ctx, doc{ID: docID, Credential: c})
	if mongo.IsDuplicateKeyError(err) {
		return ErrExists
	}
	if err != nil {
		return fmt.Errorf("create admin credential: %w", err)
	}
	return nil
}

func (s *MongoStore) Update(ctx context.Context, c Credential) error {
	_, err := s.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: docID}}, doc{ID: docID, Credential: c})
	if err != nil {
		return fmt.Errorf("update admin credential: %w", err)
	}
	return nil
}

func (s *MongoStore) Delete(ctx context.Context) error {
	if _, err := s.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: docID}}); err != nil {
		return fmt.Errorf("delete admin credential: %w", err)
	}
	return nil
}

// MemoryStore is an in-memory Store for tests.
type MemoryStore struct {
	mu sync.Mutex
	c  *Credential
}

func (s *MemoryStore) Get(context.Context) (Credential, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.c == nil {
		return Credential{}, false, nil
	}
	return *s.c, true, nil
}

func (s *MemoryStore) Create(_ context.Context, c Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.c != nil {
		return ErrExists
	}
	s.c = &c
	return nil
}

func (s *MemoryStore) Update(_ context.Context, c Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.c = &c
	return nil
}

func (s *MemoryStore) Delete(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.c = nil
	return nil
}
