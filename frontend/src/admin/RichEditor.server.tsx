// Server-side stand-in for RichEditor.tsx (swapped in by build.mjs), so the
// editor library never loads inside goja.
import { EditorFallback, type EditorProps } from "./EditorFallback";

export function RichEditor(props: EditorProps) {
  return <EditorFallback {...props} />;
}
