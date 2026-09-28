package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/michaelputong/blog/backend/internal/api"
	"github.com/michaelputong/blog/backend/internal/auth"
	"github.com/michaelputong/blog/backend/internal/categories"
	"github.com/michaelputong/blog/backend/internal/media"
	"github.com/michaelputong/blog/backend/internal/posts"
	"github.com/michaelputong/blog/backend/internal/profile"
	"github.com/michaelputong/blog/backend/internal/ssr"
	"github.com/michaelputong/blog/backend/internal/web"
)

// Usage:
//
//	server               run the web server
//	server reset-admin   delete the admin password so it can be set up again
func main() {
	client, err := mongo.Connect(options.Client().ApplyURI(env("MONGODB_URI", "mongodb://localhost:27017")))
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("mongo ping: %v", err)
	}
	db := client.Database(env("MONGODB_DB", "blog"))
	credentials := auth.NewMongoStore(db)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "reset-admin":
			if err := credentials.Delete(ctx); err != nil {
				log.Fatal(err)
			}
			fmt.Println("Admin password removed. Open /admin/setup and use the setup code from the server log to set a new one.")
			return
		default:
			log.Fatalf("unknown command %q (available: reset-admin)", os.Args[1])
		}
	}

	store := posts.NewMongoStore(db)
	if err := store.EnsureIndexes(ctx); err != nil {
		log.Fatalf("mongo indexes: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		log.Fatalf("mongo migrate: %v", err)
	}
	if err := store.SeedIfEmpty(ctx, posts.SeedPosts()); err != nil {
		log.Fatalf("mongo seed: %v", err)
	}
	cancel()

	bundle, err := web.ServerBundle()
	if err != nil {
		log.Fatalf("frontend not embedded (run `npm run build` in frontend/ before `go build`): %v", err)
	}
	renderer, err := ssr.New(bundle)
	if err != nil {
		log.Fatal(err)
	}

	languages := strings.Split(env("LANGUAGES", "en,id"), ",")
	cats := categories.NewMongoStore(db)
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	if err := cats.EnsureIndexes(ctx, languages); err != nil {
		log.Fatalf("category indexes: %v", err)
	}
	cancel()
	app := api.New(api.Config{Store: store, Categories: cats, Languages: languages, Logging: true})
	web.Register(app, web.Config{
		Store:       store,
		Profiles:    profile.NewMongoStore(db),
		Categories:  cats,
		Media:       media.NewMongoStore(db),
		Credentials: credentials,
		Renderer:    renderer,
		Languages:   languages,
	})

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
		<-quit
		_ = app.Shutdown()
	}()

	if err := app.Listen(":" + env("PORT", "8080")); err != nil {
		log.Print(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = client.Disconnect(ctx)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
