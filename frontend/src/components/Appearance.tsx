import { useState } from "react";

import type { Dictionary } from "../lib/i18n";
import type { Theme } from "../types";

const THEMES: Theme[] = ["auto", "light", "dark"];

// Applies a theme to the page and remembers it in the cookie the server reads.
function applyTheme(next: Theme) {
  const root = document.documentElement;
  if (next === "auto") delete root.dataset.theme;
  else root.dataset.theme = next;
  document.cookie =
    next === "auto" ? "theme=; Path=/; Max-Age=0; SameSite=Lax" : `theme=${next}; Path=/; Max-Age=31536000; SameSite=Lax`;
}

// Wikipedia-style "Appearance" menu. The choice is stored in a cookie that the
// Go server reads, so the next page is rendered in the right theme up front.
export function AppearanceMenu({ t, initial }: { t: Dictionary; initial: Theme }) {
  const [theme, setTheme] = useState<Theme>(initial);
  const labels: Record<Theme, string> = { auto: t.themeAuto, light: t.themeLight, dark: t.themeDark };

  function choose(next: Theme) {
    setTheme(next);
    applyTheme(next);
  }

  return (
    <details className="appearance">
      <summary title={t.appearance}>
        <svg className="appearance-icon" viewBox="0 0 20 20" aria-hidden="true">
          <circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" strokeWidth="1.8" />
          <path d="M10 2a8 8 0 0 1 0 16z" fill="currentColor" />
        </svg>
        <span className="appearance-label">{t.appearance}</span>
      </summary>
      <div className="appearance-panel">
        <fieldset>
          <legend>{t.color}</legend>
          {THEMES.map((value) => (
            <label key={value}>
              <input type="radio" name="theme" value={value} checked={theme === value} onChange={() => choose(value)} />
              {labels[value]}
            </label>
          ))}
        </fieldset>
      </div>
    </details>
  );
}
