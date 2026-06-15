import { useEditor, EditorContent, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";
import { FocusExtension } from "./FocusExtension";
import "./Tiptap.css";

export interface TiptapHandle {
  /** Move keyboard focus into the editor (end of current document). */
  focus: () => void;
  /** Return the current Tiptap JSON doc. */
  getDoc: () => object;
  /** Return the current ProseMirror selection range, or null if no editor view. */
  getSelection: () => { from: number; to: number } | null;
  /** Set the ProseMirror selection (clamped to doc size) and focus the view. */
  setSelection: (sel: { from: number; to: number }) => void;
  /** Find text matches inside the current scene, optionally selecting the first match. */
  findText: (query: string, options?: TiptapFindOptions) => TiptapFindResult;
  /** Select the next current-scene match, wrapping at the end. */
  nextMatch: () => void;
  /** Select the previous current-scene match, wrapping at the start. */
  prevMatch: () => void;
  /** Replace the selected current-scene match and return the updated doc. */
  replaceActiveMatch: (replacement: string) => object | null;
  /** Replace every current-scene text match and return the updated doc. */
  replaceAllMatches: (query: string, replacement: string) => object | null;
  /** Insert a note-marker atom at the current selection. */
  addNoteMarker: (noteId: string) => void;
  /** Remove the first note-marker atom with the given noteId. */
  removeNoteMarker: (noteId: string) => void;
  /** Underlying Tiptap Editor instance (null until first mount). Used by
   *  ghost-text + Cmd+I AI prompt bar to read selection and dispatch commands. */
  editor: Editor | null;
}

export interface TiptapFindResult {
  count: number;
  activeIndex: number;
}

export interface TiptapFindOptions {
  select?: boolean;
}

interface TextMatch {
  from: number;
  to: number;
}

export interface TiptapSelectionMenuPayload {
  from: number;
  to: number;
  text: string;
}

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
  /** Extra Tiptap extensions to merge with StarterKit (e.g., MentionExtension). */
  extensions?: any[];
  /** Fired when a `.mention` atom inside the doc is double-clicked. */
  onMentionDoubleClick?: (entityId: string) => void;
  /** Focus mode: dim every paragraph except the one containing the cursor. */
  focus?: boolean;
  /** Fired when the writer right-clicks a non-empty editor selection. */
  onSelectionContextMenu?: (event: React.MouseEvent, payload: TiptapSelectionMenuPayload) => void;
}

