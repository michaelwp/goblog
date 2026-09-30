import { Fragment, type ReactNode } from "react";

import { Contents } from "./components/Article";
import { renderBlock } from "./components/Markdown";
import { ShareBar } from "./components/Share";
import { Layout, MainMenu, SearchForm, SiteName, TitleBar, type Translation } from "./components/Layout";
import { categoryHref, categoryName, PostMeta, TagLinks, tagHref } from "./components/Taxonomy";
import { parseBody, readingMinutes, sections } from "./lib/article";
import { formatDate } from "./lib/format";
import { queryString } from "./lib/query";
import { type Dictionary, format, getDictionary, type Locale, locales } from "./lib/i18n";
import { AboutPage } from "./pages/About";
import type { NavCategory, PageData, Post, PublicPageData } from "./types";

export function App(props: PublicPageData) {
  const t = getDictionary(props.lang);
  switch (props.page) {
    case "home":
      return <HomePage {...props} t={t} />;
    case "post":
      return <ArticlePage {...props} t={t} />;
    case "search":
      return <SearchPage {...props} t={t} />;
    case "notFound":
      return <NotFoundPage {...props} t={t} />;
    case "about":
      return <AboutPage {...props} t={t} />;
  }
}

// Links to the same kind of page in every other site language.
function otherLocales(lang: Locale, href: (l: Locale) => string): Translation[] {
  return locales.filter((l) => l !== lang).map((l) => ({ lang: l, href: href(l) }));
}

type WithT<P> = P & { t: Dictionary };

// A dictionary string with {site} replaced by the styled site name.
function WithSiteName({ text, name }: { text: string; name: string }) {
  const [before, after = ""] = text.split("{site}");
  return (
    <>
      {before}
      <SiteName name={name} />
      {after}
    </>
  );
}

function HomePage({ lang, theme, year, posts, groups, categories, t }: WithT<Extract<PageData, { page: "home" }>>) {
  const [featured] = posts;
  const onlyOther = groups.length === 1 && groups[0].category.slug === "";
  return (
    <Layout
      lang={lang}
      theme={theme}
      year={year}
      t={t}
      translations={otherLocales(lang, (l) => `/${l}`)}
      sidebar={<MainMenu lang={lang} t={t} categories={categories} />}
    >
      <section className="mp-banner">
        <div className="mp-hero-row">
          <h1 className="mp-welcome">
            <WithSiteName text={t.welcome} name={t.siteTitle} />
          </h1>
        </div>
        <p className="mp-tagline">{t.welcomeTagline}</p>
        <p className="mp-count">{format(posts.length === 1 ? t.articleCountOne : t.articleCount, { n: posts.length })}</p>
      </section>

      {posts.length === 0 ? (
        <p>{t.noPosts}</p>
      ) : (
        <>
          <section className="mp-box mp-featured">
            <h2>{t.featured}</h2>
            <div className="mp-box-body">
              <p className="mp-meta">{formatDate(featured.publishedAt, lang)}</p>
              <h3 className="mp-featured-title">
                <a href={postHref(featured)}>{featured.title}</a>
              </h3>
              <p className="mp-featured-summary">{featured.summary}</p>
              <PostMeta post={featured} lang={lang} categories={categories} />
              <a className="mp-featured-more" href={postHref(featured)}>
                {t.fullArticle}
              </a>
            </div>
          </section>

          <section className="mp-groups" aria-label={t.browseByCategory}>
            {!onlyOther && <h2 className="mp-section-title">{t.browseByCategory}</h2>}
            <div className="mp-group-grid">
              {groups.map(({ category, posts: items }) => {
                const title = category.slug ? category.name : onlyOther ? t.recentArticles : t.otherArticles;
                return (
                  <section key={category.slug || "other"} className="mp-group">
                    <h3 className="mp-group-title">
                      {category.slug ? <a href={categoryHref(lang, category.slug)}>{title}</a> : title}
                      <span className="mp-group-count">{category.count}</span>
                    </h3>
                    <ul>
                      {items.map((p) => (
                        <li key={p.slug}>
                          <a href={postHref(p)}>{p.title}</a>
                          <span className="mp-meta">{formatDate(p.publishedAt, lang)}</span>
                        </li>
                      ))}
                    </ul>
                    {category.slug && category.count > items.length && (
                      <a className="mp-group-more" href={categoryHref(lang, category.slug)}>
                        {format(t.viewAll, { n: category.count })}
                      </a>
                    )}
                  </section>
                );
              })}
            </div>
          </section>
        </>
      )}
    </Layout>
  );
}

