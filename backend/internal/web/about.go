package web

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/profile"
)

// aboutPage must match the "about" variant of PageData in frontend/src/types.ts.
type aboutPage struct {
	base
	Profile aboutView `json:"profile"`
}

// aboutView is the profile flattened for one language.
type aboutView struct {
	Name     string         `json:"name"`
	PhotoURL string         `json:"photoUrl"`
	Location string         `json:"location"`
	Country  string         `json:"country"`
	Email    string         `json:"email"`
	Links    []profile.Link `json:"links"`
	Headline string         `json:"headline"`
	Bio      string         `json:"bio"`
	TextLang string         `json:"textLang"` // language of Headline/Bio; differs from the page when falling back
	Empty    bool           `json:"empty"`
}

func (h *pages) about(c *fiber.Ctx) error {
	lang := c.Params("lang")
	if !slices.Contains(h.cfg.Languages, lang) {
		return h.notFound(c)
	}
	p, err := h.cfg.Profiles.Get(c.UserContext())
	if err != nil {
		return h.fail(c, err)
	}

	view := aboutView{
		Name: p.Name, PhotoURL: p.PhotoURL, Location: p.Location, Country: p.Country, Email: p.Email,
		Links: p.Links, TextLang: lang,
	}
	if view.Links == nil {
		view.Links = []profile.Link{}
	}
	// Use this language's text, or fall back to the first language that has some.
	for _, l := range append([]string{lang}, h.cfg.Languages...) {
		if t := p.Texts[l]; t.Headline != "" || t.Bio != "" {
			view.Headline, view.Bio, view.TextLang = t.Headline, t.Bio, l
			break
		}
	}
	view.Empty = view.Name == "" && view.Headline == "" && view.Bio == "" && view.Location == "" && view.Country == ""

	return h.render(c, fiber.StatusOK, lang, nil, aboutPage{base: newBase(c, "about", lang), Profile: view})
}

// ---- Admin -------------------------------------------------------------

// adminProfilePage must match the "adminProfile" variant of PageData.
type adminProfilePage struct {
	base
	Form      profileForm       `json:"form"`
	Errors    map[string]string `json:"errors"`
	Languages []string          `json:"languages"`
	Notice    string            `json:"notice"`
}

type profileForm struct {
	Name     string                  `json:"name"`
	PhotoURL string                  `json:"photoUrl"`
	Location string                  `json:"location"`
	Country  string                  `json:"country"`
	Email    string                  `json:"email"`
	Links    string                  `json:"links"` // one "Label | URL" per line
	Texts    map[string]profile.Text `json:"texts"`
}

func (h *pages) adminProfileForm(c *fiber.Ctx) error {
	p, err := h.cfg.Profiles.Get(c.UserContext())
	if err != nil {
		return h.fail(c, err)
	}
	lines := make([]string, len(p.Links))
	for i, l := range p.Links {
		lines[i] = l.Label + " | " + l.URL
	}
	form := profileForm{
		Name: p.Name, PhotoURL: p.PhotoURL, Location: p.Location, Country: p.Country, Email: p.Email,
		Links: strings.Join(lines, "\n"), Texts: p.Texts,
	}
	notice := ""
	if c.Query("notice") == "saved" {
		notice = "saved"
	}
	return h.renderProfile(c, fiber.StatusOK, form, nil, notice)
}

func (h *pages) adminProfileSave(c *fiber.Ctx) error {
	v := func(key string) string { return strings.TrimSpace(strings.Clone(c.FormValue(key))) }
	form := profileForm{
		Name: v("name"), PhotoURL: v("photoUrl"), Location: v("location"), Country: strings.ToUpper(v("country")), Email: v("email"),
		Links: strings.ReplaceAll(v("links"), "\r\n", "\n"),
		Texts: map[string]profile.Text{},
	}
	for _, l := range h.cfg.Languages {
		form.Texts[l] = profile.Text{
			Headline: v("headline_" + l),
			Bio:      strings.ReplaceAll(v("bio_"+l), "\r\n", "\n"),
		}
	}

	p, errs := h.validateProfile(form)
	if len(errs) > 0 {
		return h.renderProfile(c, fiber.StatusUnprocessableEntity, form, errs, "")
	}
	p.UpdatedAt = time.Now().UTC()
	if err := h.cfg.Profiles.Save(c.UserContext(), p); err != nil {
		return h.fail(c, err)
	}
	return c.Redirect("/admin/profile?notice=saved", fiber.StatusSeeOther)
}

func (h *pages) renderProfile(c *fiber.Ctx, status int, form profileForm, errs map[string]string, notice string) error {
	if errs == nil {
		errs = map[string]string{}
	}
	if form.Texts == nil {
		form.Texts = map[string]profile.Text{}
	}
	return h.render(c, status, h.cfg.Languages[0], nil, adminProfilePage{
		base: h.adminBase(c, "adminProfile"), Form: form, Errors: errs, Languages: h.cfg.Languages, Notice: notice,
	})
}

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

const maxLinks = 10

