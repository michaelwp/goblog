package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/categories"
	"github.com/michaelputong/blog/backend/internal/media"
	"github.com/michaelputong/blog/backend/internal/posts"
)

// newCategoryValue is the category picker's value for "create the category
// named in the newCategory_<lang> fields when saving".
const newCategoryValue = "__new"

// maxSuggestBody caps how much of the article is sent to the AI: the start
// of an article says enough about its topic.
const maxSuggestBody = 8000

type suggestion struct {
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Tags     []string `json:"tags"`
	Category string   `json:"category"` // an existing category's slug, or ""
	// Names by language for a new category, when no existing one fits.
	NewCategory map[string]string `json:"newCategory,omitempty"`
	// Cover is one of the article's own images that would make a good
	// cover, or "".
	Cover string `json:"cover"`
}

const suggestSystem = `You are an editor for a blog. Given an article, the blog's existing categories and the tags it already uses, reply with ONLY a JSON object, no prose:
{"title": "...", "summary": "...", "tags": [...], "category": "...", "newCategory": null, "cover": ""}

Rules:
- Write "title" and "summary" in the article's language.
- "title": a clear, specific, engaging title, at most 80 characters, no quotes, no clickbait.
- "summary": one or two sentences (at most 300 characters) that introduce the article to a reader deciding whether to read it.
- "tags": 3 to 6 short topic tags, lowercase, words joined by "-" (e.g. "web-security"). Prefer tags the blog already uses when they fit. No "#".
- "category": the slug of the ONE existing category that fits the article's main topic, or "" if none fits well.
- "newCategory": only when "category" is "", a broad category name (1-3 words, Title Case, not as narrow as a tag) as an object with one name per requested language code, translated naturally, e.g. {"en": "Travel", "id": "Perjalanan"}. Otherwise null.
- "cover": the "url" of the image from "images" that best represents the whole article as a cover picture, or "" if there are no images or none suits.`

// articleInput reads the article fields the AI looks at.
func articleInput(c *fiber.Ctx) (adminForm, string) {
	f := readForm(c)
	body := f.Body
	if len(body) > maxSuggestBody {
		body = strings.ToValidUTF8(body[:maxSuggestBody], "") // drop a character cut in half
	}
	return f, body
}

func noAI(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "AI suggestions aren't set up: add AI_API_KEY to .env."})
}

func aiFailed(c *fiber.Ctx, what string, err error) error {
	log.Printf("ai %s: %v", what, err)
	return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "The AI couldn't " + what + ": " + err.Error()})
}

func (h *pages) adminSuggest(c *fiber.Ctx) error {
	if h.cfg.AI == nil {
		return noAI(c)
	}
	f, body := articleInput(c)
	if f.Title == "" && f.Body == "" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "Write a title or some text first."})
	}
	ctx := c.UserContext()
	lang := h.cfg.Languages[0]
	cats, err := h.cfg.Categories.List(ctx, lang)
	if err != nil {
		return h.fail(c, err)
	}
	all, err := h.cfg.Store.All(ctx)
	if err != nil {
		return h.fail(c, err)
	}
	known := knownTags(all)
	if len(known) > 150 {
		known = known[:150]
	}
	catList := make([]map[string]any, len(cats))
	for i, cat := range cats {
		catList[i] = map[string]any{"slug": cat.Slug, "names": cat.Names}
	}
	images := articleImages(f.Body)
	input, _ := json.MarshalIndent(map[string]any{
		"article":              map[string]string{"language": languageName(f.Lang), "title": f.Title, "summary": f.Summary, "body": body},
		"images":               images,
		"existingCategories":   catList,
		"existingTags":         known,
		"newCategoryLanguages": h.cfg.Languages,
	}, "", " ")

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var raw struct {
		Title       string            `json:"title"`
		Summary     string            `json:"summary"`
		Tags        []string          `json:"tags"`
		Category    string            `json:"category"`
		NewCategory map[string]string `json:"newCategory"`
		Cover       string            `json:"cover"`
	}
	if err := h.cfg.AI.CompleteJSON(ctx, suggestSystem, string(input), &raw); err != nil {
		return aiFailed(c, "make suggestions", err)
	}
	out := h.cleanSuggestion(cats, raw.Tags, raw.Category, raw.NewCategory)
	out.Title = clip(raw.Title, 200)
	out.Summary = clip(raw.Summary, 400)
	for _, img := range images {
		if img.URL == raw.Cover {
			out.Cover = img.URL
		}
	}
	return c.JSON(out)
}

