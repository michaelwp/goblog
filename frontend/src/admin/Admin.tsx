// Admin screens for managing articles. Every form posts to the Go server
// and works without JavaScript; hydration only adds the delete confirmation.
import { type FormEvent, type MouseEvent, type ReactNode, useRef, useState } from "react";

import { countriesFor, flagEmoji } from "../lib/countries";
import { formatDate } from "../lib/format";
import { queryString } from "../lib/query";
import { useHydrated } from "../lib/useHydrated";
import { getDictionary, type Locale } from "../lib/i18n";
import type { AdminForm, BulkResult, BulkVerb, PageData, Post, PostStatus } from "../types";
import { PasswordFields } from "./PasswordField";
import { PhotoField } from "./PhotoField";
import { RichEditor } from "./RichEditor";
import { TagInput } from "./TagInput";
import { type AutosaveState, useAutosave } from "./useAutosave";

type Page<K extends PageData["page"]> = Extract<PageData, { page: K }>;

type Section = "articles" | "categories" | "profile" | "password";

function AdminLayout(props: { children: ReactNode; signedIn?: boolean; section?: Section }) {
  const { children, signedIn = true, section } = props;
  const site = getDictionary("en").siteTitle;
  return (
    <div className="admin">
      <header className="admin-header">
        <a className="admin-brand" href={signedIn ? "/admin" : "/"}>
          {site} <span>Admin</span>
        </a>
        {signedIn && (
          <nav className="admin-nav">
            <a href="/admin" aria-current={section === "articles" ? "page" : undefined}>
              Articles
            </a>
            <a href="/admin/categories" aria-current={section === "categories" ? "page" : undefined}>
              Categories
            </a>
            <a href="/admin/profile" aria-current={section === "profile" ? "page" : undefined}>
              Profile
            </a>
            <a href="/admin/password" aria-current={section === "password" ? "page" : undefined}>
              Password
            </a>
            <a href="/" target="_blank" rel="noreferrer">
              View site ↗
            </a>
            <form method="post" action="/admin/logout">
              <button type="submit" className="btn btn-quiet">
                Log out
              </button>
            </form>
          </nav>
        )}
      </header>
      <main className="admin-main">{children}</main>
    </div>
  );
}

// ---- Login -------------------------------------------------------------

export function AdminLogin({ error }: Page<"adminLogin">) {
  return (
    <AdminLayout signedIn={false}>
      <form className="admin-card admin-login" method="post" action="/admin/login">
        <h1>Sign in</h1>
        <p className="admin-muted">Enter the admin password to manage articles.</p>
        {error && (
          <p className="admin-alert admin-alert-error" role="alert">
            {error}
          </p>
        )}
        <label className="field">
          <span className="field-label">Password</span>
          <input type="password" name="password" autoComplete="current-password" required autoFocus />
        </label>
        <button type="submit" className="btn btn-primary btn-block">
          Sign in
        </button>
      </form>
    </AdminLayout>
  );
}

// ---- First-time setup ------------------------------------------------

export function AdminSetup({ error }: Page<"adminSetup">) {
  return (
    <AdminLayout signedIn={false}>
      <form className="admin-card admin-login" method="post" action="/admin/setup">
        <h1>Set up admin</h1>
        <p className="admin-muted">
          Create the password for managing this blog. For security, you also need the one-time setup code printed in the server log
          (run <code>make admin-code</code> to see it).
        </p>
        {error && (
          <p className="admin-alert admin-alert-error" role="alert">
            {error}
          </p>
        )}
        <label className="field">
          <span className="field-label">Setup code</span>
          <input name="code" inputMode="numeric" autoComplete="one-time-code" placeholder="1234-5678" required autoFocus />
        </label>
        <PasswordFields />
        <button type="submit" className="btn btn-primary btn-block">
          Create password and sign in
        </button>
      </form>
    </AdminLayout>
  );
}

// ---- Change password -------------------------------------------------

export function AdminPassword({ errors, notice }: Page<"adminPassword">) {
  return (
    <AdminLayout section="password">
      <div className="admin-toolbar">
        <div>
          <h1>Password</h1>
          <p className="admin-muted">Changing it signs you out everywhere else.</p>
        </div>
      </div>
      {notice === "saved" && (
        <p className="admin-alert admin-alert-success" role="status">
          Password changed. Other sessions have been signed out.
        </p>
      )}
      <form className="admin-card admin-form admin-narrow" method="post" action="/admin/password">
        <Field label="Current password" name="current" error={errors.current}>
          <input type="password" name="current" autoComplete="current-password" required />
        </Field>
        <PasswordFields error={errors.password} />
        <div className="admin-actions">
          <button type="submit" className="btn btn-primary">
            Change password
          </button>
        </div>
      </form>
    </AdminLayout>
  );
}

