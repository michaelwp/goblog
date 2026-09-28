import { Contents } from "../components/Article";
import { renderBlock } from "../components/Markdown";
import { Layout, MainMenu, TitleBar } from "../components/Layout";
import { parseBody, sections } from "../lib/article";
import { countryName, flagEmoji } from "../lib/countries";
import { type Dictionary, locales } from "../lib/i18n";
import type { PageData } from "../types";

type Props = Extract<PageData, { page: "about" }> & { t: Dictionary };

// The blog owner's profile, laid out like a biography article: the name as
// title, headline as the lead, bio sections with contents, and an infobox.
export function AboutPage({ lang, theme, year, profile, categories, t }: Props) {
  const title = profile.name || t.aboutTitle;
  const translations = locales.filter((l) => l !== lang).map((l) => ({ lang: l, href: `/${l}/about` }));
  const blocks = parseBody(profile.bio);
  const toc = sections(blocks);
  const contents = toc.length > 0 && <Contents t={t} sections={toc} />;
  const fallback = profile.textLang !== lang;
  const hasInfo = profile.photoUrl || profile.location || profile.country || profile.email || profile.links.length > 0;
  const country = profile.country ? countryName(profile.country, lang) : "";

  return (
    <Layout lang={lang} theme={theme} year={year} t={t} sidebar={contents || <MainMenu lang={lang} t={t} categories={categories} />}>
      <TitleBar t={t} title={title} tab={t.about} translations={translations} />
      <p className="from-site">{t.fromSite}</p>

      {profile.empty ? (
        <div className="notice">
          <p>{t.profileEmpty}</p>
        </div>
      ) : (
        <div className="article">
          <div className="article-body" lang={fallback ? profile.textLang : undefined}>
            {fallback && <p className="profile-fallback">{t.profileFallback}</p>}
            {profile.headline && <p className="profile-lead">{profile.headline}</p>}
            {contents && <div className="toc-inline">{contents}</div>}
            {blocks.map((b, i) => renderBlock(b, i))}
          </div>

          {hasInfo && (
            <aside className="article-aside">
              <table className="infobox">
                <tbody>
                  <tr>
                    <th colSpan={2} className="infobox-title">
                      {title}
                    </th>
                  </tr>
                  {profile.photoUrl && (
                    <tr>
                      <td colSpan={2} className="infobox-photo">
                        <img src={profile.photoUrl} alt={profile.name} referrerPolicy="no-referrer" loading="lazy" />
                      </td>
                    </tr>
                  )}
                  {(profile.location || country) && (
                    <tr>
                      <th scope="row">{t.infoLocation}</th>
                      <td>
                        {country && (
                          <span className="flag" role="img" aria-label={country} title={country}>
                            {flagEmoji(profile.country)}
                          </span>
                        )}
                        {profile.location || country}
                      </td>
                    </tr>
                  )}
                  {profile.email && (
                    <tr>
                      <th scope="row">{t.infoEmail}</th>
                      <td>
                        <a href={`mailto:${profile.email}`}>{profile.email}</a>
                      </td>
                    </tr>
                  )}
                  {profile.links.length > 0 && (
                    <tr>
                      <th scope="row">{t.infoLinks}</th>
                      <td>
                        <ul className="infobox-links">
                          {profile.links.map((l) => (
                            <li key={l.url}>
                              <a href={l.url} rel="me noopener" target="_blank">
                                {l.label}
                              </a>
                            </li>
                          ))}
                        </ul>
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </aside>
          )}
        </div>
      )}
    </Layout>
  );
}
