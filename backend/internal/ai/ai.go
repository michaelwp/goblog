// Package ai talks to whichever AI provider the configured API key belongs
// to. Almost every provider offers an OpenAI-compatible chat completions
// endpoint, so one client covers them all: the provider is recognised from the
// key's prefix, and AI_BASE_URL / AI_MODEL override the endpoint and model for
// anything else (Mistral, DeepSeek, a local Ollama, a proxy, ...).
package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Provider is an OpenAI-compatible endpoint and sensible default models.
type Provider struct {
	Name    string
	BaseURL string // the chat completions URL is BaseURL + "/chat/completions"
	Model   string
	// ImageModel generates images at BaseURL + "/images/generations"; ""
	// when the provider can't.
	ImageModel string
	// ImageFallback is tried when ImageModel is refused (OpenAI's
	// gpt-image-1 needs a verified organization; dall-e-3 doesn't).
	ImageFallback string
}

// providers, matched by key prefix in order (most specific first).
var providers = []struct {
	prefix string
	Provider
}{
	{"sk-ant-", Provider{"Anthropic", "https://api.anthropic.com/v1", "claude-haiku-5-5", "", ""}},
	{"sk-or-", Provider{"OpenRouter", "https://openrouter.ai/api/v1", "openai/gpt-4.1-mini", "", ""}},
	{"gsk_", Provider{"Groq", "https://api.groq.com/openai/v1", "llama-3.3-70b-versatile", "", ""}},
	{"xai-", Provider{"xAI", "https://api.x.ai/v1", "grok-3-mini", "grok-2-image", ""}},
	{"AIza", Provider{"Google Gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "gemini-2.5-flash", "imagen-3.0-generate-002", ""}},
	{"sk-", Provider{"OpenAI", "https://api.openai.com/v1", "gpt-4.1-mini", "gpt-image-1", "dall-e-3"}},
}

// Detect returns the provider a key belongs to, defaulting to OpenAI.
func Detect(key string) Provider {
	for _, p := range providers {
		if strings.HasPrefix(key, p.prefix) {
			return p.Provider
		}
	}
	return providers[len(providers)-1].Provider
}

type Client struct {
	Provider Provider
	apiKey   string
	http     *http.Client
	// discoverImages lets GenerateImage look for another image model when
	// the default one isn't available (not when AI_IMAGE_MODEL chose it).
	discoverImages bool

	mu                sync.Mutex
	workingImageModel string // the image model that last worked
}

// New returns a client for key, or nil when key is empty (AI features off).
// baseURL, model and imageModel override the detected provider's when set.
// A custom baseURL generates images only when imageModel is given.
func New(key, baseURL, model string, imageModel ...string) *Client {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	p := Detect(key)
	if baseURL != "" {
		p.Name, p.BaseURL, p.ImageModel, p.ImageFallback = "custom", strings.TrimRight(baseURL, "/"), "", ""
	}
	if model != "" {
		p.Model = model
	}
	discover := p.ImageModel != ""
	if len(imageModel) > 0 && imageModel[0] != "" {
		p.ImageModel, p.ImageFallback, discover = imageModel[0], "", false // the author's choice: no silent substitute
	}
	// Image generation can take a while.
	return &Client{Provider: p, apiKey: key, http: &http.Client{Timeout: 120 * time.Second}, discoverImages: discover}
}

// CanGenerateImages reports whether the provider has an image model.
func (c *Client) CanGenerateImages() bool { return c != nil && c.Provider.ImageModel != "" }

// Complete sends a system and a user message and returns the reply text.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	return c.CompleteLong(ctx, system, user, 2000)
}

// CompleteLong is Complete for replies up to maxTokens long, such as a
// translated article.
func (c *Client) CompleteLong(ctx context.Context, system, user string, maxTokens int) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": c.Provider.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.2,
		"max_tokens":  maxTokens, // with room for models that think before answering
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Provider.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	raw, status, err := c.post(req)
	if err != nil {
		return "", err
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &out)
	if status >= 300 {
		return "", c.apiError(c.Provider.Model, status, raw)
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("%s returned no answer", c.Provider.Name)
	}
	return out.Choices[0].Message.Content, nil
}

