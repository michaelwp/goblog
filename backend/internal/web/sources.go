package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michaelputong/blog/backend/internal/posts"
)

// AI models sometimes cite pages that don't exist. Before an AI-written
// article reaches the editor, the links the AI added are checked: links to
// this blog must point at one of its articles, and outside links must load.
// Links already in the author's own text are left alone. A failed link is
// turned back into plain text; in the Sources and Related articles sections
// at the end, an item left without a link is dropped, and so is the heading
// if nothing is left under it. Code blocks are never touched.

// markdownLink matches [text](url "title"); images, which look the same
// after a "!", are skipped by the caller.
var markdownLink = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

// linkExists reports whether an outside https link loads. A variable so
// tests can stand in for the network.
var linkExists = checkLinkExists

// blockedRanges are addresses an AI-written link must not make the server
// reach, beyond what net.IP's own checks cover.
var blockedRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),     // "this network"
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("64:ff9b::/96"),  // NAT64, which can map to private IPv4
}

func publicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, p := range blockedRanges {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// linkClient fetches only public addresses. It never uses a proxy, so the
// address checked when connecting is the site's own (behind a proxy it
// would be the proxy's, and the check would mean nothing).
var linkClient = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				if !publicIP(net.ParseIP(host)) {
					return errors.New("not a public address")
				}
				return nil
			},
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
	},
	CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

func checkLinkExists(ctx context.Context, link string) bool {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; blog-link-check/1.0)")
	req.Header.Set("Accept", "text/html,*/*;q=0.8")
	res, err := linkClient.Do(req)
	if err != nil {
		return false
	}
	_ = res.Body.Close() // only the status matters
	switch {
	case res.StatusCode < 400:
		return true
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden,
		res.StatusCode == http.StatusMethodNotAllowed, res.StatusCode == http.StatusTooManyRequests:
		return true // the page is there, but turns away automated visitors
	}
	return false // 404, 410, 5xx…
}

// linkCheck says what verifyLinks accepts without fetching, and which end
// sections it may tidy.
type linkCheck struct {
	internal map[string]bool // this blog's article paths, "/en/posts/slug"
	trusted  map[string]bool // links already in the author's text
	sections []string        // the end sections' headings, e.g. "Sources"
}

// verifyLinks checks the links the AI added to a Markdown body. It returns
// the body with broken links removed, and how many were removed. ctx should
// have time to spare: links still being checked when it ends count as broken.
func verifyLinks(ctx context.Context, body string, lc linkCheck) (string, int) {
	// The outside links to check: new ones, each once.
	var outside []string
	seen := map[string]bool{}
	for _, link := range linksIn(body) {
		if lc.trusted[link] || seen[link] || !isWebLink(link) {
			continue
		}
		seen[link] = true
		outside = append(outside, link)
	}

	ok := map[string]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, link := range outside {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			exists := linkExists(ctx, link)
			mu.Lock()
			ok[link] = exists
			mu.Unlock()
		}()
	}
	wg.Wait()

	good := func(link string) bool {
		switch {
		case lc.trusted[link]:
			return true
		case strings.HasPrefix(link, "/"):
			return lc.internal[strings.TrimSuffix(strings.SplitN(link, "#", 2)[0], "/")]
		case isWebLink(link):
			return ok[link]
		case strings.HasPrefix(link, "#"), strings.HasPrefix(link, "mailto:"):
			return true
		}
		return false
	}

	removed := 0
	inFence, inEndSection := false, false
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if isFence(line) {
			inFence = !inFence
		}
		if inFence || isFence(line) {
			lines = append(lines, line)
			continue
		}
		if level := headingLevel(line); level > 0 {
			inEndSection = level <= 2 && lc.isEndSection(line)
		}
		var b strings.Builder
		last, links, lost := 0, 0, false
		for _, m := range markdownLink.FindAllStringSubmatchIndex(line, -1) {
			if m[0] > 0 && line[m[0]-1] == '!' {
				continue // an image: left as it is
			}
			links++
			if good(line[m[4]:m[5]]) {
				continue
			}
			b.WriteString(line[last:m[0]])
			b.WriteString(line[m[2]:m[3]]) // keep the words, drop the link
			last, lost = m[1], true
			removed++
			links--
		}
		b.WriteString(line[last:])
		// In Sources or Related articles, an item left without a link was a
		// source or article that didn't check out: drop it. Elsewhere, the
		// words stay.
		if inEndSection && lost && links == 0 && isListItem(line) {
			continue
		}
		lines = append(lines, b.String())
	}
	return lc.dropEmptyEndSections(strings.Join(lines, "\n")), removed
}

// linksIn lists the links in Markdown outside code blocks, not images.
func linksIn(md string) []string {
	var out []string
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		if isFence(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range markdownLink.FindAllStringSubmatchIndex(line, -1) {
			if m[0] == 0 || line[m[0]-1] != '!' {
				out = append(out, line[m[4]:m[5]])
			}
		}
	}
	return out
}

func isWebLink(link string) bool {
	return strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "http://")
}

func isFence(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// headingLevel is 1-6 for an ATX heading ("## Title"), else 0.
func headingLevel(line string) int {
	t := strings.TrimLeft(line, " ")
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || (n < len(t) && t[n] != ' ') {
		return 0
	}
	return n
}

func headingText(line string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
}

func (lc linkCheck) isEndSection(line string) bool {
	text := headingText(line)
	for _, s := range lc.sections {
		if strings.EqualFold(text, s) {
			return true
		}
	}
	return false
}

var listItem = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)

func isListItem(line string) bool { return listItem.MatchString(line) }

// dropEmptyEndSections removes a Sources or Related articles heading with
// nothing under it before the next section (same or higher level) or the
// end. Other headings, and anything in code blocks, are left alone.
func (lc linkCheck) dropEmptyEndSections(md string) string {
	lines := strings.Split(md, "\n")
	var out []string
	inFence := false
	for i := 0; i < len(lines); i++ {
		if isFence(lines[i]) {
			inFence = !inFence
		}
		level := headingLevel(lines[i])
		if !inFence && level > 0 && level <= 2 && lc.isEndSection(lines[i]) {
			j := i + 1
			for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
				j++
			}
			empty := j == len(lines)
			if !empty {
				next := headingLevel(lines[j])
				empty = next > 0 && next <= level
			}
			if empty {
				i = j - 1 // skip the heading and its blank lines
				continue
			}
		}
		out = append(out, lines[i])
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// blogArticles lists this blog's published articles in lang, newest first,
// for the AI to link to as related reading (excluding slug, the article
// being written), and the set of their paths.
func blogArticles(all []posts.Post, lang, slug string) ([]map[string]string, map[string]bool) {
	all = slices.Clone(all)
	slices.SortFunc(all, func(a, b posts.Post) int { return b.PublishedAt.Compare(a.PublishedAt) })
	var list []map[string]string
	paths := map[string]bool{}
	for _, p := range all {
		if p.Lang != lang || p.Status != posts.Published || p.Slug == slug {
			continue
		}
		path := "/" + p.Lang + "/posts/" + p.Slug
		paths[path] = true
		if len(list) < 60 {
			list = append(list, map[string]string{"url": path, "title": p.Title, "summary": clip(p.Summary, 160)})
		}
	}
	return list, paths
}

func removedLinksNote(n int) string {
	if n == 0 {
		return ""
	}
	if n == 1 {
		return " I removed 1 link that didn't work."
	}
	return fmt.Sprintf(" I removed %d links that didn't work.", n)
}