func (h *pages) validateProfile(f profileForm) (profile.Profile, map[string]string) {
	errs := map[string]string{}
	p := profile.Profile{
		Name: f.Name, PhotoURL: f.PhotoURL, Location: f.Location, Country: f.Country, Email: f.Email,
		Links: []profile.Link{}, Texts: map[string]profile.Text{},
	}

	if utf8.RuneCountInString(f.Name) > 100 {
		errs["name"] = "Keep the name under 100 characters."
	}
	if f.PhotoURL != "" {
		if msg := photoURLProblem(f.PhotoURL); msg != "" {
			errs["photoUrl"] = msg
		}
	}
	if utf8.RuneCountInString(f.Location) > 100 {
		errs["location"] = "Keep the location under 100 characters."
	}
	if f.Country != "" && !slices.Contains(countryCodes, f.Country) {
		errs["country"] = "Choose a country from the list."
	}
	if f.Email != "" && (len(f.Email) > 254 || !emailPattern.MatchString(f.Email)) {
		errs["email"] = "Enter a valid email address."
	}

	for i, line := range strings.Split(f.Links, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		label, link, ok := strings.Cut(line, "|")
		label, link = strings.TrimSpace(label), strings.TrimSpace(link)
		switch {
		case !ok || label == "" || link == "":
			errs["links"] = fmt.Sprintf("Line %d: write each link as “Label | https://…”.", i+1)
		case utf8.RuneCountInString(label) > 50:
			errs["links"] = fmt.Sprintf("Line %d: keep the label under 50 characters.", i+1)
		case !isWebURL(link) && !strings.HasPrefix(link, "mailto:"):
			// Only http(s) and mailto: are allowed, which rules out javascript: links.
			errs["links"] = fmt.Sprintf("Line %d: the address must start with https://, http:// or mailto:", i+1)
		}
		if _, bad := errs["links"]; bad {
			break
		}
		p.Links = append(p.Links, profile.Link{Label: label, URL: link})
	}
	if len(p.Links) > maxLinks {
		errs["links"] = fmt.Sprintf("Add at most %d links.", maxLinks)
	}

	for _, l := range h.cfg.Languages {
		t := f.Texts[l]
		if utf8.RuneCountInString(t.Headline) > 200 {
			errs["headline_"+l] = "Keep the headline under 200 characters."
		}
		if utf8.RuneCountInString(t.Bio) > 20000 {
			errs["bio_"+l] = "Keep the bio under 20,000 characters."
		}
		if t.Headline != "" || t.Bio != "" {
			p.Texts[l] = t
		}
	}
	return p, errs
}

var uploadedImage = regexp.MustCompile(`^/media/[0-9a-f]{24}\.(png|jpg|gif|webp)$`)

// photoURLProblem explains why u can't be used as a profile photo, or
// returns "". Share links from photo and file services open a web page
// rather than the image, so they're caught with a specific message.
func photoURLProblem(u string) string {
	if uploadedImage.MatchString(u) {
		return ""
	}
	if !isWebURL(u) {
		return "Upload a photo, or enter a full image address starting with https://"
	}
	parsed, _ := url.Parse(u)
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	switch {
	case host == "drive.google.com" || host == "docs.google.com":
		return "That's a Google Drive page, not the image itself, so browsers can't display it. Use Upload photo instead."
	case host == "photos.google.com" || host == "photos.app.goo.gl":
		return "That's a Google Photos page, not the image itself, so browsers can't display it. Use Upload photo instead."
	case host == "dropbox.com" && !strings.Contains(parsed.RawQuery, "raw=1"):
		return "That's a Dropbox page, not the image itself. Use Upload photo instead."
	}
	return ""
}

func isWebURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

// countryCodes are the ISO 3166-1 alpha-2 codes (plus XK, Kosovo) a profile
// may use. Keep in sync with frontend/src/lib/countries.ts.
var countryCodes = []string{
	"AF", "AX", "AL", "DZ", "AS", "AD", "AO", "AI", "AQ", "AG", "AR", "AM", "AW", "AU", "AT", "AZ", "BS", "BH", "BD", "BB",
	"BY", "BE", "BZ", "BJ", "BM", "BT", "BO", "BA", "BW", "BV", "BR", "IO", "VG", "BN", "BG", "BF", "BI", "KH", "CM", "CA",
	"CV", "BQ", "KY", "CF", "TD", "CL", "CN", "CX", "CC", "CO", "KM", "CG", "CD", "CK", "CR", "CI", "HR", "CU", "CW", "CY",
	"CZ", "DK", "DJ", "DM", "DO", "EC", "EG", "SV", "GQ", "ER", "EE", "SZ", "ET", "FK", "FO", "FJ", "FI", "FR", "GF", "PF",
	"TF", "GA", "GM", "GE", "DE", "GH", "GI", "GR", "GL", "GD", "GP", "GU", "GT", "GG", "GN", "GW", "GY", "HT", "HM", "HN",
	"HK", "HU", "IS", "IN", "ID", "IR", "IQ", "IE", "IM", "IL", "IT", "JM", "JP", "JE", "JO", "KZ", "KE", "KI", "XK", "KW",
	"KG", "LA", "LV", "LB", "LS", "LR", "LY", "LI", "LT", "LU", "MO", "MG", "MW", "MY", "MV", "ML", "MT", "MH", "MQ", "MR",
	"MU", "YT", "MX", "FM", "MD", "MC", "MN", "ME", "MS", "MA", "MZ", "MM", "NA", "NR", "NP", "NL", "NC", "NZ", "NI", "NE",
	"NG", "NU", "NF", "KP", "MK", "MP", "NO", "OM", "PK", "PW", "PS", "PA", "PG", "PY", "PE", "PH", "PN", "PL", "PT", "PR",
	"QA", "RE", "RO", "RU", "RW", "WS", "SM", "ST", "SA", "SN", "RS", "SC", "SL", "SG", "SX", "SK", "SI", "SB", "SO", "ZA",
	"GS", "KR", "SS", "ES", "LK", "BL", "SH", "KN", "LC", "MF", "PM", "VC", "SD", "SR", "SJ", "SE", "CH", "SY", "TW", "TJ",
	"TZ", "TH", "TL", "TG", "TK", "TO", "TT", "TN", "TR", "TM", "TC", "TV", "UM", "VI", "UG", "UA", "AE", "GB", "US", "UY",
	"UZ", "VU", "VA", "VE", "VN", "WF", "EH", "YE", "ZM", "ZW",
}
