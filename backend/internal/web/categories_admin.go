package web

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"github.com/michaelputong/blog/backend/internal/categories"
)

// adminCategoriesPage must match the "adminCategories" variant of PageData.
type adminCategoriesPage struct {
	base
	Categories []adminCategory         `json:"list"` // not "categories": that name is the public menu list
	Languages  []string                `json:"languages"`
	Form       categoryForm            `json:"form"`      // the "add category" form
	Errors     map[string]string       `json:"errors"`    // for the add form: "slug", "name_<lang>"
	RowErrors  map[string]string       `json:"rowErrors"` // rename errors, by category slug
	RowForms   map[string]categoryForm `json:"rowForms"`  // rejected rename input, by slug
	Notice     string                  `json:"notice"`
}

type adminCategory struct {
	Slug     string            `json:"slug"`
	Names    map[string]string `json:"names"`
	Articles int               `json:"articles"` // articles (not translations) in it, any status
}

type categoryForm struct {
	Slug  string            `json:"slug"`
	Names map[string]string `json:"names"`
}

const maxCategoryName = 50

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// slugFromName makes "Web & Cloud" into "web-cloud".
func slugFromName(name string) string {
	return strings.Trim(nonSlugChars.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

func (h *pages) readCategoryForm(c *fiber.Ctx) categoryForm {
	f := categoryForm{Slug: strings.ToLower(strings.TrimSpace(strings.Clone(c.FormValue("slug")))), Names: map[string]string{}}
	for _, lang := range h.cfg.Languages {
		f.Names[lang] = strings.Join(strings.Fields(strings.Clone(c.FormValue("name_"+lang))), " ")
	}
	return f
}

// validateCategory checks names (required in every language) and, when
// creating, the slug, which defaults to one made from the first name.
func (h *pages) validateCategory(f *categoryForm, creating bool) map[string]string {
	errs := map[string]string{}
	for _, lang := range h.cfg.Languages {
		switch n := utf8.RuneCountInString(f.Names[lang]); {
		case n == 0:
			errs["name_"+lang] = "Enter a name."
		case n > maxCategoryName:
			errs["name_"+lang] = fmt.Sprintf("Keep the name under %d characters.", maxCategoryName)
		}
	}
	if creating {
		if f.Slug == "" {
			f.Slug = slugFromName(f.Names[h.cfg.Languages[0]])
		}
		if f.Slug == "" || len(f.Slug) > 50 || !slugPattern.MatchString(f.Slug) {
			errs["slug"] = "Use lowercase letters, numbers and single hyphens (e.g. web-security)."
		}
	}
	return errs
}

// nameTakenMessage turns a uniqueness error into a message for a form field.
func (h *pages) nameTakenMessage(err error) (field, msg string, ok bool) {
	var taken *categories.NameTakenError
	if errors.As(err, &taken) {
		return "name_" + taken.Lang, fmt.Sprintf("“%s” is already used by another category.", taken.Name), true
	}
	return "", "", false
}

func (h *pages) adminCategories(c *fiber.Ctx) error {
	notice := c.Query("notice")
	switch notice {
	case "created", "renamed", "deleted":
	default:
		notice = ""
	}
	return h.renderCategories(c, fiber.StatusOK, categoryForm{}, nil, nil, nil, notice)
}

func (h *pages) renderCategories(c *fiber.Ctx, code int, form categoryForm, errs, rowErrs map[string]string, rowForms map[string]categoryForm, notice string) error {
	cats, err := h.cfg.Categories.List(c.UserContext(), h.cfg.Languages[0])
	if err != nil {
		return h.fail(c, err)
	}
	all, err := h.cfg.Store.All(c.UserContext())
	if err != nil {
		return h.fail(c, err)
	}
	articles := map[string]map[string]bool{} // category → article slugs
	for _, p := range all {
		if articles[p.Category] == nil {
			articles[p.Category] = map[string]bool{}
		}
		articles[p.Category][p.Slug] = true
	}
	list := make([]adminCategory, len(cats))
	for i, cat := range cats {
		list[i] = adminCategory{Slug: cat.Slug, Names: cat.Names, Articles: len(articles[cat.Slug])}
	}
	if form.Names == nil {
		form.Names = map[string]string{}
	}
	for _, m := range []*map[string]string{&errs, &rowErrs} {
		if *m == nil {
			*m = map[string]string{}
		}
	}
	if rowForms == nil {
		rowForms = map[string]categoryForm{}
	}
	return h.render(c, code, h.cfg.Languages[0], nil, adminCategoriesPage{
		base: h.adminBase(c, "adminCategories"), Categories: list, Languages: h.cfg.Languages,
		Form: form, Errors: errs, RowErrors: rowErrs, RowForms: rowForms, Notice: notice,
	})
}

// POST /admin/categories
func (h *pages) adminCreateCategory(c *fiber.Ctx) error {
	f := h.readCategoryForm(c)
	errs := h.validateCategory(&f, true)
	if len(errs) == 0 {
		err := h.cfg.Categories.Create(c.UserContext(), categories.Category{Slug: f.Slug, Names: f.Names, CreatedAt: time.Now().UTC()})
		if field, msg, ok := h.nameTakenMessage(err); ok {
			errs[field] = msg
		} else if errors.Is(err, categories.ErrSlugTaken) {
			errs["slug"] = "A category with this slug already exists."
		} else if err != nil {
			return h.fail(c, err)
		}
	}
	if len(errs) > 0 {
		return h.renderCategories(c, fiber.StatusUnprocessableEntity, f, errs, nil, nil, "")
	}
	return c.Redirect("/admin/categories?notice=created", fiber.StatusSeeOther)
}

// POST /admin/categories/:slug
func (h *pages) adminRenameCategory(c *fiber.Ctx) error {
	f := h.readCategoryForm(c)
	f.Slug = strings.Clone(c.Params("slug")) // a category's slug never changes
	errs := h.validateCategory(&f, false)
	if len(errs) == 0 {
		err := h.cfg.Categories.Rename(c.UserContext(), categories.Category{Slug: f.Slug, Names: f.Names})
		if errors.Is(err, categories.ErrNotFound) {
			return h.notFound(c)
		}
		if _, msg, ok := h.nameTakenMessage(err); ok {
			errs["name"] = msg
		} else if err != nil {
			return h.fail(c, err)
		}
	}
	if len(errs) > 0 {
		msg := errs["name"]
		for _, lang := range h.cfg.Languages {
			if m := errs["name_"+lang]; msg == "" && m != "" {
				msg = m
			}
		}
		return h.renderCategories(c, fiber.StatusUnprocessableEntity, categoryForm{}, nil,
			map[string]string{f.Slug: msg}, map[string]categoryForm{f.Slug: f}, "")
	}
	return c.Redirect("/admin/categories?notice=renamed", fiber.StatusSeeOther)
}

// POST /admin/categories/:slug/delete. Its articles are kept and become
// uncategorized.
func (h *pages) adminDeleteCategory(c *fiber.Ctx) error {
	slug := strings.Clone(c.Params("slug"))
	if _, err := h.cfg.Categories.Get(c.UserContext(), slug); errors.Is(err, categories.ErrNotFound) {
		return c.Redirect("/admin/categories", fiber.StatusSeeOther)
	} else if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.cfg.Store.ReassignCategory(c.UserContext(), slug, ""); err != nil {
		return h.fail(c, err)
	}
	if err := h.cfg.Categories.Delete(c.UserContext(), slug); err != nil && !errors.Is(err, categories.ErrNotFound) {
		return h.fail(c, err)
	}
	return c.Redirect("/admin/categories?notice=deleted", fiber.StatusSeeOther)
}
