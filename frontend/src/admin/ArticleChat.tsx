// The new article page's chat box: describe an article and the AI writes it
// into the form (title, slug, summary, tags, category, text and a cover);
// later messages revise that draft. The chat itself isn't saved: only the
// article fields are, like anything typed into them.
import { type FormEvent, type KeyboardEvent, useEffect, useRef, useState } from "react";

type Message = { from: "you" | "ai"; text: string; error?: boolean; failed?: boolean };

export function ArticleChat(props: {
  busy: string;
  generate: (prompt: string, history: string[]) => Promise<{ ok: boolean; text: string }>;
  languageName: string; // the article's language, which the AI writes in
}) {
  const { busy, generate, languageName } = props;
  const [messages, setMessages] = useState<Message[]>([]);
  const [prompt, setPrompt] = useState("");
  const [working, setWorking] = useState(false);
  const log = useRef<HTMLDivElement>(null);

  useEffect(() => {
    log.current?.scrollTo({ top: log.current.scrollHeight, behavior: "smooth" });
  }, [messages, working]);

  async function send(e?: FormEvent) {
    e?.preventDefault();
    const text = prompt.trim();
    if (!text || working || busy) return;
    // Earlier requests that worked: a failed one is retried, not repeated.
    const history = messages.filter((m) => m.from === "you" && !m.failed).map((m) => m.text);
    setMessages((m) => [...m, { from: "you", text }]);
    setPrompt("");
    setWorking(true);
    const result = await generate(text, history);
    setWorking(false);
    setMessages((m) => {
      const next = [...m];
      if (!result.ok) next[next.length - 1] = { ...next[next.length - 1], failed: true };
      return [...next, { from: "ai", text: result.text, error: !result.ok }];
    });
    if (!result.ok) setPrompt(text); // keep it to try again
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    // Enter sends; Shift+Enter makes a new line.
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void send();
    }
  }

  const started = messages.length > 0;
  return (
    <section className="article-chat" aria-labelledby="article-chat-title">
      <h2 id="article-chat-title" className="article-chat-title">
        ✨ Write with AI <span className="article-chat-lang">in {languageName}</span>
      </h2>
      <p className="admin-muted article-chat-intro">
        {started
          ? "Ask for changes (e.g. “make it shorter”, “add a section on costs”). Your own edits in the fields are kept and sent along."
          : `Describe the article you want, in any language: the AI writes it in ${languageName} (the Language field), filling in the title, slug, summary, tags, category and text, and makes a cover. Everything stays editable, and nothing is published until you do.`}
      </p>

      {started && (
        <div className="article-chat-log" ref={log} aria-live="polite">
          {messages.map((m, i) => (
            <p key={i} className={`chat-msg chat-${m.from}${m.error ? " is-error" : ""}`}>
              <span className="chat-who">{m.from === "you" ? "You" : "AI"}</span>
              {m.text}
            </p>
          ))}
          {working && (
            <p className="chat-msg chat-ai is-working">
              <span className="chat-who">AI</span>
              {busy === "cover" ? "Making a cover image…" : "Writing… this can take a minute."}
            </p>
          )}
        </div>
      )}

      {/* Not a <form>: it sits inside the article form. */}
      <div className="article-chat-input">
        <textarea
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          onKeyDown={onKeyDown}
          rows={started ? 2 : 3}
          maxLength={4000}
          placeholder={started ? "Ask for a change…" : "Describe the article you want…"}
          aria-label="Message to the AI"
          disabled={working}
        />
        <button type="button" className="btn btn-primary" onClick={() => void send()} disabled={working || !!busy || !prompt.trim()}>
          {working ? "Working…" : started ? "Send" : "Write article"}
        </button>
      </div>
    </section>
  );
}