// ---- Status helpers ----------------------------------------------------

const STATUS_LABEL: Record<PostStatus, string> = { draft: "Draft", published: "Published", disabled: "Disabled" };

function StatusBadge({ status }: { status: PostStatus }) {
  return <span className={`status-badge status-${status}`}>{STATUS_LABEL[status]}</span>;
}

// ---- Article list ------------------------------------------------------

const LIST_NOTICES = {
  deleted: "Translation deleted.",
  "deleted-all": "Article deleted in every language.",
  "none-selected": "Select at least one article first.",
};

const FILTERS: { value: "" | PostStatus; label: string }[] = [
  { value: "", label: "All" },
  { value: "published", label: "Published" },
  { value: "draft", label: "Drafts" },
  { value: "disabled", label: "Disabled" },
];

const VERB_LABEL: Record<BulkVerb, string> = { publish: "Publish", disable: "Disable", draft: "Move to drafts", delete: "Delete" };

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

function bulkMessage(r: BulkResult): string {
  const what = `${plural(r.articles, "article")} (${plural(r.translations, "translation")})`;
  const done = { publish: "Published", disable: "Disabled", draft: "Moved to drafts", delete: "Deleted" }[r.verb];
  const skipped = r.skipped ? ` ${plural(r.skipped, "empty draft")} skipped: add a body before publishing.` : "";
  return r.articles === 0 && !r.skipped ? "Nothing changed." : `${done} ${what}.${skipped}`;
}

