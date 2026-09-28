import type { ReactNode } from "react";

import { type Block, parseBody } from "../lib/article";

// Renders an article body. Everything becomes React elements, so text is
// always escaped; only vetted URL schemes reach href/src attributes.
export function Markdown({ blocks, body }: { blocks?: Block[]; body?: string }) {
  return <>{(blocks ?? parseBody(body ?? "")).map((b, i) => renderBlock(b, i))}</>;
}

export function renderBlock(b: Block, key: number): ReactNode {
  switch (b.kind) {
    case "h2":
      return (
        <h2 key={key} id={b.id}>
          {inline(b.text)}
        </h2>
      );
    case "h3":
      return (
        <h3 key={key} id={b.id}>
          {inline(b.text)}
        </h3>
      );
    case "p":
      return <p key={key}>{inline(b.text)}</p>;
    case "quote":
      return (
        <blockquote key={key}>
          <p>{inline(b.text)}</p>
        </blockquote>
      );
    case "ul":
    case "ol": {
      const List = b.kind;
      return (
        <List key={key}>
          {b.items.map((item, i) => (
            <li key={i}>{inline(item)}</li>
          ))}
        </List>
      );
    }
    case "code":
      return (
        <pre key={key} data-lang={b.lang || undefined}>
          <code>{b.code}</code>
        </pre>
      );
    case "img":
      if (!safeImageSrc(b.src)) return <p key={key}>{b.alt}</p>;
      return (
        <figure key={key}>
          <img src={b.src} alt={b.alt} loading="lazy" decoding="async" />
          {(b.title || b.alt) && <figcaption>{b.title || b.alt}</figcaption>}
        </figure>
      );
    case "hr":
      return <hr key={key} />;
  }
}

// Links may point to the web, email, this site, or an anchor on the page.
function safeHref(url: string): boolean {
  return /^(https?:\/\/|mailto:|\/(?!\/)|#)/i.test(url);
}

// Images come from the web over http(s) or from this site's uploads.
export function safeImageSrc(url: string): boolean {
  return /^(https?:\/\/|\/media\/)/i.test(url);
}

type LinkParts = { label: string; url: string; title: string; end: number };

// Parses "[label](url "title")" starting at text[start] === "[". Like
// CommonMark, the address may contain balanced parentheses, as in
// https://en.wikipedia.org/wiki/Go_(programming_language).
function linkAt(text: string, start: number): LinkParts | null {
  const close = text.indexOf("](", start);
  if (close < 0) return null;
  let end = -1;
  for (let i = close + 2, depth = 1; i < text.length; i++) {
    if (text[i] === "\\") i++;
    else if (text[i] === "(") depth++;
    else if (text[i] === ")" && --depth === 0) {
      end = i;
      break;
    }
  }
  if (end < 0) return null;
  const inside = text.slice(close + 2, end).trim();
  const m = /^(\S+)(?:\s+"([^"]*)")?$/.exec(inside);
  if (!m) return null;
  return { label: text.slice(start + 1, close), url: m[1], title: m[2] ?? "", end: end + 1 };
}

const isWordChar = (c: string | undefined) => !!c && /[\p{L}\p{N}]/u.test(c);

// Whether text[i] is escaped by an odd number of backslashes before it.
function escaped(text: string, i: number): boolean {
  let n = 0;
  while (i - n - 1 >= 0 && text[i - n - 1] === "\\") n++;
  return n % 2 === 1;
}

// indexOf for a closing marker that skips escaped ones ("\*" is a literal *).
function closing(text: string, marker: string, from: number): number {
  let i = text.indexOf(marker, from);
  while (i >= 0 && escaped(text, i)) i = text.indexOf(marker, i + 1);
  return i;
}

// Inline Markdown → React nodes.
export function inline(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let buf = "";
  const flush = () => {
    if (buf) out.push(buf);
    buf = "";
  };

  let i = 0;
  outer: while (i < text.length) {
    const ch = text[i];

    // A backslash makes any ASCII punctuation literal, as in CommonMark.
    if (ch === "\\" && i + 1 < text.length && /[!-/:-@[-`{-~]/.test(text[i + 1])) {
      buf += text[i + 1];
      i += 2;
      continue;
    }
    if (ch === "`") {
      const end = text.indexOf("`", i + 1);
      if (end > i + 1) {
        flush();
        out.push(<code key={out.length}>{text.slice(i + 1, end)}</code>);
        i = end + 1;
        continue;
      }
    }
    if (ch === "!" && text[i + 1] === "[") {
      const link = linkAt(text, i + 1);
      if (link && safeImageSrc(link.url)) {
        flush();
        out.push(<img key={out.length} src={link.url} alt={link.label} title={link.title || undefined} loading="lazy" />);
        i = link.end;
        continue;
      }
    }
    if (ch === "[") {
      const link = linkAt(text, i);
      if (link && safeHref(link.url)) {
        flush();
        const external = /^https?:\/\//i.test(link.url);
        out.push(
          <a
            key={out.length}
            href={link.url}
            title={link.title || undefined}
            {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
          >
            {inline(link.label)}
          </a>,
        );
        i = link.end;
        continue;
      }
    }
    for (const [delim, Tag] of [
      ["**", "strong"],
      ["__", "strong"],
      ["~~", "del"],
    ] as const) {
      if (text.startsWith(delim, i)) {
        const end = closing(text, delim, i + 2);
        if (end > i + 2) {
          flush();
          out.push(<Tag key={out.length}>{inline(text.slice(i + 2, end))}</Tag>);
          i = end + 2;
          continue outer;
        }
      }
    }
    if ((ch === "*" || ch === "_") && text[i + 1] && text[i + 1] !== " " && text[i + 1] !== ch) {
      // "_" only works at word boundaries, so snake_case stays intact.
      if (ch === "*" || !isWordChar(text[i - 1])) {
        let end = closing(text, ch, i + 1);
        while (end > 0 && (text[end - 1] === " " || (ch === "_" && isWordChar(text[end + 1])))) end = closing(text, ch, end + 1);
        if (end > i + 1) {
          flush();
          out.push(<em key={out.length}>{inline(text.slice(i + 1, end))}</em>);
          i = end + 1;
          continue;
        }
      }
    }
    buf += ch;
    i++;
  }
  flush();
  return out;
}
