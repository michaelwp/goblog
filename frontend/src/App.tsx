import { Fragment, type ReactNode } from "react";

import { Contents } from "./components/Article";
import { renderBlock } from "./components/Markdown";
import { LanguageMenu, Layout, MainMenu, SearchForm, TitleBar, type Translation } from "./components/Layout";
import { parseBody, readingMinutes, sections } from "./lib/article";
import { formatDate } from "./lib/format";
import { type Dictionary, format, getDictionary, type Locale, locales } from "./lib/i18n";
import { AboutPage } from "./pages/About";
import type { PageData, Post, PublicPageData } from "./types";

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

function HomePage({ lang, theme, year, posts, t }: WithT<Extract<PageData, { page: "home" }>>) {
  const [featured, ...recent] = posts;
  return (
    <Layout lang={lang} theme={theme} year={year} t={t} sidebar={<MainMenu lang={lang} t={t} />}>
      <section className="mp-banner">
        <div className="mp-hero-row">
          <h1 className="mp-welcome">{format(t.welcome, { site: t.siteTitle })}</h1>
          <LanguageMenu t={t} translations={otherLocales(lang, (l) => `/${l}`)} />
        </div>
        <p className="mp-tagline">{t.welcomeTagline}</p>
        <p className="mp-count">{format(posts.length === 1 ? t.articleCountOne : t.articleCount, { n: posts.length })}</p>
      </section>

      {posts.length === 0 ? (
        <p>{t.noPosts}</p>
      ) : (
        <div className="mp-columns">
          <section className="mp-box mp-featured">
            <h2>{t.featured}</h2>
            <div className="mp-box-body">
              <p className="mp-meta">{formatDate(featured.publishedAt, lang)}</p>
              <h3 className="mp-featured-title">
                <a href={postHref(featured)}>{featured.title}</a>
              </h3>
              <p className="mp-featured-summary">{featured.summary}</p>
              <a className="mp-featured-more" href={postHref(featured)}>
                {t.fullArticle}
              </a>
            </div>
          </section>
          <section className="mp-box mp-recent">
            <h2>{t.recentArticles}</h2>
            <div className="mp-box-body">
              <ul>
                {(recent.length > 0 ? recent : posts).map((p) => (
                  <li key={p.slug}>
                    <a href={postHref(p)}>{p.title}</a> <span className="mp-meta">{formatDate(p.publishedAt, lang)}</span>
                  </li>
                ))}
              </ul>
            </div>
          </section>
        </div>
      )}
    </Layout>
  );
}

function ArticlePage({ lang, theme, year, post, availableLanguages, preview, pendingChanges, t }: WithT<Extract<PageData, { page: "post" }>>) {
  const blocks = parseBody(post.body);
  const toc = sections(blocks);
  const translations = availableLanguages.filter((l) => l !== lang).map((l) => ({ lang: l, href: `/${l}/posts/${post.slug}` }));
  const published = formatDate(post.publishedAt, lang);

  const contents = toc.length > 0 && <Contents t={t} sections={toc} />;

  return (
    <Layout
      lang={lang}
      theme={theme}
      year={year}
      t={t}
      sidebar={contents || <MainMenu lang={lang} t={t} />}
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
      <TitleBar t={t} title={post.title} tab={t.article} translations={translations} />
      <p className="from-site">{t.fromSite}</p>

      <div className="article">
        <div className="article-body">
          {/* On narrow screens the sidebar is hidden, so contents move inline. */}
          {contents && <div className="toc-inline">{contents}</div>}

          {blocks.map((b, i) => renderBlock(b, i))}
        </div>

        <aside className="article-aside">
          <table className="infobox">
            <tbody>
              <tr>
                <th colSpan={2} className="infobox-title">
                  {post.title}
                </th>
              </tr>
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
        </aside>
      </div>
    </Layout>
  );
}

function SearchPage({ lang, theme, year, query, results, t }: WithT<Extract<PageData, { page: "search" }>>) {
  const count = results.length;
  return (
    <Layout lang={lang} theme={theme} year={year} t={t} query={query} sidebar={<MainMenu lang={lang} t={t} />}>
      <TitleBar
        t={t}
        title={t.searchResults}
        tab={t.search}
        translations={otherLocales(lang, (l) => `/${l}/search${query ? `?q=${encodeURIComponent(query)}` : ""}`)}
      />
      <SearchForm lang={lang} t={t} query={query} className="page-search" />

      {!query ? (
        <p className="search-info">{t.searchPrompt}</p>
      ) : count === 0 ? (
        <p className="search-info">{format(t.searchNoResults, { q: query })}</p>
      ) : (
        <>
          <p className="search-info">{format(count === 1 ? t.searchCountOne : t.searchCount, { n: count, q: query })}</p>
          <ul className="search-results">
            {results.map((p) => (
              <li key={p.slug}>
                <a className="result-title" href={postHref(p)}>
                  {highlight(p.title, query)}
                </a>
                <p className="result-snippet">{highlight(p.summary, query)}</p>
                <p className="result-meta">{formatDate(p.publishedAt, lang)}</p>
              </li>
            ))}
          </ul>
        </>
      )}
    </Layout>
  );
}

function NotFoundPage({ lang, theme, year, t }: WithT<Extract<PageData, { page: "notFound" }>>) {
  const [before, after] = t.notFoundHint.split("{link}");
  return (
    <Layout lang={lang} theme={theme} year={year} t={t} sidebar={<MainMenu lang={lang} t={t} />}>
      <TitleBar t={t} title={t.notFoundTitle} tab={t.article} translations={[]} />
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

// Document <head> fields for a page; the Go server writes them into the HTML shell.
export function head(props: PublicPageData): { title: string; description: string } {
  const t = getDictionary(props.lang);
  switch (props.page) {
    case "home":
      return { title: `${t.siteTitle} – ${t.siteTagline}`, description: t.siteDescription };
    case "post":
      return { title: `${props.post.title} – ${t.siteTitle}`, description: props.post.summary };
    case "search":
      return {
        title: `${props.query ? `${props.query} – ` : ""}${t.searchResults} – ${t.siteTitle}`,
        description: t.siteDescription,
      };
    case "notFound":
      return { title: `${t.notFoundTitle} – ${t.siteTitle}`, description: t.notFoundBody };
    case "about":
      return {
        title: `${props.profile.name || t.aboutTitle} – ${t.siteTitle}`,
        description: props.profile.headline || t.siteDescription,
      };
  }
}