export function AdminList({ posts, languages, notice, filter, bulk, categories, categoryFilter }: Page<"adminList">) {
  // Group translations under one row per article, newest first.
  const groups = new Map<string, Post[]>();
  for (const p of posts) groups.set(p.slug, [...(groups.get(p.slug) ?? []), p]);
  const count = (status: "" | PostStatus) => (status ? posts.filter((p) => p.status === status).length : posts.length);
  // Category is shared by an article's translations, so any one says it.
  const known = new Set(categories.map((c) => c.slug));
  const categoryOf = (ts: Post[]) => (ts[0].category && known.has(ts[0].category) ? ts[0].category : "none");
  const shown = [...groups].filter(
    ([, ts]) => (!filter || ts.some((p) => p.status === filter)) && (!categoryFilter || categoryOf(ts) === categoryFilter),
  );
  // The list is grouped by category (alphabetical), then "No category".
  const sections = [...categories, { slug: "none", name: "No category" }]
    .map((c) => ({ slug: c.slug, name: c.name, items: shown.filter(([, ts]) => categoryOf(ts) === c.slug) }))
    .filter((sec) => sec.items.length > 0);
  const listHref = (show: string, category: string) => {
    const q = queryString({ show, category });
    return q ? `/admin?${q}` : "/admin";
  };

  const [selected, setSelected] = useState<Set<string>>(new Set());
  const shownSlugs = shown.map(([slug]) => slug);
  const allChecked = shownSlugs.length > 0 && shownSlugs.every((s) => selected.has(s));
  const toggle = (slug: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (!next.delete(slug)) next.add(slug);
      return next;
    });
  const selectedTranslations = shown.filter(([slug]) => selected.has(slug)).reduce((n, [, ts]) => n + ts.length, 0);

  // One article row, covering all its translations.
  const renderRow = ([slug, translations]: [string, Post[]]) => {
    const main = translations.find((p) => p.lang === languages[0]) ?? translations[0];
    const missing = languages.filter((l) => !translations.some((p) => p.lang === l));
    const live = translations.find((p) => p.status === "published");
    const statuses = new Set(translations.map((p) => p.status));
    const checked = selected.has(slug);
    return (
      <li key={slug} className={`admin-row${checked ? " is-selected" : ""}`}>
        <input
          type="checkbox"
          name="slug"
          value={slug}
          className="row-check"
          checked={checked}
          onChange={() => toggle(slug)}
          aria-label={`Select “${main.title}”`}
        />
        <div className="admin-row-main">
          <a className="admin-row-title" href={`/admin/posts/${slug}/${main.lang}`}>
            {main.title}
          </a>
          <p className="admin-row-meta">
            <code>{slug}</code> · {formatDate(main.publishedAt, main.lang)}
            {!!main.tags?.length && <span className="admin-row-tags"> · {main.tags.map((tag) => `#${tag}`).join(" ")}</span>}
          </p>
        </div>
        <div className="admin-langs">
          {translations.map((p) => (
            <a
              key={p.lang}
              className={`lang-chip chip-${p.status}`}
              href={`/admin/posts/${slug}/${p.lang}`}
              title={`Edit ${languageName(p.lang)} (${STATUS_LABEL[p.status].toLowerCase()}${p.status === "published" && p.draft ? ", with unpublished changes" : ""})`}
            >
              {p.lang.toUpperCase()}
              {p.status !== "published" && <span className="chip-status">{STATUS_LABEL[p.status]}</span>}
              {p.status === "published" && p.draft && <span className="chip-status chip-edited-label">Edited</span>}
            </a>
          ))}
          {missing.map((l) => (
            <a
              key={l}
              className="lang-chip lang-chip-missing"
              href={`/admin/new?slug=${slug}&lang=${l}&from=${main.lang}`}
              title={`Add ${languageName(l)} translation`}
            >
              + {l.toUpperCase()}
            </a>
          ))}
          {live ? (
            <a className="admin-view" href={`/${live.lang}/posts/${slug}`} target="_blank" rel="noreferrer" title="View on site">
              ↗
            </a>
          ) : (
            <span className="admin-view admin-view-off" title="Not published in any language">
              ↗
            </span>
          )}
          <details className="row-menu">
            <summary aria-label={`Actions for “${main.title}”`} title="Actions">
              ⋯
            </summary>
            <div className="row-menu-panel">
              {!(statuses.size === 1 && statuses.has("published")) && (
                <button type="submit" name="action" value={`publish:${slug}`}>
                  Publish
                </button>
              )}
              {statuses.has("published") && (
                <button type="submit" name="action" value={`disable:${slug}`}>
                  Disable
                </button>
              )}
              {!(statuses.size === 1 && statuses.has("draft")) && (
                <button type="submit" name="action" value={`draft:${slug}`}>
                  Move to drafts
                </button>
              )}
              <button type="submit" name="action" value={`delete:${slug}`} className="row-menu-danger">
                Delete
              </button>
            </div>
          </details>
        </div>
      </li>
    );
  };

  // Confirm destructive actions; the clicked button says which action and
  // (for a row menu) which article.
  function confirmAction(e: FormEvent<HTMLFormElement>) {
    const submitter = (e.nativeEvent as SubmitEvent).submitter as HTMLButtonElement | null;
    const [verb, only] = (submitter?.value ?? "").split(":") as [BulkVerb, string | undefined];
    if (!only && selected.size === 0) {
      e.preventDefault();
      return;
    }
    const target = only
      ? `“${groups.get(only)?.[0]?.title ?? only}” (${plural(groups.get(only)?.length ?? 0, "translation")})`
      : `${plural(selected.size, "article")} (${plural(selectedTranslations, "translation")})`;
    const question =
      verb === "delete"
        ? `Delete ${target}? This can't be undone.`
        : verb === "disable"
          ? `Take ${target} off the site? You can publish again later.`
          : null;
    if (question && !window.confirm(question)) e.preventDefault();
  }

  return (
    <AdminLayout section="articles">
      <div className="admin-toolbar">
        <div>
          <h1>Articles</h1>
          <p className="admin-muted">
            {plural(groups.size, "article")}, {plural(posts.length, "translation")}
          </p>
        </div>
        <a className="btn btn-primary" href="/admin/new">
          New article
        </a>
      </div>

      {bulk && (
        <p className={`admin-alert ${bulk.skipped && !bulk.articles ? "admin-alert-error" : "admin-alert-success"}`} role="status">
          {bulkMessage(bulk)}
        </p>
      )}
      {notice && (
        <p className={`admin-alert ${notice === "none-selected" ? "admin-alert-error" : "admin-alert-success"}`} role="status">
          {LIST_NOTICES[notice]}
        </p>
      )}

      <div className="admin-filter-row">
        <nav className="admin-filters" aria-label="Filter by status">
          {FILTERS.map((f) => (
            <a key={f.value} href={listHref(f.value, categoryFilter)} aria-current={filter === f.value ? "page" : undefined}>
              {f.label} <span>{count(f.value)}</span>
            </a>
          ))}
        </nav>
        <form className="admin-category-filter" method="get" action="/admin">
          {filter && <input type="hidden" name="show" value={filter} />}
          <select
            name="category"
            defaultValue={categoryFilter}
            aria-label="Filter by category"
            onChange={(e) => (window.location.href = listHref(filter, e.target.value))}
          >
            <option value="">All categories</option>
            {categories.map((c) => (
              <option key={c.slug} value={c.slug}>
                {c.name}
              </option>
            ))}
            <option value="none">No category</option>
          </select>
          <noscript>
            <button type="submit" className="btn btn-sm btn-secondary">
              Filter
            </button>
          </noscript>
        </form>
      </div>

      {groups.size === 0 ? (
        <div className="admin-card admin-empty">
          <p>No articles yet.</p>
          <a className="btn btn-primary" href="/admin/new">
            Write the first one
          </a>
        </div>
      ) : shown.length === 0 ? (
        <p className="admin-muted admin-none">No articles match these filters.</p>
      ) : (
        <form method="post" action="/admin/bulk" onSubmit={confirmAction}>
          <input type="hidden" name="show" value={filter} />
          {categoryFilter && <input type="hidden" name="category" value={categoryFilter} />}
          <div className={`bulk-bar${selected.size ? " has-selection" : ""}`}>
            <label className="bulk-check">
              <input
                type="checkbox"
                checked={allChecked}
                ref={(el) => {
                  if (el) el.indeterminate = selected.size > 0 && !allChecked;
                }}
                onChange={() => setSelected(allChecked ? new Set() : new Set(shownSlugs))}
                aria-label="Select all articles"
              />
              <span>{selected.size ? `${selected.size} selected` : "Select all"}</span>
            </label>
            <div className="bulk-actions" aria-label="Actions for selected articles">
              {(["publish", "disable", "draft", "delete"] as const).map((verb) => (
                <button
                  key={verb}
                  type="submit"
                  name="action"
                  value={verb}
                  className={`btn btn-sm ${verb === "delete" ? "btn-danger" : "btn-secondary"}`}
                  aria-disabled={selected.size === 0 || undefined}
                >
                  {VERB_LABEL[verb]}
                </button>
              ))}
              {selected.size > 0 && (
                <button type="button" className="btn btn-quiet btn-sm" onClick={() => setSelected(new Set())}>
                  Clear
                </button>
              )}
            </div>
          </div>

          {sections.map((sec) => (
            <section key={sec.slug} className="admin-group">
              <h2 className="admin-group-title">
                {sec.name} <span>{sec.items.length}</span>
              </h2>
              <ul className="admin-list">{sec.items.map(renderRow)}</ul>
            </section>
          ))}
        </form>
      )}
    </AdminLayout>
  );
}

