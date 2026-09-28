import type { Dictionary } from "../lib/i18n";
import type { NavCategory, Post } from "../types";

export const categoryHref = (lang: string, slug: string) => `/${lang}/categories/${encodeURIComponent(slug)}`;
export const tagHref = (lang: string, tag: string) => `/${lang}/tags/${encodeURIComponent(tag)}`;

export function categoryName(categories: NavCategory[] | undefined, slug: string): string {
  return categories?.find((c) => c.slug === slug)?.name ?? "";
}

export function TagLinks({ lang, tags, className = "tag-list" }: { lang: string; tags: string[] | null; className?: string }) {
  if (!tags?.length) return null;
  return (
    <ul className={className}>
      {tags.map((tag) => (
        <li key={tag}>
          <a className="tag" href={tagHref(lang, tag)} rel="tag">
            #{tag}
          </a>
        </li>
      ))}
    </ul>
  );
}

// The category and tags of an article, for lists: "Security · #go #seo".
export function PostMeta({ post, lang, categories }: { post: Post; lang: string; categories?: NavCategory[] }) {
  const name = post.category ? categoryName(categories, post.category) : "";
  if (!name && !post.tags?.length) return null;
  return (
    <div className="post-meta">
      {name && (
        <a className="category-link" href={categoryHref(lang, post.category)}>
          {name}
        </a>
      )}
      <TagLinks lang={lang} tags={post.tags} className="tag-list tag-list-inline" />
    </div>
  );
}

// The "Categories" section of the side menu.
export function CategoryMenu({ lang, t, categories }: { lang: string; t: Dictionary; categories?: NavCategory[] }) {
  if (!categories?.length) return null;
  return (
    <nav className="side-nav side-categories" aria-label={t.categories}>
      <h2 className="side-heading">{t.categories}</h2>
      <ul>
        {categories.map((c) => (
          <li key={c.slug}>
            <a href={categoryHref(lang, c.slug)}>
              {c.name} <span className="side-count">{c.count}</span>
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
