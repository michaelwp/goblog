package web

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/michaelputong/blog/backend/internal/posts"
)

func TestArticleFormatting(t *testing.T) {
	body := strings.Join([]string{
		"Some **bold**, *italic*, ~~old~~ and `code` with a [link](https://go.dev) and snake_case_name.",
		"## Section *one*",
		"### Detail",
		"> A wise quote",
		"- first\n- second **item**",
		"1. step one\n2. step two",
		"```go\nfmt.Println(\"<hi>\")\n```",
		"![A diagram](/media/abc123.png \"Figure 1\")",
		"---",
		"[bad](javascript:alert(1)) ![x](javascript:alert(1)) <script>alert(1)</script>",
	}, "\n\n")
	app := newApp(t, []posts.Post{{
		Slug: "fmt", Lang: "en", Title: "Formatting", Body: body,
		PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}})
	code, html, _ := get(t, app, "/en/posts/fmt")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	for _, want := range []string{
		"<strong>bold</strong>", "<em>italic</em>", "<del>old</del>", "<code>code</code>",
		`<a href="https://go.dev" target="_blank" rel="noopener noreferrer">link</a>`,
		"snake_case_name", // underscores inside words aren't italics
		`<h2 id="section-one">Section <em>one</em></h2>`,
		`<a href="#section-one">Section one</a>`, // contents shows plain text
		`<h3 id="detail">Detail</h3>`,
		"<blockquote><p>A wise quote</p></blockquote>",
		"<ul><li>first</li><li>second <strong>item</strong></li></ul>",
		"<ol><li>step one</li><li>step two</li></ol>",
		`<pre data-lang="go"><code>fmt.Println(&quot;&lt;hi&gt;&quot;)</code></pre>`,
		`<img src="/media/abc123.png" alt="A diagram"`,
		"<figcaption>Figure 1</figcaption>",
		"<hr/>",
		"&lt;script&gt;alert(1)&lt;/script&gt;", // raw HTML is shown as text
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, bad := range []string{`href="javascript:`, `src="javascript:`, "<script>alert"} {
		if strings.Contains(html, bad) {
			t.Errorf("unsafe output: %s", bad)
		}
	}
}

func pngFile(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func uploadReq(t *testing.T, filename string, data []byte, cookies ...*http.Cookie) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", filename)
	part.Write(data)
	w.Close()
	req := httptest.NewRequest("POST", "/admin/media", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return req
}

func TestImageUploadAndServe(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	data := pngFile(t)

	resp, _ := app.Test(uploadReq(t, "photo.png", data, session))
	var out struct{ URL, Error string }
	json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 201 || !strings.HasPrefix(out.URL, "/media/") || !strings.HasSuffix(out.URL, ".png") {
		t.Fatalf("upload: %d %+v", resp.StatusCode, out)
	}

	resp, _ = app.Test(httptest.NewRequest("GET", out.URL, nil))
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !bytes.Equal(got, data) {
		t.Fatalf("serve: status %d, %d bytes", resp.StatusCode, len(got))
	}
	for header, want := range map[string]string{
		"Content-Type":            "image/png",
		"Cache-Control":           "public, max-age=31536000, immutable",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'",
	} {
		if v := resp.Header.Get(header); v != want {
			t.Errorf("%s = %q, want %q", header, v, want)
		}
	}

	if resp, _ := app.Test(httptest.NewRequest("GET", "/media/nope.png", nil)); resp.StatusCode != 404 {
		t.Errorf("missing image: status %d", resp.StatusCode)
	}
}

func TestImageUploadRejections(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)

	// Not an image, whatever the file is called.
	resp, _ := app.Test(uploadReq(t, "evil.png", []byte(`<svg onload="alert(1)"></svg>`), session))
	if resp.StatusCode != 415 {
		t.Errorf("svg disguised as png: status %d, want 415", resp.StatusCode)
	}
	// Signed-out uploads are refused.
	resp, _ = app.Test(uploadReq(t, "photo.png", pngFile(t)))
	if resp.StatusCode != 303 {
		t.Errorf("signed-out upload: status %d, want redirect to login", resp.StatusCode)
	}
}