function ArticlePage({ lang, theme, year, post, availableLanguages, preview, pendingChanges, url, related, categories, t }: WithT<Extract<PageData, { page: "post" }>>) {
  const blocks = parseBody(post.body);
  const toc = sections(blocks);
  const translations = availableLanguages.filter((l) => l !== lang).map((l) => ({ lang: l, href: `/${l}/posts/${post.slug}` }));
  const published = formatDate(post.publishedAt, lang);

  const contents = toc.length > 0 && <Contents t={t} sections={toc} />;
  const categoryLabel = post.category ? categoryName(categories, post.category) : "";

  return (
    <Layout
      lang={lang}
      theme={theme}
      year={year}
      t={t}
      translations={translations}
      sidebar={contents || <MainMenu lang={lang} t={t} categories={categories} />}
      footer={<p>{format(t.footerPublished, { date: published })}</p>}
    >
      {preview && (
        <div className="preview-banner" role="note">
          <strong>Preview.</strong>{" "}
          {pendingChanges
            ? "These are unpublished changes; readers still see the published version."
            : post.status === "published"
              ? "This article is live."
              : `This ${post.status} article isn’t visible on the site.`}{" "}
          <a href={`/admin/posts/${post.slug}/${post.lang}`}>Back to the editor</a>
        </div>
      )}
      <BackToMain lang={lang} t={t} />
      <TitleBar t={t} title={post.title} subtitle={post.subtitle} tab={t.article} />
      <p className="from-site">{t.fromSite}</p>
      {!preview && <ShareBar url={url} title={post.title} t={t} className="share-top" />}
      {post.cover && (
        <figure className="article-cover">
          <img src={post.cover} alt="" />
        </figure>
      )}
      {/* The summary introduces the article, under the cover (or the share buttons without one). */}
      {post.summary && (
        <section className="article-summary" aria-labelledby="article-summary">
          <h2 id="article-summary" className="side-heading">
            {t.summaryHeading}
          </h2>
          <p>{post.summary}</p>
        </section>
      )}

      <div className="article">
        <div className="article-body">
          {/* On narrow screens the sidebar is hidden, so contents move inline. */}
          {contents && <div className="toc-inline">{contents}</div>}

          {blocks.map((b, i) => renderBlock(b, i))}

          {!!post.tags?.length && (
            <footer className="article-tags">
              <span className="article-tags-label">{t.tags}</span>
              <TagLinks lang={lang} tags={post.tags} />
            </footer>
          )}

          {!preview && <ShareBar url={url} title={post.title} t={t} />}

          {/* On narrow screens the infobox sits above the text, so suggestions move to the end. */}
          <RelatedArticles lang={lang} posts={related} categories={categories} t={t} className="related-inline" />

          <BackToMain lang={lang} t={t} className="back-link-end" />
        </div>

        <aside className="article-aside">
          <table className="infobox">
            <tbody>
              <tr>
                <th colSpan={2} className="infobox-title">
                  {post.title}
                </th>
              </tr>
              {categoryLabel && (
                <tr>
                  <th scope="row">{t.category}</th>
                  <td>
                    <a href={categoryHref(lang, post.category)}>{categoryLabel}</a>
                  </td>
                </tr>
              )}
              <tr>
                <th scope="row">{t.infoPublished}</th>
                <td>
                  <time dateTime={post.publishedAt}>{published}</time>
                </td>
              </tr>
              <tr>
                <th scope="row">{t.infoLanguage}</th>
                <td>{t.languageName}</td>
              </tr>
              {translations.length > 0 && (
                <tr>
                  <th scope="row">{t.infoAlsoIn}</th>
                  <td>
                    {translations.map((tr, i) => (
                      <Fragment key={tr.lang}>
                        {i > 0 && ", "}
                        <a href={tr.href} hrefLang={tr.lang}>
                          {getDictionary(tr.lang).languageName}
                        </a>
                      </Fragment>
                    ))}
                  </td>
                </tr>
              )}
              <tr>
                <th scope="row">{t.infoReadingTime}</th>
                <td>{format(t.minutes, { n: readingMinutes(post.body) })}</td>
              </tr>
            </tbody>
          </table>
          <RelatedArticles lang={lang} posts={related} categories={categories} t={t} />
        </aside>
      </div>
    </Layout>
  );
}

