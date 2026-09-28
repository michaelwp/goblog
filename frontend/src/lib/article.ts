// Article bodies are Markdown (a practical subset):
//
//   ## Heading / ### Subheading     paragraphs separated by blank lines
//   **bold**  *italic*  ~~strike~~  `code`  [link](https://…)  ![alt](/media/…)
//   > quote     - bullet     1. numbered     ``` code block ```     ---
//
// This file turns the text into blocks; components/Markdown.tsx renders them
// (including the inline styling) as React elements, never as raw HTML.

export type Block =
  | { kind: "h2" | "h3"; text: string; id: string }
  | { kind: "p" | "quote"; text: string }
  | { kind: "ul" | "ol"; items: string[] }
  | { kind: "code"; code: string; lang: string }
  | { kind: "img"; src: string; alt: string; title: string }
  | { kind: "hr" };

const BULLET = /^[-*+]\s+(.*)$/;
const NUMBERED = /^\d+[.)]\s+(.*)$/;
const IMAGE_ONLY = /^!\[([^\]]*)\]\((\S+?)(?:\s+"([^"]*)")?\)$/;

export function parseBody(body: string): Block[] {
  const lines = body.replace(/\r\n?/g, "\n").split("\n");
  const blocks: Block[] = [];
  const usedIds = new Set<string>();
  let para: string[] = [];

  const flush = () => {
    if (para.length) blocks.push({ kind: "p", text: para.join(" ") });
    para = [];
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const trimmed = line.trim();

    if (trimmed.startsWith("```")) {
      flush();
      const lang = trimmed.slice(3).trim();
      const code: string[] = [];
      for (i++; i < lines.length && !lines[i].trim().startsWith("```"); i++) code.push(lines[i]);
      blocks.push({ kind: "code", code: code.join("\n"), lang });
      continue;
    }
    if (trimmed === "") {
      flush();
      continue;
    }
    const heading = /^(#{2,3})\s+(.+)$/.exec(trimmed);
    if (heading) {
      flush();
      const text = heading[2].trim();
      blocks.push({ kind: heading[1].length === 2 ? "h2" : "h3", text, id: uniqueId(slugify(plainText(text)), usedIds) });
      continue;
    }
    if (/^(-{3,}|\*{3,})$/.test(trimmed)) {
      flush();
      blocks.push({ kind: "hr" });
      continue;
    }
    const img = IMAGE_ONLY.exec(trimmed);
    if (img) {
      flush();
      blocks.push({ kind: "img", alt: img[1], src: img[2], title: img[3] ?? "" });
      continue;
    }
    if (trimmed.startsWith(">")) {
      flush();
      const quote: string[] = [];
      for (; i < lines.length && lines[i].trim().startsWith(">"); i++) quote.push(lines[i].trim().replace(/^>\s?/, ""));
      i--;
      blocks.push({ kind: "quote", text: quote.join(" ") });
      continue;
    }
    const listKind = BULLET.test(trimmed) ? "ul" : NUMBERED.test(trimmed) ? "ol" : null;
    if (listKind) {
      flush();
      const re = listKind === "ul" ? BULLET : NUMBERED;
      const items: string[] = [];
      for (; i < lines.length; i++) {
        const m = re.exec(lines[i].trim());
        if (m) items.push(m[1]);
        else if (lines[i].trim() && /^\s+/.test(lines[i]) && items.length) items[items.length - 1] += " " + lines[i].trim(); // wrapped item
        else break;
      }
      i--;
      blocks.push({ kind: listKind, items });
      continue;
    }
    para.push(trimmed);
  }
  flush();
  return blocks;
}

// Sections for the table of contents: the ## headings.
export function sections(blocks: Block[]): { id: string; text: string }[] {
  return blocks.flatMap((b) => (b.kind === "h2" ? [{ id: b.id, text: plainText(b.text) }] : []));
}

export function readingMinutes(body: string): number {
  const words = plainText(body).split(/\s+/).filter(Boolean).length;
  return Math.max(1, Math.round(words / 200));
}

// Strips inline Markdown, e.g. for headings in the contents list.
export function plainText(text: string): string {
  return text
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/(\*\*|__|~~|`)/g, "")
    .replace(/(^|\W)[*_](\S[^*_]*?)[*_](?=\W|$)/g, "$1$2");
}

function slugify(text: string): string {
  return (
    text
      .toLowerCase()
      .replace(/[^a-z0-9À-ɏ]+/g, "-")
      .replace(/^-+|-+$/g, "") || "section"
  );
}

function uniqueId(base: string, used: Set<string>): string {
  let id = base;
  for (let n = 2; used.has(id); n++) id = `${base}-${n}`;
  used.add(id);
  return id;
}