// ---- Create / edit -----------------------------------------------------

// Formats an ISO time in the viewer's locale, after hydration only: the
// server renders without Intl, and the times must match for hydration.
function useLocalTime(iso: string): string {
  const hydrated = useHydrated();
  return hydrated && iso ? new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "";
}

function AutosaveStatus({ state }: { state: AutosaveState }) {
  const time = useLocalTime(state.kind === "saved" ? state.at : "");
  const text =
    state.kind === "saving"
      ? "Saving…"
      : state.kind === "saved"
        ? `Draft saved${time ? ` at ${time}` : ""}`
        : state.kind === "dirty"
          ? "Unsaved changes · autosaves every minute"
          : state.kind === "paused"
            ? state.message
            : "Autosaves every minute";
  return (
    <span className={`autosave autosave-${state.kind}`} role="status" aria-live="polite">
      {text}
    </span>
  );
}

const TRANSLATION_STATE: Record<PostStatus | "edited", string> = {
  published: "Published",
  edited: "Published, with unpublished changes",
  draft: "Draft",
  disabled: "Disabled",
};

// The Language field of an existing article: pick a language to edit that
// translation, or to start it if it doesn't exist yet. Changes are saved
// before leaving.
function LanguageSwitcher(props: {
  slug: string;
  current: string;
  languages: Locale[];
  translations: Page<"adminEdit">["translations"];
  saveNow: () => Promise<boolean>;
  allowLeave: () => void;
}) {
  const { slug, current, languages, translations, saveNow, allowLeave } = props;
  const [switching, setSwitching] = useState(false);

  async function go(target: Locale) {
    if (target === current) return;
    setSwitching(true);
    const saved = await saveNow();
    if (!saved && !window.confirm("Your latest changes couldn't be saved. Switch languages anyway and lose them?")) {
      setSwitching(false);
      return;
    }
    allowLeave();
    window.location.assign(
      translations[target] ? `/admin/posts/${slug}/${target}` : `/admin/new?slug=${slug}&lang=${target}&from=${current}`,
    );
  }

  return (
    <label className="field">
      <span className="field-label">Language</span>
      <select value={current} onChange={(e) => void go(e.target.value as Locale)} disabled={switching} aria-describedby="lang-switch-hint">
        {languages.map((l) => {
          const state = translations[l];
          return (
            <option key={l} value={l}>
              {languageName(l)} · {l === current ? "editing" : state ? TRANSLATION_STATE[state] : "Not translated yet (start it)"}
            </option>
          );
        })}
      </select>
      <span className="field-hint" id="lang-switch-hint">
        {switching ? "Saving and switching…" : "Choose a language to edit that translation."}
      </span>
    </label>
  );
}