function SearchPage(props: WithT<Extract<PageData, { page: "search" }>>) {
  const { lang, theme, year, scope, query, tag, category, results, tags, filtered, categories, t } = props;
  const count = results.length;
  const catName = category ? categoryName(categories, category) || category : "";
  const title = scope === "category" ? format(t.categoryTitle, { name: catName }) : scope === "tag" ? format(t.tagTitle, { tag }) : t.searchResults;
  const tab = scope === "category" ? t.category : scope === "tag" ? t.tags : t.search;
  const params = (l: Locale) => {
    const qs = queryString({ q: query, tag: scope === "tag" ? "" : tag, category: scope === "category" ? "" : category });
    const path = scope === "category" ? categoryHref(l, category) : scope === "tag" ? tagHref(l, tag) : `/${l}/search`;
    return qs ? `${path}?${qs}` : path;
  };

  let summary: string;
  if (!filtered) summary = t.searchPrompt;
  else if (count === 0) summary = query && !tag && !category ? format(t.searchNoResults, { q: query }) : t.noMatches;
  else if (query && !tag && !category) summary = format(count === 1 ? t.searchCountOne : t.searchCount, { n: count, q: query });
  else summary = format(count === 1 ? t.filteredCountOne : t.filteredCount, { n: count });

  return (
    <Layout
      lang={lang}
      theme={theme}
      year={year}
      t={t}
      query={query}
      translations={otherLocales(lang, params)}
      sidebar={<MainMenu lang={lang} t={t} categories={categories} />}
    >
      <TitleBar t={t} title={title} tab={tab} />
      {scope === "category" && <p className="from-site">{format(t.categoryIntro, { name: catName })}</p>}
      {scope === "tag" && <p className="from-site">{format(t.tagIntro, { tag: `#${tag}` })}</p>}

      <form className="browse-filters" role="search" action={`/${lang}/search`} method="get">
        <label className="browse-field browse-text">
          <span>{t.searchText}</span>
          <input type="search" name="q" defaultValue={query} placeholder={t.searchPlaceholder} />
        </label>
        <label className="browse-field">
          <span>{t.category}</span>
          <select name="category" defaultValue={category}>
            <option value="">{t.allCategories}</option>
            {(categories ?? []).map((c) => (
              <option key={c.slug} value={c.slug}>
                {c.name} ({c.count})
              </option>
            ))}
          </select>
        </label>
        <label className="browse-field">
          <span>{t.tags}</span>
          <select name="tag" defaultValue={tag}>
            <option value="">{t.allTags}</option>
            {tags.map((x) => (
              <option key={x.tag} value={x.tag}>
                #{x.tag} ({x.count})
              </option>
            ))}
          </select>
        </label>
        <div className="browse-actions">
          <button type="submit" className="browse-submit">
            {t.applyFilters}
          </button>
          {filtered && (
            <a className="browse-clear" href={`/${lang}/search`}>
              {t.clearFilters}
            </a>
          )}
        </div>
      </form>

      <p className="search-info">{summary}</p>

      {filtered ? (
        count > 0 && (
          <ul className="search-results">
            {results.map((p) => (
              <li key={p.slug}>
                <a className="result-title" href={postHref(p)}>
                  {highlight(p.title, query)}
                </a>
                <p className="result-snippet">{highlight(p.summary, query)}</p>
                <div className="result-meta">
                  <span>{formatDate(p.publishedAt, lang)}</span>
                  <PostMeta post={p} lang={lang} categories={categories} />
                </div>
              </li>
            ))}
          </ul>
        )
      ) : (
        <div className="browse-overview">
          {!!categories?.length && (
            <section>
              <h2 className="side-heading">{t.browseByCategory}</h2>
              <ul className="category-cloud">
                {categories.map((c) => (
                  <li key={c.slug}>
                    <a href={categoryHref(lang, c.slug)}>
                      {c.name} <span>{c.count}</span>
                    </a>
                  </li>
                ))}
              </ul>
            </section>
          )}
          {tags.length > 0 && (
            <section>
              <h2 className="side-heading">{t.popularTags}</h2>
              <TagLinks lang={lang} tags={tags.map((x) => x.tag)} className="tag-list tag-cloud" />
            </section>
          )}
        </div>
      )}
    </Layout>
  );
}

