import { type KeyboardEvent, useId, useState } from "react";

import { useHydrated } from "../lib/useHydrated";

const MAX_TAGS = 10;

// Mirrors posts.NormalizeTag in the Go backend: " Node JS " → "node-js".
export function normalizeTag(s: string): string {
  return s
    .trim()
    .replace(/^#/, "")
    .trim()
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .join("-")
    .replace(/-{2,}/g, "-")
    .replace(/^-+|-+$/g, "");
}

const parse = (value: string) => [...new Set(value.split(",").map(normalizeTag).filter(Boolean))];

// Tags for an article. The server renders (and the first client render
// matches) a plain comma-separated input; after hydration it becomes chips
// with suggestions from tags already in use. Either way the form submits
// "tags" as a comma-separated list.
export function TagInput({ defaultValue, known, invalid }: { defaultValue: string; known: string[]; invalid?: boolean }) {
  const hydrated = useHydrated();
  const [tags, setTags] = useState(() => parse(defaultValue));
  const [text, setText] = useState("");
  const listId = useId();

  if (!hydrated) {
    return <input type="text" name="tags" defaultValue={defaultValue} placeholder="go, security, web" aria-invalid={invalid || undefined} />;
  }

  const add = (raw: string) => {
    const next = parse(raw).filter((t) => !tags.includes(t));
    if (next.length) setTags((cur) => [...cur, ...next].slice(0, MAX_TAGS));
    setText("");
  };
  const remove = (tag: string) => setTags((cur) => cur.filter((t) => t !== tag));

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault(); // Enter must not submit the article form here
      add(text);
    } else if (e.key === "Backspace" && text === "" && tags.length) {
      remove(tags[tags.length - 1]);
    }
  }

  const query = normalizeTag(text);
  const suggestions = known.filter((t) => !tags.includes(t) && (!query || t.startsWith(query))).slice(0, 8);
  const full = tags.length >= MAX_TAGS;

  return (
    <div className={`tag-input${invalid ? " has-error" : ""}`}>
      <input type="hidden" name="tags" value={tags.join(", ")} />
      <div className="tag-input-box">
        {tags.map((tag) => (
          <span key={tag} className="tag-chip">
            #{tag}
            <button type="button" onClick={() => remove(tag)} aria-label={`Remove tag ${tag}`}>
              ×
            </button>
          </span>
        ))}
        <input
          type="text"
          value={text}
          disabled={full}
          placeholder={full ? `Up to ${MAX_TAGS} tags` : tags.length ? "Add a tag" : "Type a tag and press Enter"}
          aria-label="Add a tag"
          aria-describedby={listId}
          onChange={(e) => (e.target.value.includes(",") ? add(e.target.value) : setText(e.target.value))}
          onKeyDown={onKeyDown}
          onBlur={() => text && add(text)}
        />
      </div>
      {suggestions.length > 0 && !full && (
        <div className="tag-suggestions" id={listId}>
          <span>{query ? "Matching tags:" : "Used before:"}</span>
          {suggestions.map((t) => (
            <button key={t} type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => add(t)}>
              #{t}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
