import type { Locale } from "./lib/i18n";

// Mirrors posts.Post in the Go backend.
export type Post = {
  slug: string;
  lang: Locale;
  title: string;
  summary: string;
  body: string;
  publishedAt: string;
  status: PostStatus;
  category: string; // category slug, "" for none (shared by all translations)
  tags: string[] | null;
  // Unpublished edits to a published article (admin pages only).
  draft?: { title: string; summary: string; body: string; publishedAt: string; savedAt: string } | null;
};

export type PostStatus = "draft" | "published" | "disabled";

export type BulkVerb = "publish" | "disable" | "draft" | "delete";

// Mirrors bulkResult in backend/internal/web/bulk.go.
export type BulkResult = { verb: BulkVerb; articles: number; translations: number; skipped: number };

export type Theme = "auto" | "light" | "dark";

// What the Go server passes to render() and embeds as window.__PAGE__ for
// hydration. Keep in sync with the page handlers in backend/internal/web.
// Categories with published articles in the page's language (public pages).
export type NavCategory = { slug: string; name: string; count: number };
export type TagCount = { tag: string; count: number };
export type CategoryOption = { slug: string; name: string };

type PageBase = { lang: Locale; theme: Theme; year: number; categories?: NavCategory[] };

// Pages of the public site, hydrated by app.js.
export type PublicPageData = PageBase & (
  | { page: "home"; posts: Post[]; groups: { category: NavCategory; posts: Post[] }[] }
  | {
      page: "post";
      post: Post;
      availableLanguages: Locale[];
      preview: boolean;
      pendingChanges: boolean;
      url: string; // the article's absolute address, for sharing
    }
  | {
      page: "search";
      scope: "search" | "category" | "tag";
      query: string;
      tag: string;
      category: string;
      results: Post[];
      tags: TagCount[];
      filtered: boolean;
    }
  | { page: "notFound" }
  | { page: "about"; profile: AboutProfile }
);

// Admin pages, hydrated by admin.js (which includes the editor).
export type AdminPageData = PageBase & (
  | { page: "adminLogin"; error: string }
  | {
      page: "adminCategories";
      list: { slug: string; names: Record<string, string>; articles: number }[];
      languages: Locale[];
      form: CategoryForm;
      errors: Record<string, string>;
      rowErrors: Record<string, string>;
      rowForms: Record<string, CategoryForm>;
      notice: "" | "created" | "renamed" | "deleted";
    }
  | { page: "adminSetup"; error: string }
  | { page: "adminPassword"; errors: Partial<Record<"current" | "password", string>>; notice: "" | "saved" }
  | {
      page: "adminList";
      posts: Post[];
      languages: Locale[];
      notice: "" | "deleted" | "deleted-all" | "none-selected";
      filter: "" | PostStatus;
      bulk: BulkResult | null;
      categories: CategoryOption[];
      categoryFilter: string; // "", a category slug, or "none"
    }
  | {
      page: "adminEdit";
      mode: "new" | "edit";
      status: "" | PostStatus;
      notice: "" | "draft" | "published" | "saved" | "disabled" | "revised" | "discarded";
      draftSavedAt: string; // set when a published article has unpublished changes
      translations: Partial<Record<Locale, PostStatus | "edited">>; // existing translations of this article
      categories: CategoryOption[];
      knownTags: string[];
      otherTranslations: Locale[];
      form: AdminForm;
      errors: Partial<Record<keyof AdminForm, string>>;
      languages: Locale[];
    }
  | {
      page: "adminProfile";
      form: ProfileForm;
      errors: Record<string, string>;
      languages: Locale[];
      notice: "" | "saved";
    }
);

export type PageData = PublicPageData | AdminPageData;

export const isAdminPage = (p: PageData): p is AdminPageData => p.page.startsWith("admin");

export type ProfileLink = { label: string; url: string };

// Mirrors aboutView in backend/internal/web/about.go.
export type AboutProfile = {
  name: string;
  photoUrl: string;
  location: string;
  country: string; // ISO 3166-1 alpha-2, e.g. "ID"; "" for none
  email: string;
  links: ProfileLink[];
  headline: string;
  bio: string;
  textLang: Locale; // language of headline/bio; may differ from the page when it falls back
  empty: boolean;
};

// Mirrors profileForm in backend/internal/web/about.go.
export type ProfileForm = {
  name: string;
  photoUrl: string;
  location: string;
  country: string; // ISO 3166-1 alpha-2, e.g. "ID"; "" for none
  email: string;
  links: string; // one "Label | URL" per line
  texts: Partial<Record<Locale, { headline: string; bio: string }>>;
};

// Mirrors adminForm in backend/internal/web/admin.go.
export type AdminForm = {
  slug: string;
  lang: string;
  title: string;
  summary: string;
  body: string;
  date: string; // YYYY-MM-DD
  category: string;
  tags: string; // comma-separated
};

export type CategoryForm = { slug: string; names: Record<string, string> | null };
