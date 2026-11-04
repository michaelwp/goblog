package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// stubLinks makes only the given outside links "load" for the test.
func stubLinks(t *testing.T, live ...string) {
	t.Helper()
	ok := map[string]bool{}
	for _, l := range live {
		ok[l] = true
	}
	prev := linkExists
	linkExists = func(_ context.Context, link string) bool { return ok[link] }
	t.Cleanup(func() { linkExists = prev })
}

func TestVerifyLinks(t *testing.T) {
	stubLinks(t, "https://go.dev/doc/")
	body := "Read [the docs](https://go.dev/doc/) and [a made-up page](https://example.com/nope) first.\n" +
		"Also [my old link](http://author.example/kept) and [about](/about).\n" +
		"\n" +
		"![diagram](/media/a.png)\n" +
		"\n" +
		"## Setup\n" +
		"\n" +
		"### Install\n" +
		"\n" +
		"- Follow [the guide](https://example.com/bad-guide) carefully\n" +
		"\n" +
		"```bash\n" +
		"# install deps\n" +
		"# run\n" +
		"echo [x](./file)\n" +
		"```\n" +
		"\n" +
		"## Sources\n" +
		"\n" +
		"- [Go documentation](https://go.dev/doc/)\n" +
		"- [Invented paper](https://example.com/paper)\n" +
		"\n" +
		"## Related articles\n" +
		"\n" +
		"- [Ghost](/en/posts/does-not-exist)"
	lc := linkCheck{
		internal: map[string]bool{"/en/posts/hello-world": true},
		trusted:  map[string]bool{"http://author.example/kept": true, "/about": true}, // already in the author's text
		sections: []string{"Sources", "Related articles"},
	}
	got, removed := verifyLinks(t.Context(), body, lc)
	want := "Read [the docs](https://go.dev/doc/) and a made-up page first.\n" +
		"Also [my old link](http://author.example/kept) and [about](/about).\n" + // the author's links stay
		"\n" +
		"![diagram](/media/a.png)\n" +
		"\n" +
		"## Setup\n" + // a heading followed by a subheading isn't empty
		"\n" +
		"### Install\n" +
		"\n" +
		"- Follow the guide carefully\n" + // outside the end sections, the words stay
		"\n" +
		"```bash\n" + // code is untouched
		"# install deps\n" +
		"# run\n" +
		"echo [x](./file)\n" +
		"```\n" +
		"\n" +
		"## Sources\n" +
		"\n" +
		"- [Go documentation](https://go.dev/doc/)" // the bad source and the empty section are gone
	if got != want || removed != 4 {
		t.Errorf("removed %d, got:\n%s", removed, got)
	}
}

func TestPublicIP(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "192.168.1.1": false, "169.254.169.254": false,
		"100.64.0.1": false, "0.1.2.3": false, "::1": false, "fd00::1": false, "::ffff:10.0.0.1": false,
	} {
		if publicIP(net.ParseIP(addr)) != want {
			t.Errorf("publicIP(%s) != %v", addr, want)
		}
	}
	if linkClient.Transport.(*http.Transport).Proxy != nil {
		t.Error("link checks must not go through a proxy")
	}
}

func TestCheckLinkRefusesPrivateAddresses(t *testing.T) {
	for _, link := range []string{"https://127.0.0.1/", "https://10.0.0.1/", "http://example.com/", "ftp://x"} {
		if checkLinkExists(t.Context(), link) {
			t.Errorf("%s should not be fetched", link)
		}
	}
}

func TestGenerateAddsCheckedSources(t *testing.T) {
	stubLinks(t, "https://go.dev/blog/error-handling-and-go")
	reply := `{"reply": "Done.", "title": "Errors in Go", "slug": "errors", "summary": "s", "tags": ["go"], "category": "go"}
` + bodyMarker + `
Errors are values.

## Sources

- [Error handling and Go](https://go.dev/blog/error-handling-and-go)
- [Fake RFC](https://example.com/rfc-9999)

## Related articles

- [Why SSR](/en/posts/why-ssr)`
	app, _, _, got := newSuggestApp(t, reply)
	session := login(t, app)
	res := send(t, app, "POST", "/admin/generate", url.Values{"prompt": {"Go errors"}, "lang": {"en"}, "slug": {"hello-world"}}, session)
	var out generatedArticle
	if res.code != 200 || json.Unmarshal([]byte(res.body), &out) != nil {
		t.Fatalf("generate: %d %s", res.code, res.body)
	}
	if strings.Contains(out.Body, "rfc-9999") || !strings.Contains(out.Body, "go.dev/blog") || !strings.Contains(out.Body, "(/en/posts/why-ssr)") {
		t.Errorf("body:\n%s", out.Body)
	}
	if !strings.Contains(out.Reply, "removed 1 link") {
		t.Errorf("reply: %q", out.Reply)
	}
	// The AI is given the blog's articles (not the one being written) and the
	// section names in the article's language.
	if !strings.Contains(*got, `"url": "/en/posts/why-ssr"`) || strings.Contains(*got, `"url": "/en/posts/hello-world"`) {
		t.Errorf("blogArticles in prompt: %s", *got)
	}
}

// A revision keeps the author's own links, even ones the checker would fail.
func TestGenerateKeepsAuthorsLinks(t *testing.T) {
	stubLinks(t) // no outside link "loads"
	reply := `{"title": "T", "slug": "t", "summary": "s", "tags": ["go"], "category": "go"}
` + bodyMarker + `
See [my notes](http://notes.example/x) and [new](https://example.com/new).`
	app, _, _, _ := newSuggestApp(t, reply)
	session := login(t, app)
	res := send(t, app, "POST", "/admin/generate", url.Values{"prompt": {"shorter"}, "lang": {"en"}, "title": {"T"}, "body": {"See [my notes](http://notes.example/x)."}}, session)
	if res.code != 200 || !strings.Contains(res.body, "[my notes](http://notes.example/x)") || strings.Contains(res.body, "example.com/new") {
		t.Errorf("%d %s", res.code, res.body)
	}
}

func TestEndSectionNames(t *testing.T) {
	if n := endSectionNames("id"); n[0] != "Sumber" || n[1] != "Artikel terkait" {
		t.Errorf("id: %v", n)
	}
	if n := endSectionNames("en"); n[0] != "Sources" {
		t.Errorf("en: %v", n)
	}
}