export function AdminEdit(props: Page<"adminEdit">) {
  const { mode, status, notice, form, errors, languages, otherTranslations, draftSavedAt, translations, categories, knownTags } = props;
  // A new article becomes an existing draft when autosave first creates it.
  const [created, setCreated] = useState<{ slug: string; lang: string } | null>(null);
  const isNew = mode === "new" && !created;
  const slug = created?.slug ?? form.slug;
  const articleLang = created?.lang ?? form.lang;
  const base = `/admin/posts/${slug}/${articleLang}`;
  const action = isNew ? "/admin/new" : base;
  const hasErrors = Object.keys(errors).length > 0;
  const current: PostStatus = status || "draft";
  const liveUrl = `/${articleLang}/posts/${slug}`;
  const lang = languageName(articleLang);
  const draftTime = useLocalTime(draftSavedAt);

  const formRef = useRef<HTMLFormElement>(null);
  const { state: autosave, saveNow, allowLeave } = useAutosave(formRef, isNew ? "/admin/new/autosave" : `${base}/autosave`, (editUrl) => {
    const [, , , s, l] = editUrl.split("/"); // /admin/posts/<slug>/<lang>
    setCreated({ slug: s, lang: l });
    window.history.replaceState(null, "", editUrl);
  });
  const hasDraftChanges = current === "published" && (!!draftSavedAt || autosave.kind === "saved");

  const confirm = (message: string) => (e: FormEvent<HTMLFormElement> | MouseEvent<HTMLButtonElement>) => {
    if (!window.confirm(message)) e.preventDefault();
  };

  const notices: Record<Exclude<typeof notice, "">, ReactNode> = {
    draft: <>Saved as a draft. Only you can see it; use Preview to check how it looks.</>,
    published: (
      <>
        Published. It&apos;s live at{" "}
        <a href={liveUrl} target="_blank" rel="noreferrer">
          {liveUrl}
        </a>
        .
      </>
    ),
    saved: <>Changes saved.</>,
    disabled: <>Disabled. It&apos;s hidden from the site until you publish it again.</>,
    revised: <>Draft saved. Readers still see the published version until you click Publish changes.</>,
    discarded: <>Draft changes discarded. The editor shows the published version again.</>,
  };

  // The action buttons, shown at the top and the bottom of the form. Both sets
  // submit the same form; pressing Enter uses the first button in the page,
  // so the safe choice (saving, never publishing) comes first.
  const actions = (position: "top" | "bottom") => (
    <div className={`admin-actions admin-actions-${position}`}>
      {current === "published" ? (
        <>
          <button type="submit" name="action" value="revise" className="btn btn-secondary">
            Save draft
          </button>
          <button type="submit" name="action" value="publish" className="btn btn-primary">
            Publish changes
          </button>
          <button
            type="submit"
            name="action"
            value="disable"
            className="btn btn-quiet"
            onClick={confirm(`Take the ${lang} version off the site? You can publish it again later.`)}
          >
            Disable
          </button>
          <button type="submit" name="action" value="draft" className="btn btn-quiet">
            Move to drafts
          </button>
        </>
      ) : (
        <>
          <button type="submit" name="action" value={current === "disabled" ? "save" : "draft"} className="btn btn-secondary">
            {current === "disabled" ? "Save changes" : "Save draft"}
          </button>
          <button type="submit" name="action" value="publish" className="btn btn-primary">
            {current === "disabled" ? "Publish again" : "Publish"}
          </button>
        </>
      )}
      <AutosaveStatus state={autosave} />
      <a className="btn btn-quiet admin-cancel" href="/admin">
        Cancel
      </a>
    </div>
  );

  return (
    <AdminLayout section="articles">
      <p className="admin-back">
        <a href="/admin">← All articles</a>
      </p>
      <div className="admin-toolbar">
        <div className="admin-title-row">
          <h1>{isNew ? "New article" : "Edit article"}</h1>
          {!isNew && <StatusBadge status={current} />}
        </div>
        {!isNew && (
          <div className="admin-title-actions">
            <a className="btn btn-quiet" href={`${base}/preview`} target="_blank" rel="noreferrer">
              Preview ↗
            </a>
            {current === "published" && (
              <a className="btn btn-quiet" href={liveUrl} target="_blank" rel="noreferrer">
                View live ↗
              </a>
            )}
          </div>
        )}
      </div>

      {notice && !hasErrors && (
        <p className="admin-alert admin-alert-success" role="status">
          {notices[notice]}
        </p>
      )}
      {hasErrors && (
        <p className="admin-alert admin-alert-error" role="alert">
          Please fix the highlighted fields.
        </p>
      )}
      {current === "published" && (
        <div className="draft-banner" role="note">
          <p>
            {hasDraftChanges ? (
              <>
                <strong>Unpublished changes{draftTime ? ` (saved ${draftTime})` : ""}.</strong> Readers still see the published version until you
                click <em>Publish changes</em>.
              </>
            ) : (
              <>
                <strong>This article is live.</strong> Your edits are kept as a draft; readers see them only after you click <em>Publish changes</em>.
              </>
            )}
          </p>
          {hasDraftChanges && (
            <button
              type="submit"
              form="discard-draft"
              className="btn btn-quiet btn-sm"
              onClick={confirm("Discard the unpublished changes and go back to the published version?")}
            >
              Discard changes
            </button>
          )}
        </div>
      )}
      {!isNew && <form id="discard-draft" method="post" action={`${base}/discard`} hidden />}

      <form ref={formRef} className="admin-card admin-form" method="post" action={action} noValidate>
        {actions("top")}
        <div className="field-row">
          <Field label="Slug" name="slug" error={errors.slug} hint={isNew ? "Used in the URL, e.g. my-first-post. Translations share a slug." : "The slug can't be changed."}>
            <input name="slug" defaultValue={slug} readOnly={!isNew} required pattern="[a-z0-9]+(-[a-z0-9]+)*" autoComplete="off" />
          </Field>
          {isNew ? (
            <Field label="Language" name="lang" error={errors.lang}>
              <select name="lang" defaultValue={articleLang}>
                {languages.map((l) => (
                  <option key={l} value={l}>
                    {languageName(l)}
                  </option>
                ))}
              </select>
            </Field>
          ) : (
            <LanguageSwitcher
              slug={slug}
              current={articleLang}
              languages={languages}
              translations={{ ...translations, [articleLang]: translations[articleLang as Locale] ?? current }}
              saveNow={saveNow}
              allowLeave={allowLeave}
            />
          )}
          <Field label="Publish date" name="date" error={errors.date}>
            <input type="date" name="date" defaultValue={form.date} required />
          </Field>
        </div>

        <div className="field-row field-row-2">
          <Field
            label="Category"
            name="category"
            error={errors.category}
            hint={categories.length ? "Shared by all translations of this article." : "No categories yet: add them under Categories."}
          >
            <select name="category" defaultValue={form.category}>
              <option value="">No category</option>
              {categories.map((c) => (
                <option key={c.slug} value={c.slug}>
                  {c.name}
                </option>
              ))}
            </select>
          </Field>
          {/* Not a <label>: it would forward clicks to the chips' buttons. */}
          <div className={errors.tags ? "field has-error" : "field"}>
            <span className="field-label">Tags</span>
            <TagInput defaultValue={form.tags} known={knownTags} invalid={!!errors.tags} />
            <span className={errors.tags ? "field-error" : "field-hint"}>{errors.tags || "Up to 10, shared by all translations. Press Enter or comma after each."}</span>
          </div>
        </div>

        <Field label="Title" name="title" error={errors.title}>
          <input name="title" defaultValue={form.title} required maxLength={200} />
        </Field>

        <Field label="Summary" name="summary" error={errors.summary} hint="One or two sentences shown on the main page, in search results and to search engines.">
          <textarea name="summary" defaultValue={form.summary} rows={2} maxLength={300} />
        </Field>

        <EditorField label="Body" name="body" defaultValue={form.body} rows={20} error={errors.body} />

        {actions("bottom")}
      </form>

      {!isNew && (
        <section className="admin-danger">
          <form
            className="admin-danger-row"
            method="post"
            action={`${base}/delete`}
            onSubmit={confirm(`Delete the ${lang} version of “${form.title}”? This can't be undone.`)}
          >
            <div>
              <h2>Delete this translation</h2>
              <p className="admin-muted">
                Removes the {lang} version{otherTranslations.length > 0 ? "; other languages are kept." : "."}
              </p>
            </div>
            <button type="submit" className="btn btn-danger">
              Delete
            </button>
          </form>
          {otherTranslations.length > 0 && (
            <form
              className="admin-danger-row"
              method="post"
              action={`/admin/articles/${slug}/delete`}
              onSubmit={confirm(`Delete “${form.title}” in all ${otherTranslations.length + 1} languages? This can't be undone.`)}
            >
              <div>
                <h2>Delete the whole article</h2>
                <p className="admin-muted">
                  Removes every language: {[articleLang, ...otherTranslations].map((l) => languageName(l)).join(", ")}.
                </p>
              </div>
              <button type="submit" className="btn btn-danger">
                Delete all
              </button>
            </form>
          )}
        </section>
      )}
    </AdminLayout>
  );
}

