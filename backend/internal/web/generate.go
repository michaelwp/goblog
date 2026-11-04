package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
)

const maxArticlePrompt = 4000

// bodyMarker separates the JSON about the article from its Markdown body in
// the AI's reply: long Markdown is more reliable outside a JSON string.
const bodyMarker = "=====BODY====="

const generateSystem = `You are a writer for a blog. Write a complete blog article as the author asks, or, when "currentArticle" is given, revise that article as the author now asks, keeping everything they didn't ask to change.
Write in %[1]s: the title, slug, summary and the whole body must be in %[1]s, even when the author's request or the current article is in another language (then translate as you go).

Reply in exactly this format and nothing else:
1. A JSON object:
{"reply": "...", "title": "...", "slug": "...", "summary": "...", "tags": [...], "category": "...", "newCategory": null}
2. A line containing only ` + bodyMarker + `
3. The article body in Markdown.

JSON rules:
- "reply": one short sentence to the author about what you wrote or changed.
- "title": clear and specific, at most 80 characters, no quotes.
- "slug": 2 to 6 meaningful words for the URL, lowercase a-z, digits and single hyphens only (no accents).
- "summary": one or two sentences, at most 300 characters, that introduce the article.
- "tags": 3 to 6 short topic tags, lowercase, words joined by "-". Prefer tags from "existingTags" when they fit.
- "category": the slug of the ONE category from "existingCategories" that fits, or "" if none fits well.
- "newCategory": only when "category" is "", a broad category name (1-3 words, Title Case) with one name per code in "categoryLanguages", e.g. {"en": "Travel", "id": "Perjalanan"}. Otherwise null.

Body rules:
- Start directly with the text: no title heading (the title is shown separately) and no front matter.
- Use ## for sections and ### for subsections, plus lists, quotes, links and code blocks where they help.
- No images.
- Length as the author asks; otherwise about 700 to 1200 words (not counting the sections below).

End sections, only when there's something to put in them, in this order, after the rest of the text:
- "## %[2]s": if the article relies on facts, figures, quotes or documentation from elsewhere, a bulleted list of those sources as Markdown links, [Page title](https://...). Only cite pages you are confident exist at that exact address (official documentation, standards, Wikipedia, well-known publications); never guess or invent an address. Leave the section out if you have no such source.
- "## %[3]s": if any of "blogArticles" (this blog's own articles) are clearly related, a bulleted list of up to 3 of them as Markdown links, [title](url), using their url exactly as given. You may also link to them where they fit in the text. Leave the section out if none are related.
Every link you write is checked, and links that don't work are removed.`

type generatedArticle struct {
	Reply       string            `json:"reply"`
	Title       string            `json:"title"`
	Slug        string            `json:"slug"`
	Summary     string            `json:"summary"`
	Tags        []string          `json:"tags"`
	Category    string            `json:"category"`
	NewCategory map[string]string `json:"newCategory,omitempty"`
	Body        string            `json:"body"`
}

