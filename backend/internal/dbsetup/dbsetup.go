// Package dbsetup prepares a MongoDB database for the blog: it creates every
// collection with a $jsonSchema validator and builds the indexes the queries
// rely on. It is meant to run once on a new database (`blog setup-db`), and is
// idempotent, so running it again updates validators and adds missing indexes
// without touching data.
package dbsetup

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/michaelputong/blog/backend/internal/categories"
	"github.com/michaelputong/blog/backend/internal/posts"
)

// Result reports what Setup did to each collection, in order.
type Result struct {
	Collection string
	Created    bool // false: it existed and its validator was updated
}

// Setup creates the collections, validators and indexes in db. languages are
// the site's languages; each gets a unique category-name index.
func Setup(ctx context.Context, db *mongo.Database, languages []string) ([]Result, error) {
	existing, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	have := map[string]bool{}
	for _, name := range existing {
		have[name] = true
	}

	var results []Result
	for _, c := range schemas(languages) {
		validator := bson.D{{Key: "$jsonSchema", Value: c.schema}}
		if have[c.name] {
			// Moderate validation: documents that already break the schema
			// can still be updated, so an older database keeps working.
			err = db.RunCommand(ctx, bson.D{
				{Key: "collMod", Value: c.name},
				{Key: "validator", Value: validator},
				{Key: "validationLevel", Value: "moderate"},
				{Key: "validationAction", Value: "error"},
			}).Err()
		} else {
			err = db.CreateCollection(ctx, c.name, options.CreateCollection().
				SetValidator(validator).
				SetValidationLevel("moderate").
				SetValidationAction("error"))
		}
		if err != nil {
			return results, fmt.Errorf("collection %s: %w", c.name, err)
		}
		results = append(results, Result{Collection: c.name, Created: !have[c.name]})
	}

	if err := posts.NewMongoStore(db).EnsureIndexes(ctx); err != nil {
		return results, fmt.Errorf("posts indexes: %w", err)
	}
	if err := categories.NewMongoStore(db).EnsureIndexes(ctx, languages); err != nil {
		return results, fmt.Errorf("categories indexes: %w", err)
	}
	// The indexes the GridFS driver would otherwise create on the first upload.
	gridfs := map[string]mongo.IndexModel{
		"media.files":  {Keys: bson.D{{Key: "filename", Value: 1}, {Key: "uploadDate", Value: 1}}},
		"media.chunks": {Keys: bson.D{{Key: "files_id", Value: 1}, {Key: "n", Value: 1}}, Options: options.Index().SetUnique(true)},
	}
	for coll, model := range gridfs {
		if _, err := db.Collection(coll).Indexes().CreateOne(ctx, model); err != nil {
			return results, fmt.Errorf("%s indexes: %w", coll, err)
		}
	}
	return results, nil
}

type collection struct {
	name   string
	schema bson.M
}

// Helpers for the schemas below. Go encodes nil slices and maps as null, so
// fields holding them accept null too.
func typ(t ...string) bson.M {
	if len(t) == 1 {
		return bson.M{"bsonType": t[0]}
	}
	return bson.M{"bsonType": t}
}

func object(required []string, props bson.M) bson.M {
	return bson.M{"bsonType": "object", "required": required, "properties": props}
}

// schemas describes the documents each store writes. Extra fields are
// allowed, so adding a field in Go doesn't need a schema change first.
func schemas(languages []string) []collection {
	names := bson.M{}
	for _, lang := range languages {
		names[lang] = typ("string")
	}
	text := object([]string{"headline", "bio"}, bson.M{"headline": typ("string"), "bio": typ("string")})

	return []collection{
		{"posts", object(
			[]string{"slug", "lang", "title", "summary", "body", "publishedAt", "status", "category", "tags"},
			bson.M{
				"slug":        bson.M{"bsonType": "string", "minLength": 1},
				"lang":        bson.M{"bsonType": "string", "enum": languages},
				"title":       typ("string"),
				"subtitle":    typ("string"), // optional
				"summary":     typ("string"),
				"body":        typ("string"),
				"publishedAt": typ("date"),
				"status":      bson.M{"enum": []string{string(posts.Draft), string(posts.Published), string(posts.Disabled)}},
				"category":    typ("string"),
				"tags":        bson.M{"bsonType": []string{"array", "null"}, "maxItems": posts.MaxTags, "items": typ("string")},
				"cover":       typ("string"), // optional
				"draft": bson.M{"bsonType": []string{"object", "null"}, "properties": bson.M{
					"title":       typ("string"),
					"subtitle":    typ("string"),
					"summary":     typ("string"),
					"body":        typ("string"),
					"publishedAt": typ("date"),
					"savedAt":     typ("date"),
				}},
			},
		)},
		{"categories", object(
			[]string{"slug", "names", "createdAt"},
			bson.M{
				"slug":      bson.M{"bsonType": "string", "minLength": 1},
				"names":     bson.M{"bsonType": []string{"object", "null"}, "properties": names},
				"createdAt": typ("date"),
			},
		)},
		{"profile", object(
			[]string{"_id", "name", "updatedAt"},
			bson.M{
				"_id":       bson.M{"enum": []string{"owner"}},
				"name":      typ("string"),
				"photoUrl":  typ("string"),
				"location":  typ("string"),
				"country":   typ("string"),
				"email":     typ("string"),
				"links":     bson.M{"bsonType": []string{"array", "null"}, "items": object([]string{"label", "url"}, bson.M{"label": typ("string"), "url": typ("string")})},
				"texts":     bson.M{"bsonType": []string{"object", "null"}, "additionalProperties": text},
				"updatedAt": typ("date"),
			},
		)},
		{"admin", object(
			[]string{"_id", "passwordHash", "sessionKey", "epoch", "updatedAt"},
			bson.M{
				"_id":          bson.M{"enum": []string{"admin"}},
				"passwordHash": typ("binData"),
				"sessionKey":   typ("binData"),
				"epoch":        typ("long", "int"),
				"updatedAt":    typ("date"),
			},
		)},
		// GridFS bucket "media": the driver writes these; the schema checks
		// that every image records its content type.
		{"media.files", object(
			[]string{"length", "chunkSize", "uploadDate", "filename", "metadata"},
			bson.M{
				"length":     typ("long", "int"),
				"chunkSize":  typ("int", "long"),
				"uploadDate": typ("date"),
				"filename":   typ("string"),
				"metadata":   object([]string{"contentType"}, bson.M{"contentType": typ("string")}),
			},
		)},
		{"media.chunks", object(
			[]string{"files_id", "n", "data"},
			bson.M{
				"files_id": typ("objectId"),
				"n":        typ("int", "long"),
				"data":     typ("binData"),
			},
		)},
	}
}