function Field(props: { label: string; name: keyof AdminForm | string; error?: string; hint?: string; children: ReactNode }) {
  const { label, error, hint, children } = props;
  return (
    <label className={error ? "field has-error" : "field"}>
      <span className="field-label">{label}</span>
      {children}
      {error ? <span className="field-error">{error}</span> : hint && <span className="field-hint">{hint}</span>}
    </label>
  );
}

// Not a <label>: the editor's toolbar buttons would become the label's target.
function EditorField(props: { label: string; name: string; defaultValue: string; rows: number; error?: string }) {
  const { label, name, defaultValue, rows, error } = props;
  const hintId = `${name}-hint`;
  return (
    <div className={error ? "field has-error" : "field"}>
      <span className="field-label">{label}</span>
      <RichEditor name={name} defaultValue={defaultValue} rows={rows} invalid={!!error} describedBy={hintId} />
      {error ? (
        <span className="field-error" id={hintId}>
          {error}
        </span>
      ) : (
        <span className="field-hint" id={hintId}>
          Format with the toolbar or shortcuts (⌘B, ⌘I, ⌘K). Paste or drop images to upload them (PNG, JPEG, GIF or WebP, up to 5 MB).
        </span>
      )}
    </div>
  );
}

// ---- Categories --------------------------------------------------------

