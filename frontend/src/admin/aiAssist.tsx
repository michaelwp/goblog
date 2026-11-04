// AI help in the article editor, only when the author clicks; nothing is
// filled in by itself. "Suggest with AI" proposes a title, slug, summary,
// tags, a category and a cover at once: empty fields are filled, and a
// different title or summary is offered next to the field instead of
// replacing what the author wrote. The ✨ button beside a field does just
// that one: writes the title, summary or slug, adds tags, makes a cover, or
// translates the text from another language. Everything stays editable.
import { type RefObject, useEffect, useRef, useState } from "react";

import { getDictionary, type Locale } from "../lib/i18n";
import { MAX_TAG_COUNT } from "./TagInput";
import { NEW_CATEGORY, type Taxonomy } from "./TaxonomyFields";

type Suggestion = {
  title: string;
  summary: string;
  tags: string[];
  category: string;
  newCategory?: Record<string, string>;
  cover: string;
};

export type Offers = { title?: string; summary?: string };
// A field with its own ✨ button; its messages show beside the button.
export type AIField = "title" | "summary" | "tags" | "slug" | "cover" | "body";
type Status = { kind: "info" | "error"; text: string; field?: AIField } | null;

// Sets a field the way typing would, so React's onChange handlers and
// autosave both notice.
function setField(form: HTMLFormElement, name: string, value: string) {
  const el = form.elements.namedItem(name);
  if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement)) return;
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(proto, "value")?.set?.call(el, value);
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

function fieldValue(form: HTMLFormElement, name: string): string {
  return String(new FormData(form).get(name) ?? "").trim();
}

// The article as the AI endpoints read it. lang is the language being
// written: once the article exists the form has no "lang" field (the
// language switcher replaces the dropdown), so it's passed in.
function articleBody(form: HTMLFormElement, lang: string): URLSearchParams {
  const body = new URLSearchParams();
  for (const key of ["title", "summary", "body"]) body.set(key, fieldValue(form, key));
  body.set("lang", lang);
  return body;
}

async function post<T>(url: string, body: URLSearchParams): Promise<T & { error?: string; ok: boolean }> {
  const res = await fetch(url, { method: "POST", body, credentials: "same-origin", headers: { Accept: "application/json" } });
  const out = (await res.json().catch(() => ({ error: "Unexpected answer from the server." }))) as T & { error?: string };
  return { ...out, ok: res.ok };
}