export const TiptapEditor = forwardRef<TiptapHandle, Props>(function TiptapEditor(
  { initialDoc, onChange, onCharCount, typewriter, onManualSave, extensions, onMentionDoubleClick, focus, onSelectionContextMenu },
  ref,
) {
  // Stable reference for the initial doc to avoid resetting on every render.
  const initialKey = useMemo(() => JSON.stringify(initialDoc).length, [initialDoc]);
  const [emptyFocused, setEmptyFocused] = useState(false);
  const matchesRef = useRef<TextMatch[]>([]);
  const activeMatchRef = useRef(-1);
  const lastFindQueryRef = useRef("");

  const editor = useEditor(
    {
      extensions: [
        StarterKit.configure({}),
        ...(extensions ?? []),
        ...(focus ? [FocusExtension] : []),
      ],
      content: initialDoc,
      autofocus: "end",
      editorProps: {
        // Apply the global design-system prose typography to the editable area
        // so the real Tiptap surface matches the redesign's `.prose` styling.
        attributes: { class: "prose" },
      },
      onUpdate: ({ editor }) => {
        const doc = editor.getJSON();
        onChange(doc);
        if (onCharCount) onCharCount(countChars(doc));
      },
    },
    // Re-create the editor only when the initial doc actually changes id/length —
    // avoids cursor jumps from upstream re-renders. Toggling `focus` is rare,
    // so an editor re-init (with cursor loss) is acceptable.
    [initialKey, focus],
  );

  useEffect(() => {
    if (!editor) return;
    if (onCharCount) onCharCount(countChars(editor.getJSON()));
  }, [editor, onCharCount]);

  useEffect(() => {
    if (!editor) {
      setEmptyFocused(false);
      return;
    }

    const updateEmptyFocused = () => setEmptyFocused(editor.isFocused && editor.isEmpty);
    updateEmptyFocused();
    editor.on("focus", updateEmptyFocused);
    editor.on("blur", updateEmptyFocused);
    editor.on("update", updateEmptyFocused);
    editor.on("selectionUpdate", updateEmptyFocused);

    return () => {
      editor.off("focus", updateEmptyFocused);
      editor.off("blur", updateEmptyFocused);
      editor.off("update", updateEmptyFocused);
      editor.off("selectionUpdate", updateEmptyFocused);
    };
  }, [editor]);

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

  // Expose a tiny imperative API so parents can refocus the editor (e.g. after
  // the outline auto-retracts or a dialog closes).
  useImperativeHandle(
    ref,
    () => ({
      focus: () => editor?.commands.focus("end"),
      getDoc: () => editor?.getJSON() ?? {},
      getSelection: () => {
        if (!editor) return null;
        const { from, to } = editor.state.selection;
        return { from, to };
      },
      setSelection: (sel) => {
        if (!editor) return;
        const size = editor.state.doc.content.size;
        const from = Math.min(Math.max(0, sel.from), size);
        const to = Math.min(Math.max(from, sel.to), size);
        editor.commands.setTextSelection({ from, to });
        editor.view.focus();
      },
      findText: (query, options) => {
        if (!editor) return { count: 0, activeIndex: -1 };
        const matches = findTextMatches(editor, query);
        matchesRef.current = matches;
        activeMatchRef.current = matches.length > 0 ? 0 : -1;
        lastFindQueryRef.current = query;
        if (options?.select !== false) selectMatch(editor, matches, activeMatchRef.current);
        return { count: matches.length, activeIndex: activeMatchRef.current };
      },
      nextMatch: () => {
        if (!editor || matchesRef.current.length === 0) return;
        activeMatchRef.current = (activeMatchRef.current + 1) % matchesRef.current.length;
        selectMatch(editor, matchesRef.current, activeMatchRef.current);
      },
      prevMatch: () => {
        if (!editor || matchesRef.current.length === 0) return;
        activeMatchRef.current = (activeMatchRef.current - 1 + matchesRef.current.length) % matchesRef.current.length;
        selectMatch(editor, matchesRef.current, activeMatchRef.current);
      },
      replaceActiveMatch: (replacement) => {
        if (!editor) return null;
        const match = matchesRef.current[activeMatchRef.current];
        if (!match) return null;

        const previousIndex = activeMatchRef.current;
        editor.commands.insertContentAt({ from: match.from, to: match.to }, replacement);

        const matches = findTextMatches(editor, lastFindQueryRef.current);
        matchesRef.current = matches;
        activeMatchRef.current = matches.length > 0 ? Math.min(previousIndex, matches.length - 1) : -1;
        selectMatch(editor, matches, activeMatchRef.current);
        return editor.getJSON();
      },
      replaceAllMatches: (query, replacement) => {
        if (!editor) return null;
        const matches = findTextMatches(editor, query);
        if (matches.length === 0) return null;

        let tr = editor.state.tr;
        for (const match of [...matches].reverse()) {
          tr = tr.insertText(replacement, match.from, match.to);
        }
        editor.view.dispatch(tr);

        matchesRef.current = findTextMatches(editor, query);
        activeMatchRef.current = matchesRef.current.length > 0 ? 0 : -1;
        lastFindQueryRef.current = query;
        selectMatch(editor, matchesRef.current, activeMatchRef.current);
        return editor.getJSON();
      },
      addNoteMarker: (noteId: string) => {
        (editor?.commands as any)?.addNoteMarker?.(noteId);
      },
      removeNoteMarker: (noteId: string) => {
        (editor?.commands as any)?.removeNoteMarker?.(noteId);
      },
      editor,
    }),
    [editor],
  );

  // Clicking anywhere in the wrap (including the generous margins / typewriter
  // padding) should focus the editor — otherwise the writer can't tell where
  // the editable area starts.
  const onWrapMouseDown = (e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest(".ProseMirror")) return;
    e.preventDefault(); // keep focus from jumping to the wrap itself
    editor?.commands.focus("end");
  };

  const onWrapContextMenu = (e: React.MouseEvent) => {
    if (!editor || !onSelectionContextMenu) return;
    if (!(e.target as HTMLElement).closest(".ProseMirror")) return;
    const { from, to, empty } = editor.state.selection;
    if (empty || from === to) return;
    const text = editor.state.doc.textBetween(from, to, "\n").trim();
    if (!text) return;
    e.preventDefault();
    onSelectionContextMenu(e, { from, to, text });
  };

  return (
    <div
      className={`tiptap-wrap${typewriter ? " typewriter" : ""}${emptyFocused ? " empty-focused" : ""}`}
      onMouseDown={onWrapMouseDown}
      onContextMenu={onWrapContextMenu}
      onDoubleClick={(e) => {
        const t = (e.target as HTMLElement).closest(".mention");
        if (t && onMentionDoubleClick) {
          const id = t.getAttribute("data-entity-id") || t.getAttribute("data-id");
          if (id) onMentionDoubleClick(id);
        }
      }}
    >
      <EditorContent editor={editor} className="tiptap-editor" />
      <TypewriterScroll editor={editor} enabled={!!typewriter} />
    </div>
  );
});