const CATEGORY_NOTICES = {
  created: "Category added.",
  renamed: "Category renamed.",
  deleted: "Category deleted. Its articles were kept and now have no category.",
};

export function AdminCategories({ list: categories, languages, form, errors, rowErrors, rowForms, notice }: Page<"adminCategories">) {
  const nameOf = (names: Record<string, string> | null, lang: string) => names?.[lang] ?? "";
  const confirmDelete = (name: string, articles: number) => (e: FormEvent<HTMLFormElement>) => {
    const what = articles ? ` Its ${plural(articles, "article")} will be kept, without a category.` : "";
    if (!window.confirm(`Delete the category “${name}”?${what}`)) e.preventDefault();
  };

  return (
    <AdminLayout section="categories">
      <div className="admin-toolbar">
        <div>
          <h1>Categories</h1>
          <p className="admin-muted">Group articles by topic. Each article has at most one category, shared by its translations.</p>
        </div>
      </div>

      {notice && (
        <p className="admin-alert admin-alert-success" role="status">
          {CATEGORY_NOTICES[notice]}
        </p>
      )}

      <form className="admin-card admin-form" method="post" action="/admin/categories" noValidate>
        <h2 className="admin-section-title">Add a category</h2>
        <div className="field-row field-row-2">
          {languages.map((l) => (
            <Field key={l} label={`Name (${languageName(l)})`} name={`name_${l}`} error={errors[`name_${l}`]}>
              <input name={`name_${l}`} defaultValue={nameOf(form.names, l)} maxLength={50} required />
            </Field>
          ))}
        </div>
        <Field
          label="Slug (optional)"
          name="slug"
          error={errors.slug}
          hint={`Used in the address, e.g. /${languages[0]}/categories/web-security. Leave empty to make one from the ${languageName(languages[0])} name. It can't be changed later.`}
        >
          <input name="slug" defaultValue={form.slug} maxLength={50} pattern="[a-z0-9]+(-[a-z0-9]+)*" autoComplete="off" />
        </Field>
        <div className="admin-actions">
          <button type="submit" className="btn btn-primary">
            Add category
          </button>
        </div>
      </form>

      {categories.length === 0 ? (
        <p className="admin-muted admin-none">No categories yet.</p>
      ) : (
        <ul className="category-list">
          {categories.map((c) => {
            const input = rowForms[c.slug]?.names ?? c.names;
            const name = nameOf(c.names, languages[0]) || c.slug;
            return (
              <li key={c.slug} className={`category-row${rowErrors[c.slug] ? " has-error" : ""}`}>
                <form className="category-row-form" method="post" action={`/admin/categories/${c.slug}`} noValidate>
                  <div className="category-row-names">
                    {languages.map((l) => (
                      <label key={l} className="field">
                        <span className="field-label">{languageName(l)}</span>
                        <input name={`name_${l}`} defaultValue={nameOf(input, l)} maxLength={50} required />
                      </label>
                    ))}
                  </div>
                  <div className="category-row-meta">
                    <code>{c.slug}</code>
                    <a href={`/admin?category=${c.slug}`}>{plural(c.articles, "article")}</a>
                  </div>
                  <div className="category-row-actions">
                    <button type="submit" className="btn btn-secondary btn-sm">
                      Save
                    </button>
                    <button type="submit" form={`delete-${c.slug}`} className="btn btn-quiet btn-sm row-menu-danger">
                      Delete
                    </button>
                  </div>
                </form>
                <form id={`delete-${c.slug}`} method="post" action={`/admin/categories/${c.slug}/delete`} onSubmit={confirmDelete(name, c.articles)} hidden />
                {rowErrors[c.slug] && <p className="field-error">{rowErrors[c.slug]}</p>}
              </li>
            );
          })}
        </ul>
      )}
    </AdminLayout>
  );
}