function list(items: string[]): string {
  return items.length <= 1 ? items.join("") : `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

export function useAIAssist(opts: {
  formRef: RefObject<HTMLFormElement | null>;
  taxonomy: Taxonomy;
  setTaxonomy: (t: Taxonomy) => void;
  cover: string;
  setCover: (url: string) => void;
  fillSlug: (title: string) => void; // make the slug from the title, if it's the AI's to set
  slug: string; // the article, for translating one of its other languages
  lang: string; // the language being written (the Language dropdown's choice on a new article)
  setBody: (markdown: string) => void;
  canGenerateImages: boolean; // the provider makes images: Suggest also generates a cover
  setSlug: (slug: string) => boolean; // sets the slug; false once it can't change (the article is saved)
}) {
  const { formRef } = opts;
  // What's running: "" | "text" | "translate" | an AIField.
  const [busy, setBusy] = useState("");
  const [status, setStatus] = useState<Status>(null);
  const [offers, setOffers] = useState<Offers>({});

  // Latest state for async replies, which may land after more edits.
  const latest = useRef(opts);
  useEffect(() => {
    latest.current = opts;
  });

  async function suggest() {
    const form = formRef.current;
    if (!form || busy) return;
    setBusy("text");
    setStatus({ kind: "info", text: "Asking the AI for a title, summary, tags, category and cover…" });
    try {
      const out = await post<Partial<Suggestion>>("/admin/suggest", articleBody(form, latest.current.lang));
      if (!out.ok) {
        setStatus({ kind: "error", text: out.error ?? "The AI couldn't make suggestions." });
        return;
      }
      const { taxonomy: cur, cover, setTaxonomy, setCover } = latest.current;
      const filled: string[] = [];
      const offered: Offers = {};

      for (const field of ["title", "summary"] as const) {
        const value = out[field]?.trim();
        if (!value) continue;
        const now = fieldValue(form, field);
        if (!now) {
          setField(form, field, value);
          filled.push(field);
        } else if (now !== value) {
          offered[field] = value;
        }
      }

      const title = fieldValue(form, "title");
      if (title) latest.current.fillSlug(title);

      const next: Taxonomy = { ...cur };
      const tags = [...new Set([...cur.tags, ...(out.tags ?? [])])].slice(0, MAX_TAG_COUNT);
      const added = tags.length - cur.tags.length;
      if (added > 0) filled.push(`${added} tag${added === 1 ? "" : "s"}`);
      next.tags = tags;
      if (out.category && out.category !== cur.category) {
        Object.assign(next, { category: out.category, newCategory: null });
        filled.push("a category");
      } else if (!out.category && out.newCategory) {
        Object.assign(next, { category: NEW_CATEGORY, newCategory: out.newCategory });
        filled.push("a new category");
      }
      setTaxonomy(next);
      // Without a cover: generate one if the provider can (below), else
      // use the best image in the article.
      const generateCover = !cover && latest.current.canGenerateImages;
      if (!cover && !generateCover && out.cover) {
        setCover(out.cover);
        filled.push("a cover from the article's images");
      }

      setOffers(offered);
      const offeredText = Object.keys(offered).length ? ` A different ${list(Object.keys(offered))} is suggested below the field.` : "";
      const done = filled.length ? `AI filled in ${list(filled)}.` : "Nothing new to fill in.";
      if (generateCover) {
        setBusy("cover");
        setStatus({ kind: "info", text: `${done} Now generating a cover image… this can take up to a minute.` });
        const cover = await makeCover(form);
        setStatus({ kind: cover.ok ? "info" : "error", text: `${done} ${cover.text}${offeredText}` });
        return;
      }
      setStatus({ kind: "info", text: `${done} Change anything you like.${offeredText}` });
    } catch {
      setStatus({ kind: "error", text: "Couldn't reach the server for suggestions." });
    } finally {
      setBusy("");
    }
  }

  // Generates (or picks) a cover and puts it in the cover field, where it
  // can be removed or replaced like an upload.
  async function makeCover(form: HTMLFormElement): Promise<{ ok: boolean; text: string }> {
    try {
      const out = await post<{ cover?: string; generated?: boolean }>("/admin/suggest/cover", articleBody(form, latest.current.lang));
      if (!out.ok || !out.cover) return { ok: false, text: out.error ?? "The AI couldn't make a cover." };
      latest.current.setCover(out.cover);
      return {
        ok: true,
        text: out.generated
          ? "Generated a cover and uploaded it with your images. Change anything you like: remove the cover, upload your own, or generate another."
          : "Picked a cover from the article's images. Change anything you like.",
      };
    } catch {
      return { ok: false, text: "Couldn't reach the server for a cover." };
    }
  }

  async function suggestCover() {
    const form = formRef.current;
    if (!form || busy) return;
    setBusy("cover");
    setStatus({ kind: "info", text: "Making a cover image… this can take up to a minute.", field: "cover" });
    const result = await makeCover(form);
    setStatus({ kind: result.ok ? "info" : "error", text: result.text, field: "cover" });
    setBusy("");
  }

  const FIELD_NAMES: Record<"title" | "summary" | "tags" | "slug", string> = { title: "title", summary: "summary", tags: "tags", slug: "slug" };

  // One field: writes the title, summary or slug (replacing it), or adds tags.
  async function suggestField(field: "title" | "summary" | "tags" | "slug") {
    const form = formRef.current;
    if (!form || busy) return;
    setBusy(field);
    setStatus({ kind: "info", text: "Writing…", field });
    try {
      const body = articleBody(form, latest.current.lang);
      body.set("field", field);
      const out = await post<{ value?: string | string[] }>("/admin/suggest/field", body);
      if (!out.ok || !out.value) {
        setStatus({ kind: "error", text: out.error ?? `The AI couldn't write the ${FIELD_NAMES[field]}.`, field });
        return;
      }
      const { taxonomy, setTaxonomy, setSlug, fillSlug } = latest.current;
      if (field === "tags" && Array.isArray(out.value)) {
        const tags = [...new Set([...taxonomy.tags, ...out.value])].slice(0, MAX_TAG_COUNT);
        const added = tags.length - taxonomy.tags.length;
        setTaxonomy({ ...taxonomy, tags });
        setStatus({ kind: "info", text: added ? `Added ${added} tag${added === 1 ? "" : "s"}.` : "No new tags to add.", field });
        return;
      }
      const value = String(out.value);
      if (field === "slug") setSlug(value);
      else {
        setField(form, field, value);
        if (field === "title") fillSlug(value);
      }
      setOffers((o) => ({ ...o, [field]: undefined }));
      setStatus({ kind: "info", text: "Done. Edit it as you like.", field });
    } catch {
      setStatus({ kind: "error", text: "Couldn't reach the server.", field });
    } finally {
      setBusy("");
    }
  }

  // A whole article from the chat box: writes (or, with the form already
  // filled, revises) title, slug, summary, tags, category and text, then
  // makes a cover if there's none. Resolves to the AI's note for the chat.
  async function generateArticle(prompt: string, history: string[]): Promise<{ ok: boolean; text: string }> {
    const form = formRef.current;
    if (!form || busy) return { ok: false, text: "Wait for the current AI task to finish." };
    setBusy("article");
    setStatus(null);
    try {
      const { taxonomy } = latest.current;
      const body = articleBody(form, latest.current.lang);
      body.set("prompt", prompt);
      body.set("slug", latest.current.slug); // never suggested as its own related article
      for (const h of history) body.append("history", h);
      body.set("tags", taxonomy.tags.join(", "));
      body.set("category", taxonomy.category === NEW_CATEGORY ? (taxonomy.newCategory?.[latest.current.lang] ?? "") : taxonomy.category);
      const out = await post<{
        reply?: string;
        title?: string;
        slug?: string;
        summary?: string;
        tags?: string[];
        category?: string;
        newCategory?: Record<string, string>;
        body?: string;
      }>("/admin/generate", body);
      if (!out.ok || !out.title || !out.body) return { ok: false, text: out.error ?? "The AI couldn't write the article." };

      setField(form, "title", out.title);
      setField(form, "summary", out.summary ?? "");
      const slugKept = !!out.slug && out.slug !== latest.current.slug && !latest.current.setSlug(out.slug);
      latest.current.setBody(out.body);
      // Tags and category: the AI's, when it gave any; otherwise what the
      // author had stays (an empty answer must not wipe their choice).
      const cur = latest.current.taxonomy;
      const tags = (out.tags ?? []).slice(0, MAX_TAG_COUNT);
      latest.current.setTaxonomy({
        ...cur,
        tags: tags.length ? tags : cur.tags,
        ...(out.category
          ? { category: out.category, newCategory: null }
          : out.newCategory
            ? { category: NEW_CATEGORY, newCategory: out.newCategory }
            : {}),
      });
      setOffers({});
      let note = out.reply ?? "Done.";
      if (slugKept) note += ` The address stays “${latest.current.slug}”: an article's slug can't change once it's saved.`;

      if (!latest.current.cover && latest.current.canGenerateImages) {
        setBusy("cover");
        setStatus({ kind: "info", text: "Generating a cover image… this can take up to a minute.", field: "cover" });
        const cover = await makeCover(form);
        setStatus({ kind: cover.ok ? "info" : "error", text: cover.text, field: "cover" });
        note += cover.ok ? " I also made a cover image." : ` (No cover: ${cover.text})`;
      }
      return { ok: true, text: note };
    } catch {
      return { ok: false, text: "Couldn't reach the server." };
    } finally {
      setBusy("");
    }
  }

  // One field (title, summary or text), translated from the article's
  // version in another language.
  async function translateField(field: "title" | "summary" | "body", from: Locale) {
    const form = formRef.current;
    if (!form || busy) return;
    const { slug, lang } = latest.current;
    const fromName = getDictionary(from).languageName;
    const what = field === "body" ? "text" : field;
    // Replacing the text loses the most work: ask first.
    if (field === "body" && fieldValue(form, "body") && !window.confirm(`Replace this version's text with a translation of the ${fromName} text?`)) return;
    setBusy(field);
    setStatus({
      kind: "info",
      text: field === "body" ? `Translating the ${fromName} text… a long article can take a minute or two.` : `Translating the ${fromName} ${what}…`,
      field,
    });
    try {
      const out = await post<{ title?: string; summary?: string; body?: string }>("/admin/translate", new URLSearchParams({ slug, from, to: lang, only: field }));
      const value = out[field];
      if (!out.ok || !value) {
        setStatus({ kind: "error", text: out.error ?? `The AI couldn't translate the ${what}.`, field });
        return;
      }
      if (field === "body") latest.current.setBody(value);
      else {
        setField(form, field, value);
        if (field === "title") latest.current.fillSlug(value);
        setOffers((o) => ({ ...o, [field]: undefined }));
      }
      setStatus({ kind: "info", text: `Translated from ${fromName}. Edit it as you like.`, field });
    } catch {
      setStatus({ kind: "error", text: "Couldn't reach the server for the translation.", field });
    } finally {
      setBusy("");
    }
  }

  // Fills title, summary and body with a translation of the article's
  // version in another language.
  async function translate(from: Locale) {
    const form = formRef.current;
    if (!form || busy) return;
    const { slug, lang } = latest.current;
    const fromName = getDictionary(from).languageName;
    const hasContent = ["title", "summary", "body"].some((f) => fieldValue(form, f));
    if (hasContent && !window.confirm(`Replace this version's title, summary and text with a translation of the ${fromName} version?`)) return;
    setBusy("translate");
    setStatus({ kind: "info", text: `Translating the ${fromName} version… a long article can take a minute or two.` });
    try {
      const out = await post<{ title?: string; summary?: string; body?: string }>("/admin/translate", new URLSearchParams({ slug, from, to: lang }));
      if (!out.ok) {
        setStatus({ kind: "error", text: out.error ?? "The AI couldn't translate the article." });
        return;
      }
      setField(form, "title", out.title ?? "");
      setField(form, "summary", out.summary ?? "");
      latest.current.setBody(out.body ?? "");
      setOffers({});
      setStatus({ kind: "info", text: `Translated from the ${fromName} version. Read it through and adjust anything before publishing.` });
    } catch {
      setStatus({ kind: "error", text: "Couldn't reach the server for the translation." });
    } finally {
      setBusy("");
    }
  }

  return {
    busy,
    translate: (from: Locale) => void translate(from),
    translateField: (field: "title" | "summary" | "body", from: Locale) => void translateField(field, from),
    generateArticle,
    suggestField: (field: "title" | "summary" | "tags" | "slug") => void suggestField(field),
    status,
    offers,
    suggest: () => void suggest(),
    suggestCover: () => void suggestCover(),
    acceptOffer(field: keyof Offers) {
      const value = offers[field];
      if (value && formRef.current) {
        setField(formRef.current, field, value);
        if (field === "title") latest.current.fillSlug(value);
      }
      setOffers((o) => ({ ...o, [field]: undefined }));
    },
    dismissOffer: (field: keyof Offers) => setOffers((o) => ({ ...o, [field]: undefined })),
  };
}