/** Scrolls the editor so the current cursor line stays near the column's center.
 *  Scrolls only the editor's nearest scrollable ancestor (never the page),
 *  so the right context panel is unaffected. */
function TypewriterScroll({ editor, enabled }: { editor: Editor | null; enabled: boolean }) {
  const lastDelta = useRef<number>(-1);
  useEffect(() => {
    if (!editor || !enabled) return;
    const handler = () => {
      const view = editor.view;
      const pos = view.state.selection.head;
      const coords = view.coordsAtPos(pos);
      const scroller = findScrollableParent(view.dom);
      if (!scroller) return;
      const rect = scroller.getBoundingClientRect();
      const target = rect.top + rect.height / 2;
      const delta = coords.top - target;
      if (Math.abs(delta) < 4) return;
      if (delta === lastDelta.current) return;
      lastDelta.current = delta;
      scroller.scrollBy({ top: delta, behavior: "smooth" });
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

function findScrollableParent(el: HTMLElement | null): HTMLElement | null {
  while (el && el !== document.body) {
    const style = window.getComputedStyle(el);
    if (style.overflowY === "auto" || style.overflowY === "scroll") {
      return el;
    }
    el = el.parentElement;
  }
  return null;
}

function findTextMatches(editor: Editor, query: string): TextMatch[] {
  const needle = query.trim();
  if (!needle) return [];

  const normalizedNeedle = needle.toLocaleLowerCase();
  const matches: TextMatch[] = [];

  editor.state.doc.descendants((node, pos) => {
    if (!node.isText || !node.text) return;
    const normalizedText = node.text.toLocaleLowerCase();
    let index = normalizedText.indexOf(normalizedNeedle);
    while (index >= 0) {
      matches.push({ from: pos + index, to: pos + index + needle.length });
      index = normalizedText.indexOf(normalizedNeedle, index + normalizedNeedle.length);
    }
  });

  return matches;
}

function selectMatch(editor: Editor, matches: TextMatch[], activeIndex: number) {
  const match = matches[activeIndex];
  if (!match) return;
  editor.commands.setTextSelection({ from: match.from, to: match.to });
  editor.view.focus();
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