// clip tidies whitespace and quotes from an AI-written line and keeps it
// within max characters (the form's limit), cutting at a word.
func clip(s string, max int) string {
	s = strings.Trim(strings.Join(strings.Fields(s), " "), `"“”'`)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)[:max]
	if i := strings.LastIndex(string(r), " "); i > 0 {
		return strings.TrimRight(string(r)[:i], ",;:") + "…"
	}
	return string(r)
}

type articleImage struct {
	URL     string `json:"url"`
	Caption string `json:"caption,omitempty"`
}

// markdownImage matches ![alt](url "title"); the editor writes images so.
var markdownImage = regexp.MustCompile(`!\[([^\]]*)\]\((\S+?)(?:\s+"([^"]*)")?\)`)

// articleImages lists the images in a Markdown body that could be a cover:
// uploaded ones and https addresses, each once, at most 20.
func articleImages(body string) []articleImage {
	out := []articleImage{}
	seen := map[string]bool{}
	for _, m := range markdownImage.FindAllStringSubmatch(body, -1) {
		url := m[2]
		if seen[url] || len(out) == 20 || !strings.HasPrefix(url, "/media/") && !strings.HasPrefix(url, "https://") {
			continue
		}
		seen[url] = true
		caption := strings.TrimSpace(m[3])
		if caption == "" {
			caption = strings.TrimSpace(m[1])
		}
		out = append(out, articleImage{URL: url, Caption: caption})
	}
	return out
}

const coverPickSystem = `You choose a cover picture for a blog article. Given the article and its images (with captions), reply with ONLY a JSON object: {"cover": "<url of the best image>"}, or {"cover": ""} if none suits.`

// adminSuggestCover generates a cover image when the provider can, saving it
// with the blog's other images, and otherwise picks the best image already
// in the article. It answers {"cover": "/media/...", "generated": true} or
// {"error": "..."}.
func (h *pages) adminSuggestCover(c *fiber.Ctx) error {
	if h.cfg.AI == nil {
		return noAI(c)
	}
	f, body := articleInput(c)
	ctx, cancel := context.WithTimeout(c.UserContext(), 110*time.Second)
	defer cancel()

	if h.cfg.AI.CanGenerateImages() {
		if f.Title == "" && f.Summary == "" && f.Body == "" {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "Write a title or some text first."})
		}
		about := "Title: " + f.Title + "\nSummary: " + f.Summary
		if f.Summary == "" {
			about += "\nStart of the article: " + clip(body, 1200)
		}
		prompt := "A cover image for a blog article.\n" + about + "\n\n" +
			"Make a wide, landscape editorial image that captures the article's subject. " +
			"Clean, modern, high quality. Absolutely no text, letters, words, captions, logos or watermarks."
		img, err := h.cfg.AI.GenerateImage(ctx, prompt)
		if err != nil {
			return aiFailed(c, "generate a cover", err)
		}
		name, err := media.Upload(ctx, h.cfg.Media, "ai-cover.jpg", compactImage(img))
		if err != nil {
			return aiFailed(c, "generate a cover", err)
		}
		return c.JSON(fiber.Map{"cover": "/media/" + name, "generated": true})
	}

	images := articleImages(f.Body)
	if len(images) == 0 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": "This AI provider can't create images, and the article has none to choose from. Add an image to the article, or upload a cover.",
		})
	}
	input, _ := json.MarshalIndent(map[string]any{
		"article": map[string]string{"title": f.Title, "summary": f.Summary, "body": clip(body, 3000)},
		"images":  images,
	}, "", " ")
	var raw struct {
		Cover string `json:"cover"`
	}
	if err := h.cfg.AI.CompleteJSON(ctx, coverPickSystem, string(input), &raw); err != nil {
		return aiFailed(c, "pick a cover", err)
	}
	for _, img := range images {
		if img.URL == raw.Cover {
			return c.JSON(fiber.Map{"cover": img.URL, "generated": false})
		}
	}
	return c.JSON(fiber.Map{"cover": images[0].URL, "generated": false}) // the AI found none fitting: the first one
}

