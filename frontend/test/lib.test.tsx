// Unit tests for the frontend's pure helpers. Run with `npm test`.
import assert from "node:assert/strict";
import { describe, test } from "node:test";

import { passwordStrength } from "../src/admin/PasswordField";
import { normalizeTag } from "../src/admin/TagInput";
import { categoryHref, tagHref } from "../src/components/Taxonomy";
import { parseBody, plainText, readingMinutes, sections } from "../src/lib/article";
import { countriesFor, countryName, flagEmoji } from "../src/lib/countries";
import { formatDate } from "../src/lib/format";
import { format, getDictionary, hasLocale, locales } from "../src/lib/i18n";
import { queryString } from "../src/lib/query";

describe("article parser", () => {
  test("splits a body into blocks", () => {
    const blocks = parseBody(
      [
        "Intro line one",
        "line two.",
        "",
        "## Section",
        "### Sub",
        "> quote a",
        "> quote b",
        "",
        "- one",
        "- two",
        "  wrapped",
        "",
        "1. first",
        "2) second",
        "",
        "```go",
        "x := 1",
        "",
        "y := 2",
        "```",
        "---",
        '![Alt](/media/abc.png "Caption")',
      ].join("\n"),
    );
    assert.deepEqual(
      blocks.map((b) => b.kind),
      ["p", "h2", "h3", "quote", "ul", "ol", "code", "hr", "img"],
    );
    assert.deepEqual(blocks[0], { kind: "p", text: "Intro line one line two." });
    assert.deepEqual(blocks[3], { kind: "quote", text: "quote a quote b" });
    assert.deepEqual(blocks[4], { kind: "ul", items: ["one", "two wrapped"] });
    assert.deepEqual(blocks[5], { kind: "ol", items: ["first", "second"] });
    assert.deepEqual(blocks[6], { kind: "code", code: "x := 1\n\ny := 2", lang: "go" });
    assert.deepEqual(blocks[8], { kind: "img", src: "/media/abc.png", alt: "Alt", title: "Caption" });
  });

  test("gives headings unique ids and lists ## headings as sections", () => {
    const blocks = parseBody("## Setup\n\n## Setup\n\n### Detail\n\n## Kébab *Case*");
    assert.deepEqual(
      sections(blocks),
      [
        { id: "setup", text: "Setup" },
        { id: "setup-2", text: "Setup" },
        { id: "kébab-case", text: "Kébab Case" },
      ],
    );
  });

  test("handles Windows line endings and empty input", () => {
    assert.deepEqual(parseBody("a\r\n\r\nb").map((b) => b.kind), ["p", "p"]);
    assert.deepEqual(parseBody(""), []);
  });

  test("plainText strips inline markup", () => {
    assert.equal(plainText("**Bold** and *it* with [a link](https://x.dev) and `code`"), "Bold and it with a link and code");
  });

  test("readingMinutes: at least one minute, ~200 words per minute", () => {
    assert.equal(readingMinutes("short"), 1);
    assert.equal(readingMinutes(Array(1000).fill("word").join(" ")), 5);
  });
});

describe("formatting and i18n", () => {
  test("dates in each language (without Intl, as on the server)", () => {
    assert.equal(formatDate("2026-09-28T09:00:00Z", "en"), "September 28, 2026");
    assert.equal(formatDate("2026-09-28T09:00:00Z", "id"), "28 September 2026");
    assert.equal(formatDate("2026-01-01T00:00:00Z", "id"), "1 Januari 2026");
  });

  test("format fills placeholders and keeps unknown ones", () => {
    assert.equal(format("{n} results for “{q}”", { n: 3, q: "go" }), "3 results for “go”");
    assert.equal(format("Hi {name}, {missing}", { name: "Ada" }), "Hi Ada, {missing}");
  });

  test("every dictionary has the same keys", () => {
    const [first, ...rest] = locales.map((l) => Object.keys(getDictionary(l)).sort());
    for (const keys of rest) assert.deepEqual(keys, first);
    assert.ok(hasLocale("id") && !hasLocale("fr"));
  });

  test("queryString skips empty values and encodes", () => {
    assert.equal(queryString({ q: "a b&c", tag: "", category: "web" }), "q=a%20b%26c&category=web");
    assert.equal(queryString({ show: "" }), "");
  });

  test("category and tag links are URL-encoded", () => {
    assert.equal(categoryHref("en", "web-dev"), "/en/categories/web-dev");
    assert.equal(tagHref("id", "c#"), "/id/tags/c%23");
    assert.equal(tagHref("en", "c++"), "/en/tags/c%2B%2B");
  });
});

describe("countries", () => {
  test("flag emoji from a country code", () => {
    assert.equal(flagEmoji("ID"), "🇮🇩");
    assert.equal(flagEmoji("US"), "🇺🇸");
    assert.equal(flagEmoji("id"), "");
    assert.equal(flagEmoji("XYZ"), "");
  });

  test("names in both languages, sorted by name for pickers", () => {
    assert.equal(countryName("JP", "en"), "Japan");
    assert.equal(countryName("JP", "id"), "Jepang");
    assert.equal(countryName("??", "en"), "??");
    const en = countriesFor("en");
    assert.equal(en.length, 250);
    assert.deepEqual(
      en.map((c) => c.name),
      [...en.map((c) => c.name)].sort((a, b) => a.localeCompare(b)),
    );
  });
});

describe("tags", () => {
  // Same cases as TestNormalizeTag in backend/internal/posts, so both sides agree.
  const cases: [string, string][] = [
    [" Go ", "go"],
    ["#Security", "security"],
    ["Node   JS", "node-js"],
    ["--a--b--", "a-b"],
    ["C#", "c#"],
    ["Jaringan  Komputer", "jaringan-komputer"],
    ["   ", ""],
  ];
  for (const [input, want] of cases) {
    test(`normalizeTag(${JSON.stringify(input)})`, () => assert.equal(normalizeTag(input), want));
  }
});

describe("password strength", () => {
  const level = (p: string) => passwordStrength(p).label;
  test("levels follow the rules and length", () => {
    assert.equal(level(""), "");
    assert.equal(level("abc"), "Weak");
    assert.equal(level("Abcdefghijk1"), "Fair"); // missing a symbol
    assert.equal(level("Abcdefghij1!"), "Good"); // all rules, 12 characters
    assert.equal(level("Tr0ub4dor&3-Horse!"), "Strong"); // 16+ characters
  });

  test("common words and repeats cost a level", () => {
    assert.equal(level("Password123!"), "Fair");
    assert.equal(level("Baaa1!cdefghij"), "Fair"); // "aaa"
    assert.equal(level("Aaa1!bcdefghij"), "Good"); // "Aaa" isn't a repeat: the check is case-sensitive
  });

  test("reports which rules are met", () => {
    assert.deepEqual(passwordStrength("abcdefghijkl").met, [true, false, true, false, false]);
  });
});
