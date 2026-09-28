// Tests for the article Markdown pipeline: the site's renderer
// (components/Markdown.tsx) and the visual editor's serializer
// (admin/toMarkdown.ts). Run with `npm test`.
import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { renderToStaticMarkup } from "react-dom/server";

import { type PMNode, toMarkdown } from "../src/admin/toMarkdown";
import { Markdown } from "../src/components/Markdown";
import { parseBody } from "../src/lib/article";

const html = (md: string) => renderToStaticMarkup(<Markdown body={md} />);
const text = (value: string, marks: PMNode["marks"] = []): PMNode => ({ type: "text", text: value, marks });
const para = (...content: PMNode[]): PMNode => ({ type: "doc", content: [{ type: "paragraph", content }] });
const link = (href: string) => ({ type: "link", attrs: { href } });

describe("links and images with parentheses", () => {
  const wiki = "https://en.wikipedia.org/wiki/Go_(programming_language)";

  test("hand-written Markdown with balanced parentheses", () => {
    assert.match(html(`See [Go](${wiki}) now.`), /<a href="https:\/\/en\.wikipedia\.org\/wiki\/Go_\(programming_language\)"[^>]*>Go<\/a> now\./);
  });

  test("the editor encodes them, and the link survives the round trip", () => {
    const md = toMarkdown(para(text("Go", [link(wiki)])));
    assert.equal(md, "[Go](https://en.wikipedia.org/wiki/Go_%28programming_language%29)");
    const out = html(md);
    assert.match(out, /href="https:\/\/en\.wikipedia\.org\/wiki\/Go_%28programming_language%29"/);
    assert.doesNotMatch(out, /\)<\/p>/, "no stray ) after the link");
  });

  test("image addresses too", () => {
    const md = toMarkdown({ type: "doc", content: [{ type: "image", attrs: { src: "https://example.com/a (1).png", alt: "Pic" } }] });
    assert.equal(md, "![Pic](https://example.com/a%20%281%29.png)");
    assert.match(html(md), /<img src="https:\/\/example\.com\/a%20%281%29\.png" alt="Pic"/);
  });
});

describe("escaped markers inside emphasis", () => {
  test("a literal * inside italic text", () => {
    assert.equal(html("*5\\*3*"), "<p><em>5*3</em></p>");
    const md = toMarkdown(para(text("5*3", [{ type: "italic" }])));
    assert.equal(html(md), "<p><em>5*3</em></p>");
  });

  test("literal markers inside bold and strikethrough", () => {
    assert.equal(html("**a\\*\\*b**"), "<p><strong>a**b</strong></p>");
    assert.equal(html("~~x\\~~y~~"), "<p><del>x~~y</del></p>");
  });
});

describe("text that only looks like Markdown stays plain", () => {
  for (const value of ["2026. A great year", "- not a list", "+ plus", "## not a heading", "> not a quote", "---", "***", "![not](an image)", "```not code", "a * b * c", "_under_ and __double__", "back\\slash", "[x](javascript:alert(1))"]) {
    test(JSON.stringify(value), () => {
      const md = toMarkdown(para(text(value)));
      const blocks = parseBody(md);
      assert.equal(blocks.length, 1);
      assert.equal(blocks[0].kind, "p");
      assert.equal(html(md), renderToStaticMarkup(<p>{value}</p>));
    });
  }
});

describe("mark combinations never produce an ambiguous ***", () => {
  const cases: [string, PMNode, string][] = [
    ["italic then bold", para(text("a", [{ type: "italic" }]), text("b", [{ type: "bold" }])), "<p><em>a</em><strong>b</strong></p>"],
    ["bold then italic", para(text("a", [{ type: "bold" }]), text("b", [{ type: "italic" }])), "<p><strong>a</strong><em>b</em></p>"],
    [
      "bold inside italic",
      para(text("x ", [{ type: "italic" }]), text("y", [{ type: "italic" }, { type: "bold" }]), text(" z", [{ type: "italic" }])),
      "<p><em>x <strong>y</strong> z</em></p>",
    ],
    [
      "one bold span with an italic word",
      para(text("bold ", [{ type: "bold" }]), text("nested", [{ type: "bold" }, { type: "italic" }]), text(" text", [{ type: "bold" }])),
      "<p><strong>bold <em>nested</em> text</strong></p>",
    ],
  ];
  for (const [name, doc, want] of cases) {
    test(name, () => assert.equal(html(toMarkdown(doc)), want));
  }
});

describe("unsafe addresses render as text", () => {
  test("javascript: links and images", () => {
    const out = html("[x](javascript:alert(1)) ![y](javascript:alert(1))");
    assert.doesNotMatch(out, /href="javascript|src="javascript/);
  });
});