// cleanSuggestion keeps only what the editor can use: valid tags, a
// category that exists, and a new category named in every language (falling
// back to the default language's name) that doesn't duplicate an existing one.
func (h *pages) cleanSuggestion(cats []categories.Category, tags []string, category string, names map[string]string) suggestion {
	out := suggestion{Tags: []string{}}
	for _, t := range tags {
		if valid, msg := posts.ParseTags(t); msg == "" && len(valid) == 1 && !contains(out.Tags, valid[0]) && len(out.Tags) < posts.MaxTags {
			out.Tags = append(out.Tags, valid[0])
		}
	}
	for _, cat := range cats {
		if cat.Slug == category {
			out.Category = category
			return out
		}
	}
	clean := h.categoryNames(names, true)
	if clean == nil {
		return out
	}
	if existing := findByName(cats, clean, h.cfg.Languages[0]); existing != "" {
		out.Category = existing
		return out
	}
	out.NewCategory = clean
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// categoryNames tidies a new category's names and fills in languages left
// empty with the default language's name (they can be renamed under
// Categories). With anyLanguage, a name in any language can stand in for a
// missing default one, as the AI sometimes leaves it out. It returns nil
// when there's no name at all.
func (h *pages) categoryNames(names map[string]string, anyLanguage bool) map[string]string {
	def := h.cfg.Languages[0]
	clean := map[string]string{}
	for _, lang := range h.cfg.Languages {
		clean[lang] = strings.Join(strings.Fields(names[lang]), " ")
	}
	for _, lang := range h.cfg.Languages {
		if clean[def] == "" && anyLanguage {
			clean[def] = clean[lang]
		}
	}
	if clean[def] == "" {
		return nil
	}
	for _, lang := range h.cfg.Languages {
		if clean[lang] == "" {
			clean[lang] = clean[def]
		}
	}
	return clean
}

// checkNewCategory explains what's wrong with a new category's names, or "".
func (h *pages) checkNewCategory(names map[string]string) string {
	clean := h.categoryNames(names, false)
	if clean == nil {
		return "Name the new category, or pick one from the list."
	}
	for _, name := range clean {
		if utf8.RuneCountInString(name) > maxCategoryName {
			return fmt.Sprintf("Keep the category name under %d characters.", maxCategoryName)
		}
	}
	return ""
}

// findByName returns the slug of the category with the same name in the
// default language (ignoring case), or "". Only the default name counts:
// the other names may just be copies of it.
func findByName(cats []categories.Category, names map[string]string, def string) string {
	for _, cat := range cats {
		if names[def] != "" && strings.EqualFold(strings.TrimSpace(cat.Names[def]), names[def]) {
			return cat.Slug
		}
	}
	return ""
}

// saveWithCategory runs save for p. When p's category is a new one, it is
// created (or an existing one with that name reused) first and p pointed at
// it; if save then fails, a category created for it is removed again, so a
// failed save leaves nothing behind. msg explains a problem with the names.
//
// replaces is a category autosave already created for this article whose
// names the author changed since: it's renamed instead of making another,
// as long as no other article uses it.
func (h *pages) saveWithCategory(ctx context.Context, p *posts.Post, form adminForm, save func() error) (msg string, err error) {
	if p.Category != newCategoryValue {
		return "", save()
	}
	slug, created, msg, err := h.ensureCategory(ctx, form.NewCategory, form.ReplacesCategory, p.Slug)
	if err != nil || msg != "" {
		return msg, err
	}
	p.Category = slug
	if err := save(); err != nil {
		if created {
			_ = h.cfg.Categories.Delete(ctx, slug)
		}
		return "", err
	}
	return "", nil
}

// ensureCategory returns the slug of the category named names, renaming
// replaces or creating one if needed; created says it made a new one.
func (h *pages) ensureCategory(ctx context.Context, names map[string]string, replaces, article string) (slug string, created bool, msg string, err error) {
	if msg := h.checkNewCategory(names); msg != "" {
		return "", false, msg, nil
	}
	def := h.cfg.Languages[0]
	clean := h.categoryNames(names, false)
	explicit := func(lang string) bool { return strings.TrimSpace(names[lang]) != "" }
	nameTaken := func(err error) (string, bool) {
		var taken *categories.NameTakenError
		if !errors.As(err, &taken) {
			return "", false
		}
		if !explicit(taken.Lang) {
			return fmt.Sprintf("Another category is already called “%s” in %s: enter this category's %s name.", taken.Name, taken.Lang, taken.Lang), true
		}
		return fmt.Sprintf("“%s” is already used by another category.", taken.Name), true
	}

	cats, err := h.cfg.Categories.List(ctx, def)
	if err != nil {
		return "", false, "", err
	}
	if existing := findByName(cats, clean, def); existing != "" {
		return existing, false, "", nil
	}
	if replaces != "" {
		if only, err := h.onlyUsedBy(ctx, replaces, article); err != nil {
			return "", false, "", err
		} else if only {
			err := h.cfg.Categories.Rename(ctx, categories.Category{Slug: replaces, Names: clean})
			if m, ok := nameTaken(err); ok {
				return "", false, m, nil
			}
			if err == nil {
				return replaces, false, "", nil
			}
			if !errors.Is(err, categories.ErrNotFound) {
				return "", false, "", err
			}
		}
	}

	base := slugify(clean[def])
	if base == "" {
		base = "category"
	}
	if len(base) > 45 {
		base = strings.TrimRight(base[:45], "-")
	}
	for i := 1; i < 100; i++ {
		slug = base
		if i > 1 {
			slug = fmt.Sprintf("%s-%d", base, i)
		}
		err = h.cfg.Categories.Create(ctx, categories.Category{Slug: slug, Names: clean, CreatedAt: time.Now().UTC()})
		if err == nil {
			return slug, true, "", nil
		}
		if m, ok := nameTaken(err); ok {
			return "", false, m, nil
		}
		if !errors.Is(err, categories.ErrSlugTaken) {
			return "", false, "", err
		}
	}
	return "", false, "", err
}

// onlyUsedBy reports whether no article other than article uses category.
func (h *pages) onlyUsedBy(ctx context.Context, category, article string) (bool, error) {
	all, err := h.cfg.Store.All(ctx)
	if err != nil {
		return false, err
	}
	for _, p := range all {
		if p.Category == category && p.Slug != article {
			return false, nil
		}
	}
	return true, nil
}

// compactImage re-encodes a generated PNG as JPEG: a wide PNG from some
// providers can be larger than the upload limit, and a cover photo doesn't
// need lossless. JPEGs and anything it can't decode are left as they are.
func compactImage(data []byte) []byte {
	if http.DetectContentType(data) != "image/png" {
		return data
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return data
	}
	return buf.Bytes()
}

// fieldRules tells the AI how to write each single field the editor's ✨
// buttons ask for.
var fieldRules = map[string]string{
	"title":   `"value": a clear, specific, engaging title for the article, in its language, at most 80 characters, no quotes, no clickbait.`,
	"summary": `"value": one or two sentences (at most 300 characters), in the article's language, that introduce it to a reader deciding whether to read it.`,
	"tags":    `"value": an array of 3 to 6 short topic tags, lowercase, words joined by "-" (e.g. "web-security"), no "#". Prefer tags from "existingTags" when they fit.`,
	"slug":    `"value": a short URL slug for the article: 2 to 6 meaningful words from its title or topic, in the article's language, lowercase letters a-z, digits and single hyphens only (no accents), at most 60 characters.`,
}

const fieldSystem = `You are an editor for a blog. Given an article, write one thing for it. Reply with ONLY a JSON object: {"value": ...}
%s`

// adminSuggestField writes one field (title, summary, tags or slug) for the
// article in the form. It answers {"value": "..."} ({"value": [...]} for
// tags) or {"error": "..."}; nothing is saved.
func (h *pages) adminSuggestField(c *fiber.Ctx) error {
	if h.cfg.AI == nil {
		return noAI(c)
	}
	field := c.FormValue("field")
	rules, ok := fieldRules[field]
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unknown field."})
	}
	f, body := articleInput(c)
	if f.Title == "" && f.Body == "" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "Write a title or some text first."})
	}
	input := map[string]any{"article": map[string]string{"language": languageName(f.Lang), "title": f.Title, "summary": f.Summary, "body": body}}
	if field == "tags" {
		all, err := h.cfg.Store.All(c.UserContext())
		if err != nil {
			return h.fail(c, err)
		}
		known := knownTags(all)
		if len(known) > 150 {
			known = known[:150]
		}
		input["existingTags"] = known
	}
	in, _ := json.MarshalIndent(input, "", " ")

	ctx, cancel := context.WithTimeout(c.UserContext(), 60*time.Second)
	defer cancel()
	var raw struct {
		Value json.RawMessage `json:"value"`
	}
	if err := h.cfg.AI.CompleteJSON(ctx, fmt.Sprintf(fieldSystem, rules), string(in), &raw); err != nil {
		return aiFailed(c, "write the "+field, err)
	}
	if field == "tags" {
		var tags []string
		_ = json.Unmarshal(raw.Value, &tags)
		out := h.cleanSuggestion(nil, tags, "", nil).Tags
		if len(out) == 0 {
			return aiFailed(c, "write the tags", errors.New("no usable tags in the reply"))
		}
		return c.JSON(fiber.Map{"value": out})
	}
	var value string
	_ = json.Unmarshal(raw.Value, &value)
	switch field {
	case "title":
		value = clip(value, 200)
	case "summary":
		value = clip(value, 400)
	case "slug":
		value = slugify(value)
		if len(value) > 100 {
			value = strings.TrimRight(value[:100], "-")
		}
	}
	if value == "" {
		return aiFailed(c, "write the "+field, errors.New("the reply was empty"))
	}
	return c.JSON(fiber.Map{"value": value})
}