// CompleteJSON is Complete for a reply that must be a JSON object, decoded
// into v. Models sometimes wrap JSON in prose or ``` fences, so it decodes
// the outermost {...} in the reply.
func (c *Client) CompleteJSON(ctx context.Context, system, user string, v any) error {
	text, err := c.Complete(ctx, system, user)
	if err != nil {
		return err
	}
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return errors.New("the AI reply wasn't JSON")
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), v); err != nil {
		return fmt.Errorf("the AI reply wasn't valid JSON: %w", err)
	}
	return nil
}

// post sends req with the key and returns the response body (up to 32 MB,
// enough for a generated image) and status.
func (c *Client) post(req *http.Request) ([]byte, int, error) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", c.Provider.Name, err)
	}
	defer func() { _ = res.Body.Close() }() // read-only: a close error can't lose data
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	return raw, res.StatusCode, err
}

func (c *Client) apiError(model string, status int, raw []byte) error {
	var out struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &out)
	msg := strings.TrimSpace(string(raw))
	if out.Error != nil && out.Error.Message != "" {
		msg = out.Error.Message
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fmt.Errorf("%s (%s) returned %d: %s", c.Provider.Name, model, status, msg)
}

// GenerateImage makes one wide image from prompt and returns its bytes.
//
// Providers rename and retire image models, and change which settings they
// accept, so it adapts rather than trusting a fixed list: settings the API
// says it doesn't know are dropped and the request retried, and when the
// model isn't available, it asks the provider which image models it has
// (GET /models) and tries those, newest first. The model that works is
// remembered. A model chosen with AI_IMAGE_MODEL is used as is.
func (c *Client) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	if !c.CanGenerateImages() {
		return nil, errors.New("this AI provider can't generate images")
	}
	c.mu.Lock()
	first := c.workingImageModel
	c.mu.Unlock()
	if first == "" {
		first = c.Provider.ImageModel
	}
	candidates := []string{first}
	tried := map[string]bool{}
	var refusals []string
	for i := 0; i < len(candidates) && len(tried) < 5; i++ {
		model := candidates[i]
		if tried[model] {
			continue
		}
		tried[model] = true
		img, err := c.generateImage(ctx, model, prompt)
		if err == nil {
			c.mu.Lock()
			c.workingImageModel = model
			c.mu.Unlock()
			return img, nil
		}
		// The first model must be unavailable to look further; a discovered
		// one may fail for any reason (it might not even make images), so
		// the next is tried, unless time ran out.
		var unavailable *unavailableError
		if (i == 0 && !errors.As(err, &unavailable)) || ctx.Err() != nil {
			return nil, withRefusals(err, refusals)
		}
		refusals = append(refusals, err.Error())
		if i == 0 && c.discoverImages {
			candidates = append(candidates, c.imageModels(ctx)...)
			if c.Provider.ImageFallback != "" {
				candidates = append(candidates, c.Provider.ImageFallback)
			}
		}
	}
	if len(refusals) == 0 {
		return nil, errors.New("no image model to try")
	}
	return nil, fmt.Errorf("no image model worked for this key: %s", strings.Join(refusals, "; "))
}

func withRefusals(err error, refusals []string) error {
	if len(refusals) == 0 {
		return err
	}
	return fmt.Errorf("%w (after: %s)", err, strings.Join(refusals, "; "))
}

// unavailableError means the model can't be used with this key (unknown,
// retired, or not allowed), as opposed to a network, quota or prompt problem.
type unavailableError struct{ msg string }

func (e *unavailableError) Error() string { return e.msg }

var (
	modelUnavailable = regexp.MustCompile(`(?i)does not exist|not found|model_not_found|no such model|must be verified|not have access|not supported|deprecated`)
	// "Unknown parameter: 'response_format'", "Unsupported parameter: 'quality'",
	// "Invalid value: '1792x1024'": the quoted name or value to drop.
	rejectedSetting = regexp.MustCompile(`(?i)(?:unknown|unsupported|unrecognized|invalid)[^']{0,40}'([^']+)'`)
)

// optionalImageSettings may be dropped when a provider rejects them; the
// model and prompt are always sent.
var optionalImageSettings = []string{"size", "quality", "output_format", "response_format", "n", "style"}

