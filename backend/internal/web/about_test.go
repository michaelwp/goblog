package web

import (
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestAboutPageEmpty(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	res := send(t, app, "GET", "/id/about", nil)
	if res.code != 200 || !strings.Contains(res.body, "Pemilik blog belum menulis profil.") {
		t.Errorf("empty about page: status %d", res.code)
	}
}

func TestProfileEditAndAboutPage(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)

	form := url.Values{
		"name": {"Ada Lovelace"}, "location": {"London"}, "email": {"ada@example.com"},
		"photoUrl":    {"https://example.com/ada.jpg"},
		"links":       {"GitHub | https://github.com/ada\r\n\r\nWebsite | https://ada.dev"},
		"headline_en": {"Writes about Go."}, "bio_en": {"Hello.\r\n\r\n## Work\r\n\r\nI build things."},
		"headline_id": {""}, "bio_id": {""},
	}
	res := send(t, app, "POST", "/admin/profile", form, session)
	if res.code != 303 || res.location != "/admin/profile?notice=saved" {
		t.Fatalf("save: %d → %q\n%s", res.code, res.location, res.body)
	}

	res = send(t, app, "GET", "/en/about", nil)
	for _, want := range []string{
		"<title>Ada Lovelace – GoBlog.dev</title>",
		`class="profile-lead">Writes about Go.</p>`,
		`<h2 id="work">Work</h2>`,
		`<a href="#work">Work</a>`, // contents
		`src="https://example.com/ada.jpg"`,
		`href="mailto:ada@example.com"`,
		`href="https://ada.dev"`,
	} {
		if !strings.Contains(res.body, want) {
			t.Errorf("/en/about missing %s", want)
		}
	}

	// No Indonesian text: falls back to English, with a note and lang="en".
	res = send(t, app, "GET", "/id/about", nil)
	for _, want := range []string{"belum diterjemahkan", `<div class="article-body" lang="en">`, "Writes about Go."} {
		if !strings.Contains(res.body, want) {
			t.Errorf("/id/about missing %s", want)
		}
	}

	// The admin form shows the saved values.
	res = send(t, app, "GET", "/admin/profile", nil, session)
	if !strings.Contains(res.body, "GitHub | https://github.com/ada\nWebsite | https://ada.dev") {
		t.Errorf("admin form doesn't show saved links")
	}
}

func TestProfileValidation(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	cases := map[string]url.Values{
		"Line 1: the address must start with":   {"links": {"Evil | javascript:alert(1)"}},
		"Line 2: write each link as":            {"links": {"Ok | https://ok.dev\nno separator"}},
		"Upload a photo, or enter a full image": {"photoUrl": {"javascript:alert(1)"}},
		"Enter a valid email address.":          {"email": {"not-an-email"}},
		"Add at most 10 links.":                 {"links": {strings.Repeat("L | https://x.dev\n", 11)}},
	}
	for want, form := range cases {
		res := send(t, app, "POST", "/admin/profile", form, session)
		if res.code != 422 || !strings.Contains(res.body, want) {
			t.Errorf("%v: status %d, want 422 with %q", form, res.code, want)
		}
	}
	// Nothing was saved.
	if res := send(t, app, "GET", "/en/about", nil); strings.Contains(res.body, "javascript:") {
		t.Error("invalid link was saved")
	}
}

func TestProfileRequiresLogin(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	res := send(t, app, "POST", "/admin/profile", url.Values{"name": {"Mallory"}})
	if res.code != 303 || res.location != "/admin/login" {
		t.Errorf("unauthenticated save: %d → %q", res.code, res.location)
	}
	if res := send(t, app, "GET", "/en/about", nil); strings.Contains(res.body, "Mallory") {
		t.Error("unauthenticated save changed the profile")
	}
}

func TestProfilePhotoAddresses(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)
	cases := map[string]string{ // photoUrl → expected error ("" = accepted)
		"/media/6ab9483ca9cd32743a3b7848.jpg":                                                "",
		"https://avatars.githubusercontent.com/u/1?v=4":                                      "",
		"https://www.dropbox.com/s/abc/me.jpg?raw=1":                                         "",
		"https://drive.google.com/file/d/1MaSWL4xnQKzcuaBmUmkEnb9MaZUZWaFl/view?usp=sharing": "Google Drive page",
		"https://photos.app.goo.gl/abc":                                                      "Google Photos page",
		"https://www.dropbox.com/s/abc/me.jpg?dl=0":                                          "Dropbox page",
		"/media/../../etc/passwd":                                                            "Upload a photo",
		"javascript:alert(1)":                                                                "Upload a photo",
	}
	for photo, wantErr := range cases {
		res := send(t, app, "POST", "/admin/profile", url.Values{"name": {"Ada"}, "photoUrl": {photo}}, session)
		switch {
		case wantErr == "" && res.code != 303:
			t.Errorf("%s: status %d, want accepted", photo, res.code)
		case wantErr != "" && (res.code != 422 || !strings.Contains(res.body, wantErr)):
			t.Errorf("%s: status %d, want 422 mentioning %q", photo, res.code, wantErr)
		}
	}
}

func TestCountryListsMatch(t *testing.T) {
	src, err := os.ReadFile("../../../frontend/src/lib/countries.ts")
	if err != nil {
		t.Skipf("frontend source not available: %v", err)
	}
	var ts []string
	for _, m := range regexp.MustCompile(`(?m)^  \["([A-Z]{2})",`).FindAllStringSubmatch(string(src), -1) {
		ts = append(ts, m[1])
	}
	goCodes := slices.Sorted(slices.Values(countryCodes))
	slices.Sort(ts)
	if !slices.Equal(goCodes, ts) || len(ts) != 250 {
		t.Errorf("country lists differ: Go has %d, frontend has %d", len(goCodes), len(ts))
	}
}

func TestProfileCountry(t *testing.T) {
	app, _ := newAdminApp(t, testPassword)
	session := login(t, app)

	res := send(t, app, "POST", "/admin/profile", url.Values{"name": {"Ada"}, "country": {"XX"}}, session)
	if res.code != 422 || !strings.Contains(res.body, "Choose a country from the list.") {
		t.Errorf("invalid country: status %d", res.code)
	}

	send(t, app, "POST", "/admin/profile", url.Values{"name": {"Ada"}, "location": {"Jakarta"}, "country": {"id"}}, session)
	for lang, name := range map[string]string{"en": "Indonesia", "id": "Indonesia"} {
		body := send(t, app, "GET", "/"+lang+"/about", nil).body
		if !strings.Contains(body, `"country":"ID"`) || !strings.Contains(body, "🇮🇩") || !strings.Contains(body, `aria-label="`+name+`"`) {
			t.Errorf("/%s/about doesn't show the flag", lang)
		}
	}
}
