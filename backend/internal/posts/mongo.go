package posts

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoStore is a Repository backed by a MongoDB collection with one
// document per (slug, lang) translation.
type MongoStore struct {
	coll *mongo.Collection
}

func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{coll: db.Collection("posts")}
}

// EnsureIndexes creates the indexes the queries rely on. It is idempotent.
func (s *MongoStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "slug", Value: 1}, {Key: "lang", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "lang", Value: 1}, {Key: "status", Value: 1}, {Key: "publishedAt", Value: -1}}},
	})
	return err
}

// Migrate brings documents written by older versions up to date: posts
// saved before statuses existed are published. It is idempotent.
func (s *MongoStore) Migrate(ctx context.Context) error {
	_, err := s.coll.UpdateMany(ctx,
		bson.D{{Key: "status", Value: bson.D{{Key: "$exists", Value: false}}}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: Published}}}},
	)
	return err
}

// SeedIfEmpty inserts seed when the collection has no documents, so a fresh
// database renders something. It never touches existing data.
func (s *MongoStore) SeedIfEmpty(ctx context.Context, seed []Post) error {
	n, err := s.coll.EstimatedDocumentCount(ctx)
	if err != nil || n > 0 {
		return err
	}
	for i := range seed {
		if seed[i].Status == "" {
			seed[i].Status = Published
		}
	}
	_, err = s.coll.InsertMany(ctx, seed)
	return err
}

func (s *MongoStore) List(ctx context.Context, lang string) ([]Post, error) {
	cur, err := s.coll.Find(ctx,
		bson.D{{Key: "lang", Value: lang}, {Key: "status", Value: Published}},
		options.Find().SetSort(bson.D{{Key: "publishedAt", Value: -1}}).SetProjection(bson.D{{Key: "draft", Value: 0}}),
	)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	out := []Post{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode posts: %w", err)
	}
	return out, nil
}

func (s *MongoStore) Get(ctx context.Context, slug, lang string) (Post, error) {
	var p Post
	err := s.coll.FindOne(ctx, bson.D{{Key: "slug", Value: slug}, {Key: "lang", Value: lang}}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Post{}, ErrNotFound
	}
	if err != nil {
		return Post{}, fmt.Errorf("get post: %w", err)
	}
	return p, nil
}

func (s *MongoStore) Languages(ctx context.Context, slug string) ([]string, error) {
	var langs []string
	if err := s.coll.Distinct(ctx, "lang", bson.D{{Key: "slug", Value: slug}, {Key: "status", Value: Published}}).Decode(&langs); err != nil {
		return nil, fmt.Errorf("post languages: %w", err)
	}
	sort.Strings(langs)
	return langs, nil
}

func (s *MongoStore) Search(ctx context.Context, lang, query string) ([]Post, error) {
	// A case-insensitive substring match, which is plenty for a small blog.
	// Swap in a text or Atlas Search index if the collection grows large.
	re := bson.Regex{Pattern: regexp.QuoteMeta(query), Options: "i"}
	filter := bson.D{
		{Key: "lang", Value: lang},
		{Key: "status", Value: Published},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "title", Value: re}},
			bson.D{{Key: "summary", Value: re}},
			bson.D{{Key: "body", Value: re}},
		}},
	}
	cur, err := s.coll.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "publishedAt", Value: -1}}).
		SetProjection(bson.D{{Key: "draft", Value: 0}}).
		SetLimit(SearchLimit))
	if err != nil {
		return nil, fmt.Errorf("search posts: %w", err)
	}
	out := []Post{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode search results: %w", err)
	}
	return out, nil
}

func (s *MongoStore) All(ctx context.Context) ([]Post, error) {
	cur, err := s.coll.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{
		{Key: "publishedAt", Value: -1}, {Key: "slug", Value: 1}, {Key: "lang", Value: 1},
	}))
	if err != nil {
		return nil, fmt.Errorf("list all posts: %w", err)
	}
	out := []Post{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode posts: %w", err)
	}
	return out, nil
}

func (s *MongoStore) Create(ctx context.Context, p Post) error {
	_, err := s.coll.InsertOne(ctx, p)
	if mongo.IsDuplicateKeyError(err) { // unique (slug, lang) index
		return ErrExists
	}
	if err != nil {
		return fmt.Errorf("create post: %w", err)
	}
	return nil
}

func (s *MongoStore) Update(ctx context.Context, p Post) error {
	res, err := s.coll.UpdateOne(ctx,
		bson.D{{Key: "slug", Value: p.Slug}, {Key: "lang", Value: p.Lang}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "title", Value: p.Title},
			{Key: "summary", Value: p.Summary},
			{Key: "body", Value: p.Body},
			{Key: "publishedAt", Value: p.PublishedAt},
			{Key: "status", Value: p.Status},
			{Key: "draft", Value: p.Draft}, // nil clears it
		}}},
	)
	if err != nil {
		return fmt.Errorf("update post: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MongoStore) Delete(ctx context.Context, slug, lang string) error {
	res, err := s.coll.DeleteOne(ctx, bson.D{{Key: "slug", Value: slug}, {Key: "lang", Value: lang}})
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MongoStore) DeleteAll(ctx context.Context, slug string) (int, error) {
	res, err := s.coll.DeleteMany(ctx, bson.D{{Key: "slug", Value: slug}})
	if err != nil {
		return 0, fmt.Errorf("delete article: %w", err)
	}
	return int(res.DeletedCount), nil
}