// The panel above the fields: the buttons and what the AI did.
export function AIPanel(props: {
  busy: string;
  status: Status;
  onSuggest: () => void;
  translateFrom: Locale[]; // other languages this article exists in
  onTranslate: (from: Locale) => void;
}) {
  const { busy, status, onSuggest, translateFrom, onTranslate } = props;
  return (
    <div className="ai-suggest">
      <button type="button" className="btn btn-quiet btn-sm" onClick={onSuggest} disabled={!!busy}>
        {busy === "text" ? "Suggesting…" : "✨ Suggest with AI"}
      </button>
      {translateFrom.map((l) => (
        <button key={l} type="button" className="btn btn-quiet btn-sm" onClick={() => onTranslate(l)} disabled={!!busy}>
          {busy === "translate" ? "Translating…" : `✨ Translate from ${getDictionary(l).languageName}`}
        </button>
      ))}
      <span className={status?.kind === "error" && !status.field ? "field-error" : "field-hint"} role="status">
        {(!status?.field && status?.text) || "Suggests a title, slug, summary, tags, category and cover (generated when the article has none) from what you've written. Nothing is filled in until you click."}
      </span>
    </div>
  );
}

// A suggestion for a field the author already filled in.
export function Offer({ text, onUse, onDismiss }: { text?: string; onUse: () => void; onDismiss: () => void }) {
  if (!text) return null;
  return (
    <div className="ai-offer">
      <span className="ai-offer-label">✨ Suggested:</span> <span className="ai-offer-text">{text}</span>
      <span className="ai-offer-actions">
        <button type="button" className="btn btn-quiet btn-sm" onClick={onUse}>
          Use this
        </button>
        <button type="button" className="btn btn-quiet btn-sm" onClick={onDismiss} aria-label="Dismiss suggestion">
          ×
        </button>
      </span>
    </div>
  );
}

// The ✨ button beside one field.
export function AIFieldButton(props: { field: AIField; busy: string; label?: string; title: string; onClick: () => void }) {
  const { field, busy, label = "✨ AI", title, onClick } = props;
  return (
    <button type="button" className="ai-field-button" onClick={onClick} disabled={!!busy} title={title}>
      {busy === field ? "Working…" : label}
    </button>
  );
}

// That field's last AI message, shown under it.
export function AIFieldStatus({ field, status }: { field: AIField; status: Status }) {
  if (status?.field !== field) return null;
  return (
    <span className={`ai-field-status ${status.kind === "error" ? "field-error" : "field-hint"}`} role="status">
      ✨ {status.text}
    </span>
  );
}
