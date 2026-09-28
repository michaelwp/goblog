import Image from "@tiptap/extension-image";
import { Placeholder } from "@tiptap/extensions";
import { Markdown as MarkdownExtension } from "@tiptap/markdown";
import { type Editor, EditorContent, useEditor, useEditorState } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { type FormEvent, useEffect, useRef, useState } from "react";

import { Markdown } from "../components/Markdown";
import { useHydrated } from "../lib/useHydrated";
import { EditorFallback, type EditorProps } from "./EditorFallback";
import { type PMNode, toMarkdown } from "./toMarkdown";
import { uploadImage } from "./upload";

// Links the site will render (see safeHref in components/Markdown.tsx).
const SAFE_LINK = /^(https?:\/\/|mailto:|\/(?!\/)|#)/i;

// The visual (WYSIWYG) article editor. It edits a TipTap document, keeps the
// form's hidden field in sync as Markdown, and offers a raw Markdown view and
// a preview rendered exactly as the site renders it.
export function RichEditor(props: EditorProps) {
  // First render matches the server's plain textarea; swap after hydration.
  return useHydrated() ? <VisualEditor {...props} /> : <EditorFallback {...props} />;
}

// The editor learns about selection changes from a "selectionchange" event
// that fires a moment later; read the browser's selection directly so a
// shortcut pressed immediately after selecting still sees it.
function liveSelection(ed: Editor): { from: number; to: number } {
  const sel = window.getSelection();
  const { view } = ed;
  if (sel?.anchorNode && sel.focusNode && view.dom.contains(sel.anchorNode) && view.dom.contains(sel.focusNode)) {
    try {
      const a = view.posAtDOM(sel.anchorNode, sel.anchorOffset);
      const b = view.posAtDOM(sel.focusNode, sel.focusOffset);
      return { from: Math.min(a, b), to: Math.max(a, b) };
    } catch {
      // fall back to the editor's own state
    }
  }
  return { from: ed.state.selection.from, to: ed.state.selection.to };
}

type Mode = "visual" | "markdown" | "preview";
type LinkTarget = { from: number; to: number; href: string };

function VisualEditor({ name, defaultValue, rows = 16, invalid, describedBy, placeholder = "Start writing…" }: EditorProps) {
  const [markdown, setMarkdown] = useState(defaultValue);
  // The Markdown the visual editor's document corresponds to. When the text
  // differs (it was edited in the Markdown tab), the editor must reload it
  // before it's shown again, or its next update would overwrite those edits.
  const editorMarkdown = useRef(defaultValue);
  const [mode, setMode] = useState<Mode>("visual");
  const [uploading, setUploading] = useState(0);
  const [error, setError] = useState("");
  // The selection to link. Captured when the link bar opens, because focus
  // moving to the bar's input can collapse the editor's selection.
  const [linkRange, setLinkRange] = useState<LinkTarget | null>(null);
  const openLink = () => {
    const ed = editorRef.current;
    if (!ed) return;
    const { from, to } = liveSelection(ed);
    const href = (ed.getAttributes("link").href as string | undefined) ?? "";
    setLinkRange((r) => (r ? null : { from, to, href }));
  };
  const fileInput = useRef<HTMLInputElement>(null);
  const editorRef = useRef<Editor | null>(null);

  async function insertImages(files: File[], at?: number) {
    const images = files.filter((f) => f.type.startsWith("image/"));
    if (!images.length) return;
    setError("");
    for (const file of images) {
      setUploading((n) => n + 1);
      try {
        const src = await uploadImage(file);
        const alt = file.name.replace(/\.[^.]+$/, "").replace(/[-_]+/g, " ").trim();
        const node = { type: "image", attrs: { src, alt } };
        const chain = editorRef.current?.chain().focus();
        if (at !== undefined) chain?.insertContentAt(at, node).run();
        else chain?.insertContent(node).run();
      } catch (e) {
        setError(e instanceof Error ? e.message : "Upload failed.");
      } finally {
        setUploading((n) => n - 1);
      }
    }
  }

  const editor = useEditor({
    immediatelyRender: true,
    extensions: [
      StarterKit.configure({
        heading: { levels: [2, 3] },
        hardBreak: false, // the site's Markdown has no line breaks inside paragraphs
        underline: false, // Markdown can't store underline
        link: {
          openOnClick: false,
          autolink: true,
          linkOnPaste: true,
          defaultProtocol: "https",
          isAllowedUri: (url) => SAFE_LINK.test(url),
        },
      }),
      Image.configure({ inline: false, allowBase64: false }),
      Placeholder.configure({ placeholder }),
      MarkdownExtension,
    ],
    content: defaultValue,
    contentType: "markdown",
    editorProps: {
      attributes: {
        class: "rich-content article-body",
        role: "textbox",
        "aria-multiline": "true",
        "aria-label": "Article body",
        ...(describedBy ? { "aria-describedby": describedBy } : {}),
      },
      handlePaste: (_view, event) => {
        const files = [...(event.clipboardData?.files ?? [])];
        if (!files.some((f) => f.type.startsWith("image/"))) return false;
        event.preventDefault();
        void insertImages(files);
        return true;
      },
      handleDrop: (view, event, _slice, moved) => {
        const files = [...(event.dataTransfer?.files ?? [])];
        if (moved || !files.some((f) => f.type.startsWith("image/"))) return false;
        event.preventDefault();
        const pos = view.posAtCoords({ left: event.clientX, top: event.clientY })?.pos;
        void insertImages(files, pos);
        return true;
      },
      handleKeyDown: (_view, event) => {
        if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
          event.preventDefault();
          openLink();
          return true;
        }
        return false;
      },
    },
    // Only real edits re-serialize, so opening and saving an article
    // without touching it keeps its Markdown byte for byte.
    onUpdate: ({ editor }) => {
      const md = toMarkdown(editor.getJSON() as PMNode);
      editorMarkdown.current = md;
      setMarkdown(md);
    },
  });
  useEffect(() => {
    editorRef.current = editor;
  }, [editor]);

  function switchTo(next: Mode) {
    // Coming back to Visual from any tab: reload Markdown edits made meanwhile.
    if (next === "visual" && editor && markdown !== editorMarkdown.current) {
      editor.commands.setContent(markdown, { contentType: "markdown", emitUpdate: false });
      editorMarkdown.current = markdown;
    }
    setMode(next);
    setLinkRange(null);
  }

  return (
    <div className={`rich-editor${invalid ? " has-error" : ""}`}>
      <div className="rich-bar">
        {mode === "visual" && editor ? (
          <Toolbar editor={editor} uploading={uploading > 0} onImage={() => fileInput.current?.click()} onLink={openLink} />
        ) : (
          <span className="rich-bar-note">{mode === "markdown" ? "Editing Markdown source" : "Preview, as it will appear on the site"}</span>
        )}
        <div className="md-tabs" role="tablist" aria-label="Editor view">
          {(["visual", "markdown", "preview"] as const).map((m) => (
            <button key={m} type="button" role="tab" aria-selected={mode === m} className="md-tab" onClick={() => switchTo(m)}>
              {m === "visual" ? "Visual" : m === "markdown" ? "Markdown" : "Preview"}
            </button>
          ))}
        </div>
      </div>

      {editor && linkRange && mode === "visual" && <LinkBar editor={editor} range={linkRange} onClose={() => setLinkRange(null)} />}
      {editor && mode === "visual" && <ImageCaption editor={editor} />}

      <div hidden={mode !== "visual"} className="rich-surface" style={{ minHeight: `${rows * 1.7}rem` }}>
        <EditorContent editor={editor} />
      </div>
      {mode === "markdown" && (
        <textarea
          className="rich-source"
          rows={rows}
          value={markdown}
          onChange={(e) => setMarkdown(e.target.value)}
          aria-label="Markdown source"
          spellCheck
        />
      )}
      {mode === "preview" && (
        <div className="md-preview article-body">{markdown.trim() ? <Markdown body={markdown} /> : <p className="md-empty">Nothing to preview yet.</p>}</div>
      )}

      <input type="hidden" name={name} value={markdown} />
      <input
        ref={fileInput}
        type="file"
        accept="image/png,image/jpeg,image/gif,image/webp"
        multiple
        hidden
        onChange={(e) => {
          void insertImages([...(e.target.files ?? [])]);
          e.target.value = "";
        }}
      />
      {error && (
        <p className="md-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

function Toolbar({ editor, uploading, onImage, onLink }: { editor: Editor; uploading: boolean; onImage: () => void; onLink: () => void }) {
  const s = useEditorState({
    editor,
    selector: ({ editor: e }) => ({
      block: e.isActive("heading", { level: 2 }) ? "h2" : e.isActive("heading", { level: 3 }) ? "h3" : "p",
      bold: e.isActive("bold"),
      italic: e.isActive("italic"),
      strike: e.isActive("strike"),
      code: e.isActive("code"),
      link: e.isActive("link"),
      bullet: e.isActive("bulletList"),
      ordered: e.isActive("orderedList"),
      quote: e.isActive("blockquote"),
      codeBlock: e.isActive("codeBlock"),
      canUndo: e.can().undo(),
      canRedo: e.can().redo(),
    }),
  });
  const run = (fn: (c: ReturnType<Editor["chain"]>) => ReturnType<Editor["chain"]>) => () => fn(editor.chain().focus()).run();

  return (
    <div className="md-tools" role="toolbar" aria-label="Formatting">
      <div className="md-group">
        <select
          className="rich-block"
          aria-label="Text style"
          value={s.block}
          onChange={(e) => {
            const c = editor.chain().focus();
            if (e.target.value === "p") c.setParagraph().run();
            else c.setHeading({ level: e.target.value === "h2" ? 2 : 3 }).run();
          }}
        >
          <option value="p">Paragraph</option>
          <option value="h2">Heading</option>
          <option value="h3">Subheading</option>
        </select>
      </div>
      <div className="md-group">
        <ToolButton label="B" title="Bold (⌘B)" className="md-bold" active={s.bold} onClick={run((c) => c.toggleBold())} />
        <ToolButton label="I" title="Italic (⌘I)" className="md-italic" active={s.italic} onClick={run((c) => c.toggleItalic())} />
        <ToolButton label="S" title="Strikethrough (⌘⇧S)" className="md-strike" active={s.strike} onClick={run((c) => c.toggleStrike())} />
        <ToolButton label="</>" title="Inline code (⌘E)" active={s.code} onClick={run((c) => c.toggleCode())} />
        <ToolButton label="Link" title="Link (⌘K)" active={s.link} onClick={onLink} />
      </div>
      <div className="md-group">
        <ToolButton label="• List" title="Bulleted list" active={s.bullet} onClick={run((c) => c.toggleBulletList())} />
        <ToolButton label="1. List" title="Numbered list" active={s.ordered} onClick={run((c) => c.toggleOrderedList())} />
        <ToolButton label="“ ”" title="Quote" active={s.quote} onClick={run((c) => c.toggleBlockquote())} />
        <ToolButton label="{ }" title="Code block" active={s.codeBlock} onClick={run((c) => c.toggleCodeBlock())} />
        <ToolButton label="—" title="Divider" onClick={run((c) => c.setHorizontalRule())} />
      </div>
      <div className="md-group">
        <ToolButton label={uploading ? "Uploading…" : "Image"} title="Upload image" onClick={onImage} disabled={uploading} />
      </div>
      <div className="md-group">
        <ToolButton label="Undo" title="Undo (⌘Z)" onClick={run((c) => c.undo())} disabled={!s.canUndo} />
        <ToolButton label="Redo" title="Redo (⌘⇧Z)" onClick={run((c) => c.redo())} disabled={!s.canRedo} />
      </div>
    </div>
  );
}

type ToolButtonProps = { label: string; title: string; active?: boolean; onClick: () => void; disabled?: boolean; className?: string };

// A toolbar button. Defined at module level so React keeps the same button
// between renders (a component created inside Toolbar would be remounted).
function ToolButton(p: ToolButtonProps) {
  return (
    <button
      type="button"
      className={`md-tool ${p.className ?? ""}`}
      title={p.title}
      aria-label={p.title}
      aria-pressed={p.active === undefined ? undefined : p.active}
      onMouseDown={(e) => e.preventDefault()} // keep the editor's selection
      onClick={p.onClick}
      disabled={p.disabled}
    >
      {p.label}
    </button>
  );
}

// Inline bar for adding, editing or removing the link on the selection.
function LinkBar({ editor, range, onClose }: { editor: Editor; range: LinkTarget; onClose: () => void }) {
  const current = range.href;
  const restore = () => editor.chain().focus().setTextSelection({ from: range.from, to: range.to });
  const [url, setUrl] = useState(current);
  const [problem, setProblem] = useState("");

  function apply(e: FormEvent) {
    e.preventDefault();
    let href = url.trim();
    if (href && !/^[a-z]+:/i.test(href) && !href.startsWith("/") && !href.startsWith("#")) href = `https://${href}`;
    if (href && !SAFE_LINK.test(href)) {
      setProblem("Links must start with https://, http://, mailto:, / or #.");
      return;
    }
    const chain = restore().extendMarkRange("link");
    if (!href) chain.unsetLink().run();
    else if (range.from === range.to && !current) {
      // Nothing selected: insert the address itself as the link text.
      chain.insertContent({ type: "text", text: href, marks: [{ type: "link", attrs: { href } }] }).run();
    } else chain.setLink({ href }).setTextSelection(range.to).run(); // caret after the link, so typing can't replace it
    onClose();
  }

  // Not a <form>: it sits inside the article form, and forms can't nest.
  return (
    <div className="rich-subbar" role="group" aria-label="Link">
      <input
        autoFocus
        type="text"
        value={url}
        placeholder="https://example.com"
        aria-label="Link address"
        onChange={(e) => {
          setUrl(e.target.value);
          setProblem("");
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") apply(e);
          if (e.key === "Escape") onClose();
        }}
      />
      <button type="button" className="btn btn-primary btn-sm" onClick={apply}>
        {current ? "Update" : "Add link"}
      </button>
      {current && (
        <button
          type="button"
          className="btn btn-quiet btn-sm"
          onClick={() => {
            restore().extendMarkRange("link").unsetLink().run();
            onClose();
          }}
        >
          Remove
        </button>
      )}
      <button type="button" className="btn btn-quiet btn-sm" onClick={onClose}>
        Cancel
      </button>
      {problem && <span className="field-error">{problem}</span>}
    </div>
  );
}

// When an image is selected, edit its caption (also its alt text).
function ImageCaption({ editor }: { editor: Editor }) {
  const image = useEditorState({
    editor,
    selector: ({ editor: e }) => (e.isActive("image") ? { alt: String(e.getAttributes("image").alt ?? "") } : null),
  });
  if (!image) return null;
  return (
    <div className="rich-subbar" role="group" aria-label="Image caption">
      <label className="rich-subbar-label" htmlFor="rich-caption">
        Caption
      </label>
      <input
        id="rich-caption"
        type="text"
        value={image.alt}
        placeholder="Describe the image"
        onFocus={(e) => e.target.select()}
        onChange={(e) => editor.chain().updateAttributes("image", { alt: e.target.value }).run()}
      />
      <button type="button" className="btn btn-quiet btn-sm" onClick={() => editor.chain().focus().deleteSelection().run()}>
        Remove image
      </button>
    </div>
  );
}