// ---- Profile -----------------------------------------------------------

export function AdminProfile({ form, errors, languages, notice }: Page<"adminProfile">) {
  const hasErrors = Object.keys(errors).length > 0;
  return (
    <AdminLayout section="profile">
      <div className="admin-toolbar">
        <div>
          <h1>Profile</h1>
          <p className="admin-muted">Shown on the About page. Leave any field empty to hide it.</p>
        </div>
        <a className="btn btn-quiet" href={`/${languages[0]}/about`} target="_blank" rel="noreferrer">
          View ↗
        </a>
      </div>

      {notice === "saved" && (
        <p className="admin-alert admin-alert-success" role="status">
          Profile saved.
        </p>
      )}
      {hasErrors && (
        <p className="admin-alert admin-alert-error" role="alert">
          Please fix the highlighted fields.
        </p>
      )}

      <form className="admin-form" method="post" action="/admin/profile" noValidate>
        <section className="admin-card admin-form">
          <h2 className="admin-section-title">About you</h2>
          <Field label="Name" name="name" error={errors.name}>
            <input name="name" defaultValue={form.name} maxLength={100} autoComplete="name" />
          </Field>
          <div className="field-row field-row-2">
            <Field label="Location" name="location" error={errors.location} hint="E.g. Jakarta, Indonesia.">
              <input name="location" defaultValue={form.location} maxLength={100} />
            </Field>
            <Field label="Country" name="country" error={errors.country} hint="Shown as a flag next to your location.">
              <select name="country" defaultValue={form.country}>
                <option value="">None</option>
                {countriesFor("en").map((c) => (
                  <option key={c.code} value={c.code}>
                    {flagEmoji(c.code)} {c.name}
                  </option>
                ))}
              </select>
            </Field>
          </div>
          <PhotoField defaultValue={form.photoUrl} error={errors.photoUrl} />
          <Field label="Email" name="email" error={errors.email} hint="Shown publicly as a contact link.">
            <input type="email" name="email" defaultValue={form.email} autoComplete="email" />
          </Field>
          <Field label="Links" name="links" error={errors.links} hint="One per line, as Label | https://… (up to 10).">
            <textarea name="links" defaultValue={form.links} rows={4} placeholder={"GitHub | https://github.com/you\nLinkedIn | https://linkedin.com/in/you"} />
          </Field>
        </section>

        {languages.map((l) => {
          const text = form.texts[l] ?? { headline: "", bio: "" };
          return (
            <section key={l} className="admin-card admin-form" lang={l}>
              <h2 className="admin-section-title">
                {languageName(l)} <span className="lang-chip">{l.toUpperCase()}</span>
              </h2>
              <Field label="Headline" name={`headline_${l}`} error={errors[`headline_${l}`]} hint="One sentence introducing yourself, shown as the lead.">
                <input name={`headline_${l}`} defaultValue={text.headline} maxLength={200} />
              </Field>
              <EditorField label="Bio" name={`bio_${l}`} defaultValue={text.bio} rows={12} error={errors[`bio_${l}`]} />
            </section>
          );
        })}

        <div className="admin-actions">
          <button type="submit" className="btn btn-primary">
            Save profile
          </button>
        </div>
      </form>
    </AdminLayout>
  );
}

function languageName(lang: string) {
  return getDictionary(lang as Locale)?.languageName ?? lang;
}
