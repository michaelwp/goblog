import type { ReactNode } from "react";

import { type Dictionary, getDictionary, type Locale } from "../lib/i18n";
import type { NavCategory, Theme } from "../types";
import { CategoryMenu } from "./Taxonomy";
import { AppearanceMenu } from "./Appearance";

type LayoutProps = {
  lang: Locale;
  theme: Theme;
  year: number;
  t: Dictionary;
  query?: string;
  // The same page in the site's other languages, for the header's language menu.
  translations?: Translation[];
  sidebar: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
};

export function Layout({ lang, theme, year, t, query, translations = [], sidebar, footer, children }: LayoutProps) {
  return (
    <div className="page">
      <a className="skip-link" href="#content">
        {t.jumpToContent}
      </a>
      <header className="site-header">
        <a className="logo" href={`/${lang}`} aria-label={t.mainPage}>
          <LogoMark />
          <span className="wordmark">
            <span className="wordmark-title">{wordmarkTitle(t.siteTitle)}</span>
            <span className="wordmark-tagline">{t.siteTagline}</span>
          </span>
        </a>
        <SearchForm lang={lang} t={t} query={query} className="header-search" />
        <div className="header-actions">
          <LanguageMenu t={t} translations={translations} />
          <AppearanceMenu t={t} initial={theme} />
        </div>
      </header>

      <div className="layout">
        <aside className="sidebar">{sidebar}</aside>
        <main className="content" id="content">
          {children}
        </main>
      </div>

      <footer className="site-footer">
        {footer}
        <p>{`goblog.dev © ${year}`}</p>
      </footer>
    </div>
  );
}

export function SearchForm(props: { lang: Locale; t: Dictionary; query?: string; className?: string }) {
  const { lang, t, query, className } = props;
  return (
    <form className={`search ${className ?? ""}`} role="search" action={`/${lang}/search`} method="get">
      <span className="search-field">
        <svg className="search-icon" viewBox="0 0 20 20" aria-hidden="true">
          <circle cx="8.5" cy="8.5" r="5.75" fill="none" stroke="currentColor" strokeWidth="1.8" />
          <path d="M13 13l4.5 4.5" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
        </svg>
        <input type="search" name="q" defaultValue={query} placeholder={t.searchPlaceholder} aria-label={t.searchPlaceholder} />
      </span>
      <button type="submit">{t.search}</button>
    </form>
  );
}

export function MainMenu({ lang, t, categories }: { lang: Locale; t: Dictionary; categories?: NavCategory[] }) {
  // One wrapper: the sidebar makes its direct child sticky.
  return (
    <div className="side-menus">
      <nav className="side-nav" aria-label={t.mainMenu}>
        <h2 className="side-heading">{t.mainMenu}</h2>
        <ul>
          <li>
            <a href={`/${lang}`}>{t.mainPage}</a>
          </li>
          <li>
            <a href={`/${lang}/about`}>{t.about}</a>
          </li>
          <li>
            <a href={`/${lang}/search`}>{t.search}</a>
          </li>
        </ul>
      </nav>
      <CategoryMenu lang={lang} t={t} categories={categories} />
    </div>
  );
}

export type Translation = { lang: Locale; href: string };

// The page heading: title, then the tab strip underneath, modelled on
// Wikipedia's article header. The language menu lives in the site header.
export function TitleBar(props: { t: Dictionary; title: string; tab: string }) {
  const { t, title, tab } = props;
  return (
    <div className="title-bar">
      <div className="title-row">
        <h1 id="top" className="page-title">
          {title}
        </h1>
      </div>
      <div className="tabs">
        <span className="tab is-selected">{tab}</span>
        <span className="tab is-selected">{t.read}</span>
      </div>
    </div>
  );
}

export function LanguageMenu({ t, translations }: { t: Dictionary; translations: Translation[] }) {
  if (translations.length === 0) return null;
  const n = translations.length;
  const label = (n === 1 ? t.languagesOne : t.languagesMany).replace("{n}", String(n));
  return (
    <details className="lang-menu">
      <summary aria-label={label}>
        <span className="lang-icon" aria-hidden="true">
          文A
        </span>
        <span className="lang-label">{label}</span>
      </summary>
      <ul>
        {translations.map(({ lang, href }) => (
          <li key={lang}>
            <a href={href} hrefLang={lang} lang={lang}>
              {getDictionary(lang).languageName}
            </a>
          </li>
        ))}
      </ul>
    </details>
  );
}

// The "Go" logo mark spells the start of "GoBlog.dev", so the title beside it
// drops it: the header reads "[Go] Blog.dev".
function wordmarkTitle(title: string) {
  return title.startsWith("Go") ? title.slice(2) : title;
}

// The site name in running text, with "Go" set white on black like the logo.
export function SiteName({ name }: { name: string }) {
  if (!name.startsWith("Go")) return name;
  return (
    <span className="site-name">
      <span className="go-badge">Go</span>
      {name.slice(2)}
    </span>
  );
}

function LogoMark() {
  return (
    <svg className="logo-mark" viewBox="0 0 40 40" aria-hidden="true">
      <rect x="1" y="1" width="38" height="38" rx="11" />
      <text x="20" y="27" textAnchor="middle" fontFamily="'Iowan Old Style', Charter, Georgia, serif" fontSize="19" fontWeight="600" fill="#fff">
        Go
      </text>
    </svg>
  );
}