function NotFoundPage({ lang, theme, year, categories, t }: WithT<Extract<PageData, { page: "notFound" }>>) {
  const [before, after] = t.notFoundHint.split("{link}");
  return (
    <Layout lang={lang} theme={theme} year={year} t={t} sidebar={<MainMenu lang={lang} t={t} categories={categories} />}>
      <TitleBar t={t} title={t.notFoundTitle} tab={t.article} />
      <div className="notice">
        <p>
          <b>{t.notFoundBody}</b>
        </p>
        <p>
          {before}
          <a href={`/${lang}`}>{t.mainPage}</a>
          {after}
        </p>
        <SearchForm lang={lang} t={t} className="page-search" />
      </div>
    </Layout>
  );
}

// Other articles sharing tags or the category, under the infobox.
function RelatedArticles({ lang, posts, categories, t, className = "" }: { lang: Locale; posts?: Post[] | null; categories?: NavCategory[]; t: Dictionary; className?: string }) {
  if (!posts?.length) return null;
  return (
    <nav className={`related ${className}`.trim()} aria-label={t.relatedArticles}>
      <h2 className="side-heading">{t.relatedArticles}</h2>
      <ul className="related-list">
        {posts.map((p) => {
          const category = p.category ? categoryName(categories, p.category) : "";
          return (
            <li key={p.slug}>
              <a href={postHref(p)}>{p.title}</a>
              <span className="related-meta">
                <time dateTime={p.publishedAt}>{formatDate(p.publishedAt, lang)}</time>
                {category && ` · ${category}`}
              </span>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

function BackToMain({ lang, t, className = "" }: { lang: Locale; t: Dictionary; className?: string }) {
  return (
    <p className={`back-link ${className}`.trim()}>
      <a href={`/${lang}`}>
        <span aria-hidden="true">← </span>
        {t.backToMain}
      </a>
    </p>
  );
}

function postHref(p: Post) {
  return `/${p.lang}/posts/${p.slug}`;
}

// Wraps case-insensitive occurrences of query in <mark>.
function highlight(text: string, query: string): ReactNode {
  const q = query.toLowerCase();
  if (!q) return text;
  const lower = text.toLowerCase();
  const parts: ReactNode[] = [];
  let start = 0;
  for (let i = lower.indexOf(q); i !== -1; i = lower.indexOf(q, start)) {
    if (i > start) parts.push(text.slice(start, i));
    parts.push(<mark key={i}>{text.slice(i, i + q.length)}</mark>);
    start = i + q.length;
  }
  parts.push(text.slice(start));
  return parts;
}

type Head = { title: string; description: string; siteName?: string; image?: string };

// Document <head> fields for a page; the Go server writes them into the HTML
// shell, including the Open Graph tags that social networks read for link previews.
export function head(props: PublicPageData): Head {
  return { siteName: getDictionary(props.lang).siteTitle, ...pageHead(props) };
}

function pageHead(props: PublicPageData): Head {
  const t = getDictionary(props.lang);
  switch (props.page) {
    case "home":
      return { title: `${t.siteTitle} – ${t.siteTagline}`, description: t.siteDescription };
    case "post":
      // The cover becomes the link-preview picture; without one the server uses the logo.
      return { title: `${props.post.title} – ${t.siteTitle}`, description: props.post.summary, image: props.post.cover || undefined };
    case "search": {
      if (props.scope === "category") {
        const name = categoryName(props.categories, props.category) || props.category;
        return { title: `${name} – ${t.siteTitle}`, description: format(t.categoryIntro, { name }) };
      }
      if (props.scope === "tag") {
        return { title: `#${props.tag} – ${t.siteTitle}`, description: format(t.tagIntro, { tag: `#${props.tag}` }) };
      }
      return {
        title: `${props.query ? `${props.query} – ` : ""}${t.searchResults} – ${t.siteTitle}`,
        description: t.siteDescription,
      };
    }
    case "notFound":
      return { title: `${t.notFoundTitle} – ${t.siteTitle}`, description: t.notFoundBody };
    case "about":
      return {
        title: `${props.profile.name || t.aboutTitle} – ${t.siteTitle}`,
        description: props.profile.headline || t.siteDescription,
      };
  }
}
