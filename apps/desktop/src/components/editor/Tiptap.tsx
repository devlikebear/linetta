import { useEditor, EditorContent, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { useEffect, useMemo, useRef } from "react";
import "./Tiptap.css";

interface Props {
  /** Tiptap JSON doc — controls the editor's initial state. The component is
   *  uncontrolled afterwards; consumers respond to onUpdate. */
  initialDoc: object;
  /** Called whenever the document changes (every keystroke). Debounce upstream. */
  onChange: (doc: object) => void;
  /** Called with the character count after each change (whitespace-included). */
  onCharCount?: (count: number) => void;
  /** Typewriter scroll: keeps the active line near the viewport center. */
  typewriter?: boolean;
  /** Manual-save hotkey handler — receives the current doc; consumer issues the RPC. */
  onManualSave?: (doc: object) => void;
}

export function TiptapEditor({ initialDoc, onChange, onCharCount, typewriter, onManualSave }: Props) {
  // Stable reference for the initial doc to avoid resetting on every render.
  const initialKey = useMemo(() => JSON.stringify(initialDoc).length, [initialDoc]);

  const editor = useEditor(
    {
      extensions: [StarterKit.configure({})],
      content: initialDoc,
      autofocus: "end",
      onUpdate: ({ editor }) => {
        const doc = editor.getJSON();
        onChange(doc);
        if (onCharCount) onCharCount(countChars(doc));
      },
    },
    // Re-create the editor only when the initial doc actually changes id/length —
    // avoids cursor jumps from upstream re-renders.
    [initialKey],
  );

  useEffect(() => {
    if (!editor) return;
    if (onCharCount) onCharCount(countChars(editor.getJSON()));
  }, [editor, onCharCount]);

  // Cmd+S → manual save (intercept before browser save dialog).
  useEffect(() => {
    if (!editor) return;
    const handler = (e: KeyboardEvent) => {
      const isMac = navigator.platform.toLowerCase().includes("mac");
      const isSave = (isMac ? e.metaKey : e.ctrlKey) && e.key.toLowerCase() === "s";
      if (!isSave) return;
      e.preventDefault();
      if (onManualSave) onManualSave(editor.getJSON());
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [editor, onManualSave]);

  return (
    <div className={`tiptap-wrap${typewriter ? " typewriter" : ""}`}>
      <EditorContent editor={editor} className="tiptap-editor" />
      <TypewriterScroll editor={editor} enabled={!!typewriter} />
    </div>
  );
}

/** Scrolls the editor so the current cursor line stays near viewport center. */
function TypewriterScroll({ editor, enabled }: { editor: Editor | null; enabled: boolean }) {
  const lastTop = useRef<number>(-1);
  useEffect(() => {
    if (!editor || !enabled) return;
    const handler = () => {
      const view = editor.view;
      const pos = view.state.selection.head;
      const coords = view.coordsAtPos(pos);
      const target = window.innerHeight / 2;
      const delta = coords.top - target;
      if (Math.abs(delta) < 4) return;
      if (delta === lastTop.current) return;
      lastTop.current = delta;
      window.scrollBy({ top: delta, behavior: "smooth" });
    };
    editor.on("selectionUpdate", handler);
    editor.on("update", handler);
    return () => {
      editor.off("selectionUpdate", handler);
      editor.off("update", handler);
    };
  }, [editor, enabled]);
  return null;
}

/** Lightweight TS port of engine/internal/node.CountChars. */
function countChars(node: any): number {
  if (!node || typeof node !== "object") return 0;
  if (node.type === "text" && typeof node.text === "string") return [...node.text].length;
  if (Array.isArray(node.content)) {
    let n = 0;
    for (const c of node.content) n += countChars(c);
    return n;
  }
  return 0;
}
