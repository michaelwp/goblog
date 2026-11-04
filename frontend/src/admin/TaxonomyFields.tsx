// The editor's category and tags. The category can be a new one ("New: …",
// usually suggested by the AI), created when the article is saved; its
// names stay editable until then.
import type { ReactNode } from "react";

import { getDictionary, type Locale } from "../lib/i18n";
import type { CategoryOption } from "../types";
import { TagInput } from "./TagInput";

export const NEW_CATEGORY = "__new"; // mirrors newCategoryValue in the Go backend

export type Taxonomy = {
  tags: string[];
  category: string; // a category slug, NEW_CATEGORY or ""
  newCategory: Record<string, string> | null; // names by language
  // A category autosave already created for newCategory, whose names were
  // changed while it saved: the server renames it rather than making another.
  replaces?: string;
};

type Props = {
  value: Taxonomy;
  onChange: (next: Taxonomy) => void;
  categories: CategoryOption[];
  knownTags: string[];
  languages: Locale[];
  errors: { category?: string; tags?: string };
  tagsAction?: ReactNode; // the AI button beside Tags, and its message
  tagsStatus?: ReactNode;
};

export function TaxonomyFields({ value, onChange, categories, knownTags, languages, errors, tagsAction, tagsStatus }: Props) {
  const { tags, category, newCategory } = value;
  const isNew = category === NEW_CATEGORY && newCategory;
  const defaultLang = languages[0];
  const setName = (lang: string, name: string) => onChange({ ...value, newCategory: { ...newCategory, [lang]: name } });

  return (
    <>
      <div className="field-row field-row-2">
        <label className={errors.category ? "field has-error" : "field"}>
          <span className="field-label">Category</span>
          <select name="category" value={category} onChange={(e) => onChange({ ...value, category: e.target.value })}>
            <option value="">No category</option>
            {categories.map((c) => (
              <option key={c.slug} value={c.slug}>
                {c.name}
              </option>
            ))}
            {newCategory && <option value={NEW_CATEGORY}>New: {newCategory[defaultLang] || "unnamed"}</option>}
          </select>
          {errors.category ? (
            <span className="field-error">{errors.category}</span>
          ) : (
            <span className="field-hint">
              {isNew
                ? "A new category, created when you save. Rename it below."
                : categories.length
                  ? "Shared by all translations of this article."
                  : "No categories yet: add them under Categories."}
            </span>
          )}
        </label>
        {/* Not a <label>: it would forward clicks to the chips' buttons. */}
        <div className={errors.tags ? "field has-error" : "field"}>
          <span className="field-label-row">
            <span className="field-label">Tags</span>
            {tagsAction}
          </span>
          <TagInput tags={tags} onChange={(next) => onChange({ ...value, tags: next })} known={knownTags} invalid={!!errors.tags} />
          <span className={errors.tags ? "field-error" : "field-hint"}>{errors.tags || "Up to 10, shared by all translations. Press Enter or comma after each."}</span>
          {tagsStatus}
        </div>
      </div>

      {isNew && (
        <fieldset className="new-category">
          <legend>New category</legend>
          {value.replaces && <input type="hidden" name="newCategoryReplaces" value={value.replaces} />}
          <div className="field-row field-row-2">
            {languages.map((l) => (
              <label key={l} className="field">
                <span className="field-label">Name ({getDictionary(l).languageName})</span>
                <input
                  name={`newCategory_${l}`}
                  value={newCategory[l] ?? ""}
                  onChange={(e) => setName(l, e.target.value)}
                  maxLength={50}
                  required={l === defaultLang}
                  autoComplete="off"
                />
              </label>
            ))}
          </div>
        </fieldset>
      )}

    </>
  );
}
