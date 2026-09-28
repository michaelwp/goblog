// What the article editor looks like before JavaScript runs: a Markdown
// textarea that submits with the form. The server renders this (see
// RichEditor.server.tsx) and the browser's first render matches it, so
// hydration is clean; RichEditor then swaps in the visual editor.
export type EditorProps = {
  name: string;
  defaultValue: string;
  rows?: number;
  invalid?: boolean;
  describedBy?: string;
  placeholder?: string;
};

export function EditorFallback({ name, defaultValue, rows = 16, invalid, describedBy }: EditorProps) {
  return (
    <div className={invalid ? "rich-editor has-error" : "rich-editor"}>
      <textarea
        name={name}
        className="rich-source"
        rows={rows}
        defaultValue={defaultValue}
        aria-invalid={invalid || undefined}
        aria-describedby={describedBy}
      />
    </div>
  );
}
