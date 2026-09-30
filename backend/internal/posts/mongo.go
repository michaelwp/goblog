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
		{Keys: bson.D{{Key: "lang", Value: 1}, {Key: "status", Value: 1}, {Key: "category", Value: 1}}},
		{Keys: bson.D{{Key: "lang", Value: 1}, {Key: "status", Value: 1}, {Key: "tags", Value: 1}}},
	})
	return err
}

// Migrate brings documents written by older versions up to date: posts
// saved before statuses existed are published, and posts from before
// categories and tags get empty ones. It is idempotent.
func (s *MongoStore) Migrate(ctx context.Context) error {
	for field, value := range map[string]any{"status": Published, "category": "", "tags": bson.A{}} {
		_, err := s.coll.UpdateMany(ctx,
			bson.D{{Key: field, Value: bson.D{{Key: "$exists", Value: false}}}},
			bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: value}}}},
		)
		if err != nil {
			return err
		}
	}
	return nil
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

func (s *MongoStore) Find(ctx context.Context, f Filter) ([]Post, error) {
	filter := bson.D{{Key: "lang", Value: f.Lang}, {Key: "status", Value: Published}}
	if f.Category != "" {
		filter = append(filter, bson.E{Key: "category", Value: f.Category})
	}
	if f.Tag != "" {
		filter = append(filter, bson.E{Key: "tags", Value: f.Tag})
	}
	if f.Text != "" {
		// A case-insensitive substring match, which is plenty for a small blog.
		// Swap in a text or Atlas Search index if the collection grows large.
		re := bson.Regex{Pattern: regexp.QuoteMeta(f.Text), Options: "i"}
		filter = append(filter, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "title", Value: re}},
			bson.D{{Key: "summary", Value: re}},
			bson.D{{Key: "body", Value: re}},
		}})
	}
	cur, err := s.coll.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "publishedAt", Value: -1}}).
		SetProjection(bson.D{{Key: "draft", Value: 0}}).
		SetLimit(SearchLimit))
	if err != nil {
		return nil, fmt.Errorf("find posts: %w", err)
	}
	out := []Post{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode posts: %w", err)
	}
	return out, nil
}

func (s *MongoStore) TagCounts(ctx context.Context, lang string) ([]TagCount, error) {
	cur, err := s.coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "lang", Value: lang}, {Key: "status", Value: Published}}}},
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$tags"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
	})
	if err != nil {
		return nil, fmt.Errorf("count tags: %w", err)
	}
	var rows []TagCount
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode tag counts: %w", err)
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Tag] = r.Count
	}
	return sortTagCounts(counts), nil
}

func (s *MongoStore) CategoryCounts(ctx context.Context, lang string) (map[string]int, error) {
	cur, err := s.coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "lang", Value: lang}, {Key: "status", Value: Published}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$category", ""}}}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	})
	if err != nil {
		return nil, fmt.Errorf("count categories: %w", err)
	}
	var rows []struct {
		Category string `bson:"_id"`
		Count    int    `bson:"count"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode category counts: %w", err)
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Category] = r.Count
	}
	return counts, nil
}

func (s *MongoStore) SetMeta(ctx context.Context, slug, category string, tags []string) error {
	if tags == nil {
		tags = []string{}
	}
	_, err := s.coll.UpdateMany(ctx, bson.D{{Key: "slug", Value: slug}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "category", Value: category}, {Key: "tags", Value: tags}}}})
	if err != nil {
		return fmt.Errorf("set article metadata: %w", err)
	}
	return nil
}

func (s *MongoStore) SetCover(ctx context.Context, slug, cover string) error {
	_, err := s.coll.UpdateMany(ctx, bson.D{{Key: "slug", Value: slug}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "cover", Value: cover}}}})
	if err != nil {
		return fmt.Errorf("set article cover: %w", err)
	}
	return nil
}

func (s *MongoStore) ReassignCategory(ctx context.Context, from, to string) (int, error) {
	res, err := s.coll.UpdateMany(ctx, bson.D{{Key: "category", Value: from}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "category", Value: to}}}})
	if err != nil {
		return 0, fmt.Errorf("reassign category: %w", err)
	}
	return int(res.ModifiedCount), nil
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
			{Key: "subtitle", Value: p.Subtitle},
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
