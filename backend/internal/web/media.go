package web

import (
	"errors"
	"io"
	"path"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/media"
)

// media serves an uploaded image. IDs are random, so a URL's content never
// changes and can be cached indefinitely.
func (h *pages) media(c *fiber.Ctx) error {
	file := c.Params("file")
	img, err := h.cfg.Media.Get(c.UserContext(), strings.TrimSuffix(file, path.Ext(file)))
	if errors.Is(err, media.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).SendString("Image not found.")
	}
	if err != nil {
		return h.fail(c, err)
	}
	c.Set(fiber.HeaderContentType, img.ContentType)
	c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	c.Set(fiber.HeaderContentSecurityPolicy, "default-src 'none'")
	return c.Send(img.Data)
}

// adminUpload receives an image from the article editor and answers with
// its URL as JSON: {"url": "/media/<id>.png"} or {"error": "..."}.
func (h *pages) adminUpload(c *fiber.Ctx) error {
	fail := func(status int, msg string) error {
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return fail(fiber.StatusBadRequest, "Choose an image to upload.")
	}
	if fh.Size > media.MaxSize {
		return fail(fiber.StatusRequestEntityTooLarge, media.ErrTooLarge.Error()+".")
	}
	f, err := fh.Open()
	if err != nil {
		return h.fail(c, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, media.MaxSize+1))
	if err != nil {
		return h.fail(c, err)
	}

	name, err := media.Upload(c.UserContext(), h.cfg.Media, strings.Clone(fh.Filename), data)
	switch {
	case errors.Is(err, media.ErrTooLarge):
		return fail(fiber.StatusRequestEntityTooLarge, err.Error()+".")
	case errors.Is(err, media.ErrUnsupported):
		return fail(fiber.StatusUnsupportedMediaType, err.Error()+".")
	case err != nil:
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"url": "/media/" + name})
}
