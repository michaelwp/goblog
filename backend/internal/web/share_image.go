package web

import (
	_ "embed"

	"github.com/gofiber/fiber/v2"
)

// shareImagePath serves the blog's logo as a 1200×630 PNG: the picture social
// networks show for a shared link when the article has no cover image. They
// don't accept SVG, so it's a PNG rather than the favicon.
const shareImagePath = "/share.png"

//go:embed share.png
var shareImagePNG []byte

func shareImage(c *fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "public, max-age=86400")
	c.Type("png")
	return c.Send(shareImagePNG)
}