// adminGenerate writes a whole article from the author's prompt in the new
// article page's chat box, or revises the one in the form (sent along as
// title, summary, body, tags and category) by a follow-up prompt. It answers
// a generatedArticle or {"error"}; nothing is saved.
func (h *pages) adminGenerate(c *fiber.Ctx) error {
	if h.cfg.AI == nil {
		return noAI(c)
	}
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	switch {
	case prompt == "":
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "Describe the article you want."})
	case utf8.RuneCountInString(prompt) > maxArticlePrompt:
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": fmt.Sprintf("Keep the request under %d characters.", maxArticlePrompt)})
	}
	f, body := articleInput(c)
	lang := f.Lang
	if !contains(h.cfg.Languages, lang) {
		lang = h.cfg.Languages[0]
	}

	ctx := c.UserContext()
	cats, err := h.cfg.Categories.List(ctx, h.cfg.Languages[0])
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
	input := map[string]any{
		"request":            prompt,
		"existingCategories": catList,
		"existingTags":       known,
		"categoryLanguages":  h.cfg.Languages,
	}
	// Earlier requests in this chat, oldest first, for context.
	var history []string
	for _, v := range c.Request().PostArgs().PeekMulti("history") {
		if s := strings.TrimSpace(string(v)); s != "" && len(history) < 20 {
			history = append(history, s)
		}
	}
	if len(history) > 0 {
		input["earlierRequests"] = history
	}
	if f.Title != "" || f.Body != "" {
		input["currentArticle"] = map[string]any{
			"title": f.Title, "summary": f.Summary, "body": body, "tags": f.Tags, "category": f.Category,
		}
	}
	input["articleLanguage"] = languageName(lang)
	related, internal := blogArticles(all, lang, c.FormValue("slug"))
	if len(related) > 0 {
		input["blogArticles"] = related
	}
	in, _ := json.MarshalIndent(input, "", " ")

	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	sections := endSectionNames(lang)
	reply, err := h.cfg.AI.CompleteLong(ctx, sprintf(generateSystem, languageName(lang), sections[0], sections[1]), string(in), 12000)
	if err != nil {
		return aiFailed(c, "write the article", err)
	}
	out, err := parseGenerated(reply)
	if err != nil {
		return aiFailed(c, "write the article", err)
	}

	clean := h.cleanSuggestion(cats, out.Tags, out.Category, out.NewCategory)
	out.Tags, out.Category, out.NewCategory = clean.Tags, clean.Category, clean.NewCategory
	out.Title, out.Summary, out.Reply = clip(out.Title, 200), clip(out.Summary, 400), clip(out.Reply, 300)
	out.Slug = slugify(out.Slug)
	if out.Slug == "" {
		out.Slug = slugify(out.Title)
	}
	if len(out.Slug) > 100 {
		out.Slug = strings.TrimRight(out.Slug[:100], "-")
	}
	if out.Title == "" || out.Body == "" {
		return aiFailed(c, "write the article", errors.New("the reply was missing the title or text"))
	}
	if out.Reply == "" {
		out.Reply = "Here's a draft: " + out.Title + "."
	}
	// Made-up or broken links don't reach the article. Only the AI's own are
	// checked: links already in the author's text are theirs to keep. The
	// check gets its own time, not what's left after a slow generation.
	trusted := map[string]bool{}
	for _, link := range linksIn(f.Body) {
		trusted[link] = true
	}
	names := endSectionNames(lang)
	checkCtx, cancelCheck := context.WithTimeout(c.UserContext(), 20*time.Second)
	defer cancelCheck()
	var removed int
	out.Body, removed = verifyLinks(checkCtx, out.Body, linkCheck{internal: internal, trusted: trusted, sections: names[:]})
	out.Reply += removedLinksNote(removed)
	return c.JSON(out)
}

// parseGenerated reads the AI's reply: a JSON object, the body marker, then
// Markdown. A reply that put the body inside the JSON is accepted too.
func parseGenerated(reply string) (generatedArticle, error) {
	var out generatedArticle
	head, body, found := strings.Cut(reply, bodyMarker)
	start, end := strings.Index(head, "{"), strings.LastIndex(head, "}")
	if start < 0 || end < start {
		return out, errors.New("the reply didn't describe the article")
	}
	if err := json.Unmarshal([]byte(head[start:end+1]), &out); err != nil {
		return out, fmt.Errorf("the reply's details weren't valid JSON: %w", err)
	}
	if found {
		out.Body = unfence(strings.TrimSpace(body))
	}
	out.Body = strings.TrimSpace(out.Body)
	return out, nil
}

// endSectionNames are the headings of the "Sources" and "Related articles"
// sections at the end of an AI-written article, in its language.
func endSectionNames(lang string) [2]string {
	switch lang {
	case "id":
		return [2]string{"Sumber", "Artikel terkait"}
	default:
		return [2]string{"Sources", "Related articles"}
	}
}
