import type { Dictionary } from "../lib/i18n";

export function Contents({ t, sections }: { t: Dictionary; sections: { id: string; text: string }[] }) {
  return (
    <nav className="toc" aria-label={t.contents}>
      <h2 className="side-heading">{t.contents}</h2>
      <ul>
        <li>
          <a href="#top">{t.top}</a>
        </li>
        {sections.map((s) => (
          <li key={s.id}>
            <a href={`#${s.id}`}>{s.text}</a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
