import { type ReactNode, useState } from "react";

import { format, type Dictionary } from "../lib/i18n";
import { type ShareNetwork, shareLinks } from "../lib/share";
import { useHydrated } from "../lib/useHydrated";

// Round share buttons with each network's logo, shown above and below an
// article. "Copy link" and the device's own share sheet need JavaScript, so
// they appear only after hydration.
export function ShareBar({ url, title, t, className = "" }: { url: string; title: string; t: Dictionary; className?: string }) {
  const hydrated = useHydrated();
  const [copied, setCopied] = useState(false);
  const canCopy = hydrated && !!navigator.clipboard;
  const canShare = hydrated && typeof navigator.share === "function";

  const copy = () => {
    navigator.clipboard.writeText(url).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      },
      () => {}, // blocked by the browser; the address bar still has the link
    );
  };
  // Rejects when the reader closes the sheet, which isn't an error.
  const share = () => navigator.share({ title, url }).catch(() => {});

  return (
    <div className={`share ${className}`.trim()} role="group" aria-label={t.shareArticle}>
      <span className="article-tags-label" aria-hidden="true">
        {t.share}
      </span>
      <ul className="share-list">
        {shareLinks(url, title).map((l) => {
          const label = l.network === "email" ? t.shareByEmail : format(t.shareOn, { network: l.name });
          return (
            <li key={l.network}>
              <a
                className={`share-button share-${l.network}`}
                href={l.href}
                aria-label={label}
                title={label}
                {...(l.network !== "email" && { target: "_blank", rel: "noopener noreferrer" })}
              >
                {icons[l.network]}
              </a>
            </li>
          );
        })}
        {canCopy && (
          <li>
            <button type="button" className="share-button" onClick={copy} aria-label={t.copyLink} title={copied ? t.linkCopied : t.copyLink}>
              {copied ? icons.copied : icons.link}
              <span className="visually-hidden" aria-live="polite">
                {copied ? t.linkCopied : ""}
              </span>
            </button>
          </li>
        )}
        {canShare && (
          <li>
            <button type="button" className="share-button" onClick={share} aria-label={t.shareMore} title={t.shareMore}>
              {icons.more}
            </button>
          </li>
        )}
      </ul>
    </div>
  );
}

// Logos from Simple Icons (CC0); the rest are line icons. All draw in
// currentColor on a 24×24 grid.
const logo = (d: string, viewBox = "0 0 24 24") => (
  <svg viewBox={viewBox} aria-hidden="true" focusable="false">
    <path d={d} fill="currentColor" />
  </svg>
);
const line = (children: ReactNode) => (
  <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    {children}
  </svg>
);

const icons: Record<ShareNetwork | "link" | "copied" | "more", ReactNode> = {
  x: logo("M18.901 1.153h3.68l-8.04 9.19L24 22.846h-7.406l-5.8-7.584-6.638 7.584H.474l8.6-9.83L0 1.154h7.594l5.243 6.932ZM17.61 20.644h2.039L6.486 3.24H4.298Z"),
  facebook: logo(
    "M9.101 23.691v-7.98H6.627v-3.667h2.474v-1.58c0-4.085 1.848-5.978 5.858-5.978.401 0 .955.042 1.468.103a8.68 8.68 0 0 1 1.141.195v3.325a8.623 8.623 0 0 0-.653-.036 26.805 26.805 0 0 0-.733-.009c-.707 0-1.259.096-1.675.309a1.686 1.686 0 0 0-.679.622c-.258.42-.374.995-.374 1.752v1.297h3.919l-.386 2.103-.287 1.564h-3.246v8.245",
    "3 2 18 22",
  ),
  linkedin: logo(
    "M20.447 20.452h-3.554v-5.569c0-1.328-.027-3.037-1.852-3.037-1.853 0-2.136 1.445-2.136 2.939v5.667H9.351V9h3.414v1.561h.046c.477-.9 1.637-1.85 3.37-1.85 3.601 0 4.267 2.37 4.267 5.455v6.286zM5.337 7.433c-1.144 0-2.063-.926-2.063-2.065 0-1.138.92-2.063 2.063-2.063 1.14 0 2.064.925 2.064 2.063 0 1.139-.925 2.065-2.064 2.065zm1.782 13.019H3.555V9h3.564v11.452z",
    "2 2 20 20",
  ),
  whatsapp: logo(
    "M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413Z",
  ),
  // Just the plane from the Telegram logo; the button supplies the circle.
  telegram: logo(
    "M16.906 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z",
    "3.9 5.2 14.2 14.2",
  ),
  email: line(
    <>
      <rect x="2" y="4" width="20" height="16" rx="2" />
      <path d="m22 7-10 6L2 7" />
    </>,
  ),
  link: line(
    <>
      <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
      <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
    </>,
  ),
  copied: line(<path d="M20 6 9 17l-5-5" />),
  more: line(
    <>
      <circle cx="18" cy="5" r="3" />
      <circle cx="6" cy="12" r="3" />
      <circle cx="18" cy="19" r="3" />
      <path d="m8.59 13.51 6.83 3.98M15.41 6.51l-6.82 3.98" />
    </>,
  ),
};
