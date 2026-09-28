import { type RefObject, useEffect, useRef, useState } from "react";

export const AUTOSAVE_INTERVAL = 60_000;
const CHECK_INTERVAL = 2_000;

export type AutosaveState =
  | { kind: "idle" } // nothing changed since load or the last save
  | { kind: "dirty" } // unsaved changes, waiting for the next autosave
  | { kind: "saving" }
  | { kind: "saved"; at: string } // ISO time of the last successful save
  | { kind: "paused"; message: string }; // the server refused (e.g. missing title)

// The form's fields as the server will receive them, minus the submit action.
function snapshot(form: HTMLFormElement): string {
  const data = new URLSearchParams();
  for (const [key, value] of new FormData(form)) {
    if (key !== "action" && typeof value === "string") data.append(key, value);
  }
  return data.toString();
}

// Saves the form to `url` about once a minute while it has unsaved changes,
// and warns before leaving the page with changes that haven't been saved.
// `onCreated` receives the edit URL when autosave created a new article.
export function useAutosave(form: RefObject<HTMLFormElement | null>, url: string, onCreated: (editUrl: string) => void) {
  const [state, setState] = useState<AutosaveState>({ kind: "idle" });
  const saved = useRef<string | null>(null);
  const lastAttempt = useRef(Date.now());
  const busy = useRef(false);
  const inflight = useRef<Promise<void> | null>(null);
  const createdUrl = useRef<string | null>(null);
  const submitting = useRef(false);
  const urlRef = useRef(url);
  urlRef.current = url;
  const onCreatedRef = useRef(onCreated);
  onCreatedRef.current = onCreated;
  const saveNowRef = useRef<() => Promise<boolean>>(async () => true);

  useEffect(() => {
    const el = form.current;
    if (!el) return;
    // Wait a tick so the editor has filled in the form before the baseline.
    const start = setTimeout(() => (saved.current = snapshot(el)), 500);

    function save(current: string) {
      inflight.current = doSave(current).finally(() => (inflight.current = null));
    }

    async function doSave(current: string) {
      busy.current = true;
      lastAttempt.current = Date.now();
      setState({ kind: "saving" });
      try {
        const res = await fetch(urlRef.current, {
          method: "POST",
          body: current,
          credentials: "same-origin",
          headers: { "Content-Type": "application/x-www-form-urlencoded", Accept: "application/json" },
        });
        if (res.redirected || !res.headers.get("content-type")?.includes("json")) {
          setState({ kind: "paused", message: "Autosave paused: you've been signed out. Sign in again in another tab." });
          return;
        }
        const data = (await res.json()) as { savedAt?: string; editUrl?: string; error?: string };
        if (!res.ok || !data.savedAt) {
          setState({ kind: "paused", message: data.error ?? "Autosave failed; it will retry." });
          return;
        }
        saved.current = current;
        setState({ kind: "saved", at: data.savedAt });
        if (data.editUrl) {
          createdUrl.current = data.editUrl;
          onCreatedRef.current(data.editUrl);
        }
      } catch {
        setState({ kind: "paused", message: "Autosave failed (offline?); it will retry." });
      } finally {
        busy.current = false;
      }
    }

    // Saves right away (e.g. before switching to another translation).
    // Resolves true when everything is saved, false if the save failed.
    saveNowRef.current = async () => {
      if (inflight.current) await inflight.current;
      if (saved.current === null) return true;
      const current = snapshot(el);
      if (current === saved.current) return true;
      save(current);
      await inflight.current;
      return saved.current === current;
    };

    const timer = setInterval(() => {
      if (busy.current || submitting.current || saved.current === null) return;
      const current = snapshot(el);
      if (current === saved.current) return;
      setState((s) => (s.kind === "paused" || s.kind === "dirty" ? s : { kind: "dirty" }));
      if (Date.now() - lastAttempt.current >= AUTOSAVE_INTERVAL) save(current);
    }, CHECK_INTERVAL);

    // A submit must not race an autosave that's still in flight: if autosave
    // is creating the article, the submit would try to create it again, and
    // an autosave landing after "Publish changes" would leave a stale draft.
    // So wait for it, point the form at the article it created (if any), and
    // submit again with the same button.
    let resubmitting = false;
    const onSubmit = (e: SubmitEvent) => {
      submitting.current = true; // no new autosaves from here on
      if (resubmitting || !inflight.current) return;
      e.preventDefault();
      const submitter = e.submitter as HTMLElement | null;
      void inflight.current.then(() => {
        if (createdUrl.current) el.action = createdUrl.current;
        resubmitting = true;
        el.requestSubmit(submitter instanceof HTMLButtonElement ? submitter : undefined);
      });
    };
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (submitting.current || saved.current === null || snapshot(el) === saved.current) return;
      e.preventDefault();
      e.returnValue = ""; // older browsers need this to show the prompt
    };
    el.addEventListener("submit", onSubmit);
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => {
      clearTimeout(start);
      clearInterval(timer);
      el.removeEventListener("submit", onSubmit);
      window.removeEventListener("beforeunload", onBeforeUnload);
    };
  }, [form]);

  return {
    state,
    saveNow: () => saveNowRef.current(),
    // Leave without the "unsaved changes" prompt (the user already chose to).
    allowLeave: () => {
      submitting.current = true;
    },
  };
}
