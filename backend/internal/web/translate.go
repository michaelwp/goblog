package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/posts"
)

// languageNames names languages in AI prompts; other codes are used as is.
var languageNames = map[string]string{"en": "English", "id": "Indonesian (Bahasa Indonesia)"}

func languageName(code string) string {
	if n, ok := languageNames[code]; ok {
		return n
	}
	return code
}

// maxTranslateChunk is roughly how much Markdown goes to the AI at once, so
// a long article's translation fits in one reply.
const maxTranslateChunk = 6000

const translateTextSystem = `You are a professional translator for a blog. Translate the JSON values from %s to %s. Keep the meaning, tone and register; write naturally, as a native speaker would. Keep names, product names and code as they are. Reply with ONLY a JSON object with exactly the same keys as the one you're given (from "title" and "summary").`

const translateBodySystem = `You are a professional translator for a blog. Translate the Markdown article text you're given from %s to %s. Keep the meaning, tone and register; write naturally, as a native speaker would.
Keep the Markdown exactly: headings, lists, emphasis, links and line breaks. Don't translate URLs, code blocks, inline code, HTML tags or names. Do translate link text and image descriptions (the text in ![...] and in "..." image titles).
Reply with ONLY the translated Markdown: no notes, no explanations, and no surrounding code fence.`

// adminTranslate translates the article's version in another language (as
// it's being edited, with any pending changes) into this one. It answers
// {"title", "summary", "body"} or {"error"}; nothing is saved.
func (h *pages) adminTranslate(c *fiber.Ctx) error {
	if h.cfg.AI == nil {
		return noAI(c)
	}
	slug, from, to := c.FormValue("slug"), c.FormValue("from"), c.FormValue("to")
	if !contains(h.cfg.Languages, from) || !contains(h.cfg.Languages, to) || from == to {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Choose two different languages."})
	}
	src, err := h.cfg.Store.Get(c.UserContext(), slug, from)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "There's no " + languageName(from) + " version of this article to translate yet."})
	}
	src = src.Editing()
	if src.Title == "" && src.Body == "" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "The " + languageName(from) + " version is empty."})
	}

	// Long articles take a few calls; allow for that.
	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Minute)
	defer cancel()
	// only=title, summary or body translates just that field (the editor's
	// ✨ buttons beside each); without it, all three.
	only := c.FormValue("only")
	if only != "" && only != "title" && only != "summary" && only != "body" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unknown field."})
	}
	out, err := h.translatePost(ctx, src, from, to, only)
	if err != nil {
		return aiFailed(c, "translate the article", err)
	}
	return c.JSON(out)
}

type translation struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
}

// translatePost translates src's title, summary and body, or just the one
// named by only.
func (h *pages) translatePost(ctx context.Context, src posts.Post, from, to, only string) (translation, error) {
	var out translation
	want := func(field string) bool { return only == "" || only == field }
	empty := map[string]bool{"title": src.Title == "", "summary": src.Summary == "", "body": src.Body == ""}
	if only != "" && empty[only] {
		what := map[string]string{"title": "title", "summary": "summary", "body": "text"}[only]
		return out, errors.New("the " + languageName(from) + " version has no " + what + " yet")
	}

	text := map[string]string{}
	if want("title") && src.Title != "" {
		text["title"] = src.Title
	}
	if want("summary") && src.Summary != "" {
		text["summary"] = src.Summary
	}
	if len(text) > 0 {
		in, _ := jsonString(text)
		var got translation
		if err := h.cfg.AI.CompleteJSON(ctx, sprintf(translateTextSystem, languageName(from), languageName(to)), in, &got); err != nil {
			return out, err
		}
		if text["title"] != "" {
			out.Title = clip(got.Title, 200)
		}
		if text["summary"] != "" {
			out.Summary = clip(got.Summary, 400)
		}
		if (text["title"] != "" && out.Title == "") || (text["summary"] != "" && out.Summary == "") {
			return out, errors.New("the reply was missing a translation")
		}
	}

	if want("body") {
		var body []string
		for _, chunk := range markdownChunks(src.Body, maxTranslateChunk) {
			t, err := h.cfg.AI.CompleteLong(ctx, sprintf(translateBodySystem, languageName(from), languageName(to)), chunk, 8000)
			if err != nil {
				return out, err
			}
			body = append(body, unfence(t))
		}
		out.Body = strings.Join(body, "\n\n")
	}
	return out, nil
}

// markdownChunks splits Markdown into pieces of about max bytes, only at
// blank lines between blocks and never inside a code block.
func markdownChunks(md string, max int) []string {
	md = strings.TrimSpace(md)
	if md == "" {
		return nil
	}
	var chunks []string
	var cur strings.Builder
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if trimmed == "" && !inFence && cur.Len() >= max {
			chunks = append(chunks, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		chunks = append(chunks, s)
	}
	return chunks
}

// unfence removes a code fence the model wrapped its whole reply in.
func unfence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") || !strings.HasSuffix(s, "```") || strings.Count(s, "```") != 2 {
		return s
	}
	s = strings.TrimSuffix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:] // drop ``` and any language name
	}
	return strings.TrimSpace(s)
}

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

func jsonString(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
