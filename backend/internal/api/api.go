package api

import (
	"errors"
	"log"
	"slices"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/michaelputong/blog/backend/internal/posts"
)

type Config struct {
	Store     posts.Repository
	Languages []string // supported languages; the first is the default
	Logging   bool
}

// New builds the Fiber app with the health check and JSON API registered.
func New(cfg Config) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: errorHandler,
		BodyLimit:    8 << 20, // room for 5 MB image uploads plus form overhead
	})

	app.Use(recover.New())
	if cfg.Logging {
		app.Use(logger.New())
	}

	h := &handlers{store: cfg.Store, languages: cfg.Languages}

	app.Get("/healthz", func(c *fiber.Ctx) error { return c.SendString("ok") })

	v1 := app.Group("/api/v1")
	v1.Get("/languages", h.listLanguages)
	v1.Get("/:lang/posts", h.listPosts)
	v1.Get("/:lang/posts/:slug", h.getPost)
	v1.Use(func(c *fiber.Ctx) error { return fiber.ErrNotFound })

	return app
}

type handlers struct {
	store     posts.Repository
	languages []string
}

func (h *handlers) listLanguages(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"languages": h.languages, "default": h.languages[0]})
}

func (h *handlers) listPosts(c *fiber.Ctx) error {
	lang, err := h.lang(c)
	if err != nil {
		return err
	}
	list, err := h.store.List(c.UserContext(), lang)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"posts": list})
}

func (h *handlers) getPost(c *fiber.Ctx) error {
	lang, err := h.lang(c)
	if err != nil {
		return err
	}
	slug := c.Params("slug")
	p, err := h.store.Get(c.UserContext(), slug, lang)
	if errors.Is(err, posts.ErrNotFound) || (err == nil && p.Status != posts.Published) {
		return fiber.NewError(fiber.StatusNotFound, "post not found")
	}
	if err != nil {
		return err
	}
	p.Draft = nil // unpublished edits are admin-only
	langs, err := h.store.Languages(c.UserContext(), slug)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"post": p, "availableLanguages": langs})
}

func (h *handlers) lang(c *fiber.Ctx) (string, error) {
	lang := c.Params("lang")
	if !slices.Contains(h.languages, lang) {
		return "", fiber.NewError(fiber.StatusNotFound, "unsupported language")
	}
	return lang, nil
}

// errorHandler returns every error as JSON so the frontend can rely on one shape.
func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "internal server error"
	var fe *fiber.Error
	if errors.As(err, &fe) {
		code, msg = fe.Code, fe.Message
	} else {
		log.Printf("%s %s: %v", c.Method(), c.Path(), err)
	}
	return c.Status(code).JSON(fiber.Map{"error": msg})
}
