import { getDictionary, type Locale } from "./i18n";

// Formats without Intl: goja (the Go-side JS engine) doesn't implement it, and
// server and browser output must match exactly for hydration.
export function formatDate(iso: string, lang: Locale) {
  const d = new Date(iso);
  const t = getDictionary(lang);
  return t.dateFormat
    .replace("{day}", String(d.getUTCDate()))
    .replace("{month}", t.months[d.getUTCMonth()])
    .replace("{year}", String(d.getUTCFullYear()));
}