func (c *Client) generateImage(ctx context.Context, model, prompt string) ([]byte, error) {
	params := map[string]any{"model": model, "prompt": prompt, "n": 1}
	switch {
	case strings.HasPrefix(model, "gpt-image"):
		// JPEG keeps a wide image well under the upload limit.
		params["size"], params["quality"], params["output_format"] = "1536x1024", "medium", "jpeg"
	case strings.HasPrefix(model, "dall-e-3"):
		params["size"], params["quality"] = "1792x1024", "standard"
	}

	for attempt := 0; ; attempt++ {
		body, _ := json.Marshal(params)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Provider.BaseURL+"/images/generations", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		raw, status, err := c.post(req)
		if err != nil {
			return nil, err
		}
		if status < 300 {
			return c.imageFromReply(ctx, raw)
		}
		apiErr := c.apiError(model, status, raw)
		if status == http.StatusBadRequest && attempt < len(optionalImageSettings) {
			if key := rejectedKey(apiErr.Error(), params); key != "" {
				delete(params, key) // the provider doesn't take it: try without
				continue
			}
		}
		if status == http.StatusNotFound || status == http.StatusForbidden || (status == http.StatusBadRequest && modelUnavailable.MatchString(apiErr.Error())) {
			return nil, &unavailableError{msg: apiErr.Error()}
		}
		return nil, apiErr
	}
}

// rejectedKey finds the optional setting an error complains about, by its
// name or its value.
func rejectedKey(msg string, params map[string]any) string {
	m := rejectedSetting.FindStringSubmatch(msg)
	if m == nil {
		return ""
	}
	for _, key := range optionalImageSettings {
		if _, ok := params[key]; ok && (m[1] == key || fmt.Sprint(params[key]) == m[1]) {
			return key
		}
	}
	return ""
}

// imageModels asks the provider for its image models, best first: newer
// names before older, full models before "mini" ones. It returns nothing if
// the provider doesn't list models.
func (c *Client) imageModels(ctx context.Context) []string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Provider.BaseURL+"/models", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	res, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = res.Body.Close() }() // read-only: a close error can't lose data
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if res.StatusCode >= 300 || json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&out) != nil {
		return nil
	}
	var models []string
	for _, m := range out.Data {
		id := strings.TrimPrefix(m.ID, "models/") // Gemini lists "models/imagen-..."
		if isImageModel(id) {
			models = append(models, id)
		}
	}
	family := func(id string) int {
		for i, prefix := range []string{"gpt-image", "imagen", "grok", "dall-e"} {
			if strings.HasPrefix(id, prefix) {
				return i
			}
		}
		return 9
	}
	sort.SliceStable(models, func(i, j int) bool {
		a, b := models[i], models[j]
		if family(a) != family(b) {
			return family(a) < family(b)
		}
		if ma, mb := strings.Contains(a, "mini"), strings.Contains(b, "mini"); ma != mb {
			return mb
		}
		return a > b // newer version numbers sort later in the alphabet
	})
	return models
}

func isImageModel(id string) bool {
	id = strings.ToLower(id)
	if strings.Contains(id, "edit") || strings.Contains(id, "variation") || strings.Contains(id, "vision") {
		return false
	}
	return strings.HasPrefix(id, "gpt-image") || strings.HasPrefix(id, "dall-e") || strings.HasPrefix(id, "imagen") ||
		(strings.Contains(id, "image") && !strings.Contains(id, "image-preview-text"))
}

// imageFromReply returns the image in an images/generations reply, as
// base64 or downloaded from its URL.
func (c *Client) imageFromReply(ctx context.Context, raw []byte) ([]byte, error) {
	var out struct {
		Data []struct {
			B64 string `json:"b64_json"`
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Data) == 0 {
		return nil, fmt.Errorf("%s returned no image", c.Provider.Name)
	}
	if b64 := out.Data[0].B64; b64 != "" {
		return base64.StdEncoding.DecodeString(b64)
	}
	if u := out.Data[0].URL; strings.HasPrefix(u, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		res, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = res.Body.Close() }() // read-only: a close error can't lose data
		return io.ReadAll(io.LimitReader(res.Body, 32<<20))
	}
	return nil, fmt.Errorf("%s returned no image", c.Provider.Name)
}
