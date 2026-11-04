package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient is an OpenAI-like client pointed at srv.
func newTestClient(srv *httptest.Server) *Client {
	p := Detect("sk-proj-x")
	p.BaseURL = srv.URL
	return &Client{Provider: p, apiKey: "k", http: srv.Client(), discoverImages: true}
}

func reply(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// The case seen in practice: gpt-image-1 retired, dall-e-3 rejecting a
// setting. It should find a listed model, drop what's rejected, and
// remember what worked.
func TestImageAdaptsToProvider(t *testing.T) {
	var tries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			reply(w, 200, `{"data":[{"id":"gpt-4.1-mini"},{"id":"gpt-image-2-mini"},{"id":"gpt-image-2"},{"id":"dall-e-3"},{"id":"whisper-1"}]}`)
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		model, _ := req["model"].(string)
		tries = append(tries, model)
		switch {
		case model == "gpt-image-1":
			reply(w, 400, `{"error":{"message":"The model 'gpt-image-1' does not exist."}}`)
		case req["quality"] != nil:
			reply(w, 400, `{"error":{"message":"Unknown parameter: 'quality'."}}`)
		case req["size"] == "1536x1024":
			reply(w, 400, `{"error":{"message":"Invalid value: '1536x1024'. Supported values are: '1024x1024'."}}`)
		default:
			reply(w, 200, `{"data":[{"b64_json":"aGVsbG8="}]}`)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	img, err := c.GenerateImage(t.Context(), "a cat")
	if err != nil || string(img) != "hello" {
		t.Fatalf("img %q, err %v (tried %v)", img, err, tries)
	}
	if tries[0] != "gpt-image-1" || tries[1] != "gpt-image-2" {
		t.Errorf("order tried: %v", tries)
	}
	tries = nil
	if _, err := c.GenerateImage(t.Context(), "a dog"); err != nil || tries[0] != "gpt-image-2" {
		t.Errorf("second call should start with the model that worked: %v %v", tries, err)
	}
}

func TestImageNoRetryForQuota(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		reply(w, 429, `{"error":{"message":"You have no credits remaining."}}`)
	}))
	defer srv.Close()
	_, err := newTestClient(srv).GenerateImage(t.Context(), "a cat")
	if err == nil || !strings.Contains(err.Error(), "no credits") || calls != 1 {
		t.Errorf("err %v after %d calls", err, calls)
	}
}

func TestChosenImageModelIsNotReplaced(t *testing.T) {
	var models int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			models++
		}
		reply(w, 404, `{"error":{"message":"The model 'my-model' does not exist."}}`)
	}))
	defer srv.Close()
	c := New("sk-x", srv.URL, "", "my-model")
	if _, err := c.GenerateImage(t.Context(), "a cat"); err == nil || models != 0 {
		t.Errorf("err %v, listed models %d times", err, models)
	}
}

func TestIsImageModel(t *testing.T) {
	for id, want := range map[string]bool{
		"gpt-image-1": true, "dall-e-3": true, "imagen-4.0-generate-001": true, "grok-2-image": true,
		"gpt-4.1-mini": false, "dall-e-2-edit": false, "whisper-1": false,
	} {
		if isImageModel(id) != want {
			t.Errorf("isImageModel(%q) != %v", id, want)
		}
	}
}
