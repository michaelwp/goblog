// Serializes the visual editor's document (TipTap/ProseMirror JSON) to the
// Markdown dialect in lib/article.ts. It is written for that parser rather
// than generic CommonMark, so whatever the visual editor shows is exactly
// what the site renders, and text that merely looks like Markdown
// ("2026. A year", "- note", "---") is escaped instead of becoming a list,
// divider or heading.

export type PMNode = {
  type: string;
  attrs?: Record<string, unknown>;
  content?: PMNode[];
  text?: string;
  marks?: { type: string; attrs?: Record<string, unknown> }[];
};

export function toMarkdown(doc: PMNode): string {
  return (doc.content ?? [])
    .map(block)
    .filter((s) => s !== "")
    .join("\n\n");
}

function block(node: PMNode): string {
  switch (node.type) {
    case "paragraph":
      return escapeLineStart(inline(node.content));
    case "heading": {
      const level = Math.min(3, Math.max(2, Number(node.attrs?.level) || 2));
      const text = inline(node.content);
      return text ? `${"#".repeat(level)} ${text}` : "";
    }
    case "bulletList":
      return listItems(node).map((t) => `- ${t}`).join("\n");
    case "orderedList": {
      const start = Number(node.attrs?.start) || 1;
      return listItems(node).map((t, i) => `${start + i}. ${t}`).join("\n");
    }
    case "blockquote": {
      const text = (node.content ?? []).map((c) => (c.type === "paragraph" ? inline(c.content) : textOf(c))).filter(Boolean).join(" ");
      return text ? `> ${text}` : "";
    }
    case "codeBlock": {
      const lang = typeof node.attrs?.language === "string" ? node.attrs.language : "";
      return "```" + lang + "\n" + textOf(node) + "\n```";
    }
    case "horizontalRule":
      return "---";
    case "image":
      return image(node);
    default:
      // Unknown blocks keep their text rather than disappearing.
      return escapeLineStart(escapeText(textOf(node)));
  }
}

// The site's lists aren't nested, so nested items are flattened into the
// parent list and multi-paragraph items are joined; no text is lost.
function listItems(list: PMNode): string[] {
  const items: string[] = [];
  for (const item of list.content ?? []) {
    const parts: string[] = [];
    for (const child of item.content ?? []) {
      if (child.type === "bulletList" || child.type === "orderedList") {
        if (parts.length) items.push(parts.join(" "));
        parts.length = 0;
        items.push(...listItems(child));
      } else {
        parts.push(child.type === "paragraph" ? inline(child.content) : textOf(child));
      }
    }
    if (parts.length) items.push(parts.join(" "));
  }
  return items.filter(Boolean);
}

function image(node: PMNode): string {
  const src = String(node.attrs?.src ?? "");
  if (!src) return "";
  const alt = String(node.attrs?.alt ?? "").replace(/[[\]\\]/g, "\\$&");
  const title = String(node.attrs?.title ?? "").replace(/"/g, "'");
  return `![${alt}](${encodeDestination(src)}${title ? ` "${title}"` : ""})`;
}

function textOf(node: PMNode): string {
  if (node.type === "text") return node.text ?? "";
  return (node.content ?? []).map(textOf).join("");
}

// Inline content: text nodes with marks. Marks are kept on a stack and only
// opened or closed where they change, so "bold with an *italic* word" stays
// one bold span. Whitespace is kept outside markers (the parser needs that
// for *), and italic uses _ next to a ** so the output never has "***".
const ORDER = ["link", "bold", "italic", "strike"] as const;
type MarkName = (typeof ORDER)[number];
type Open = { mark: MarkName; close: string; href?: string };

function inline(nodes: PMNode[] | undefined): string {
  let out = "";
  const stack: Open[] = [];

  const closeFrom = (k: number) => {
    const trail = /\s*$/.exec(out)![0];
    out = out.slice(0, out.length - trail.length);
    while (stack.length > k) out += stack.pop()!.close;
    out += trail;
  };

  for (const node of nodes ?? []) {
    if (node.type !== "text") {
      closeFrom(0);
      out += node.type === "image" ? image(node) : escapeText(textOf(node));
      continue;
    }
    const text = node.text ?? "";
    const marks = new Map((node.marks ?? []).map((m) => [m.type, m.attrs] as const));
    const wanted = ORDER.filter((m) => marks.has(m));
    const href = typeof marks.get("link")?.href === "string" ? String(marks.get("link")!.href) : undefined;

    // Close everything from the first open mark this node doesn't continue.
    const keep = stack.findIndex((o) => !wanted.includes(o.mark) || (o.mark === "link" && o.href !== href));
    if (keep >= 0) closeFrom(keep);

    const lead = /^\s*/.exec(text)![0];
    out += lead;
    for (const mark of wanted) {
      if (stack.some((o) => o.mark === mark)) continue;
      if (mark === "link") {
        if (!href) continue;
        out += "[";
        stack.push({ mark, close: `](${encodeDestination(href)})`, href });
      } else {
        // Pick markers that can't merge into an ambiguous "***" with a
        // neighbouring or enclosing marker.
        const starOpen = (d: string) => stack.some((o) => o.close === d);
        const delim =
          mark === "strike"
            ? "~~"
            : mark === "bold"
              ? starOpen("*") || out.endsWith("*")
                ? "__"
                : "**"
              : starOpen("**") || out.endsWith("*")
                ? "_"
                : "*";
        out += delim;
        stack.push({ mark, close: delim });
      }
    }
    const body = text.slice(lead.length);
    out += marks.has("code") ? "`" + body.replace(/`/g, "'") + "`" : escapeText(body);
  }
  closeFrom(0);
  return out.trim();
}

// Percent-encodes what would end or split a link/image address in Markdown.
// (encodeURIComponent leaves "(" and ")" alone, so it can't be used here.)
// Servers treat %28/%29 like ( and ), so the link still works.
function encodeDestination(url: string): string {
  return url.replace(/[\s()<>]/g, (c) => "%" + c.charCodeAt(0).toString(16).toUpperCase().padStart(2, "0"));
}

// Characters that start inline syntax. The renderer turns "\x" back into "x".
function escapeText(s: string): string {
  return s.replace(/[\\`*_~[\]]/g, "\\$&");
}

// Text at the start of a paragraph that the block parser would otherwise read
// as a list, heading, quote or divider.
function escapeLineStart(s: string): string {
  return s
    .replace(/^(\d+)([.)])(\s)/, "$1\\$2$3")
    .replace(/^([-+])(\s)/, "\\$1$2")
    .replace(/^(#{1,6})(\s|$)/, "\\$1$2")
    .replace(/^>/, "\\>")
    .replace(/^(-{3,})\s*$/, "\\$1")
    .replace(/^!\[/, "!\\[");
}
