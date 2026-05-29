import { Extension, type RawCommands } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import "./GhostExtension.css";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    ghost: {
      /**
       * Backward-compat single-variation init. Equivalent to
       * setGhostVariations(1, mode) + setGhostVariationText(0, text).
       */
      setGhostText: (text: string, mode?: GhostMode) => ReturnType;
      /** Plan 20: init N empty variation slots. currentIdx resets to 0. */
      setGhostVariations: (count: number, mode: GhostMode) => ReturnType;
      /** Plan 20: replace text on a specific variation. */
      setGhostVariationText: (idx: number, text: string) => ReturnType;
      /** Plan 20: mark a variation done (with optional error message). */
      setGhostVariationDone: (idx: number, error?: string) => ReturnType;
      /** Plan 20: switch the currently visible variation. Wraps modulo N. No-op when N===1. */
      switchGhostVariation: (direction: -1 | 1) => ReturnType;
      /** Accept the currently visible variation — insert into (or replace) the document. */
      acceptGhostText: () => ReturnType;
      /** Drop the ghost — clear decoration without inserting. */
      dropGhostText: () => ReturnType;
    };
  }
}

export type GhostMode =
  | { kind: "insert"; pos: number }
  | { kind: "replace"; from: number; to: number };

export interface GhostVariation {
  text: string;
  done: boolean;
  /** Optional per-variation error message; if set, also treated as done. */
  error?: string;
}

export interface GhostState {
  mode: GhostMode;
  variations: GhostVariation[];
  currentIdx: number;
}

export const ghostPluginKey = new PluginKey<GhostState | null>("linetta-ghost");

type GhostMeta =
  | { kind: "set"; mode: GhostMode; text: string }
  | { kind: "setVariations"; mode: GhostMode; count: number }
  | { kind: "setVariationText"; idx: number; text: string }
  | { kind: "setVariationDone"; idx: number; error?: string }
  | { kind: "switchVariation"; direction: -1 | 1 }
  | { kind: "drop" }
  | { kind: "done" };

export const GhostExtension = Extension.create({
  name: "linettaGhost",

  addProseMirrorPlugins() {
    return [
      new Plugin<GhostState | null>({
        key: ghostPluginKey,
        state: {
          init: () => null,
          apply(tr, prev) {
            const meta = tr.getMeta(ghostPluginKey) as GhostMeta | undefined;

            if (meta?.kind === "set") {
              // Backward-compat single-variation init.
              return {
                mode: meta.mode,
                variations: [{ text: meta.text, done: false }],
                currentIdx: 0,
              };
            }
            if (meta?.kind === "setVariations") {
              const variations: GhostVariation[] = [];
              for (let i = 0; i < meta.count; i++) {
                variations.push({ text: "", done: false });
              }
              return { mode: meta.mode, variations, currentIdx: 0 };
            }
            if (meta?.kind === "setVariationText" && prev) {
              if (meta.idx < 0 || meta.idx >= prev.variations.length) return prev;
              const next = prev.variations.slice();
              next[meta.idx] = { ...next[meta.idx], text: meta.text };
              return { ...prev, variations: next };
            }
            if (meta?.kind === "setVariationDone" && prev) {
              if (meta.idx < 0 || meta.idx >= prev.variations.length) return prev;
              const next = prev.variations.slice();
              next[meta.idx] = { ...next[meta.idx], done: true, error: meta.error };
              return { ...prev, variations: next };
            }
            if (meta?.kind === "switchVariation" && prev) {
              const n = prev.variations.length;
              if (n <= 1) return prev;
              const nextIdx = ((prev.currentIdx + meta.direction) % n + n) % n;
              return { ...prev, currentIdx: nextIdx };
            }
            if (meta?.kind === "drop") {
              return null;
            }
            if (meta?.kind === "done" && prev) {
              // Legacy single-mode "done" — marks current (only) variation done.
              if (prev.variations.length === 0) return prev;
              const next = prev.variations.slice();
              next[prev.currentIdx] = { ...next[prev.currentIdx], done: true };
              return { ...prev, variations: next };
            }
            // Plan 18 design 2.7: auto-drop on doc edit.
            if (prev && tr.docChanged) {
              return null;
            }
            return prev;
          },
        },
        props: {
          decorations(state) {
            const ghost = this.getState(state);
            if (!ghost) return DecorationSet.empty;
            const pos = ghost.mode.kind === "insert" ? ghost.mode.pos : ghost.mode.to;
            const widget = Decoration.widget(
              pos,
              () => {
                const wrap = document.createElement("span");
                wrap.className = "ai-ghost-wrap";

                const current = ghost.variations[ghost.currentIdx];
                const textSpan = document.createElement("span");
                textSpan.className = "ai-ghost" + (current.done ? " done" : "");
                if (current.error) {
                  textSpan.className += " ai-ghost-error";
                  textSpan.textContent = `(오류: ${current.error})`;
                } else {
                  textSpan.textContent = current.text;
                }
                wrap.appendChild(textSpan);

                if (ghost.variations.length > 1) {
                  const indicator = document.createElement("div");
                  indicator.className = "ai-ghost-indicator";
                  indicator.textContent = `[${ghost.currentIdx + 1}/${ghost.variations.length}] ◀ ▶  Tab 수락`;
                  wrap.appendChild(indicator);
                }
                return wrap;
              },
              { side: 1 },
            );
            return DecorationSet.create(state.doc, [widget]);
          },
        },
      }),
    ];
  },

  addKeyboardShortcuts() {
    return {
      Tab: ({ editor }) => {
        const ghost = ghostPluginKey.getState(editor.state);
        if (!ghost) return false;
        return editor.commands.acceptGhostText();
      },
      Escape: ({ editor }) => {
        const ghost = ghostPluginKey.getState(editor.state);
        if (!ghost) return false;
        return editor.commands.dropGhostText();
      },
    };
  },

  addCommands() {
    return {
      setGhostText:
        (text: string, mode?: GhostMode) =>
        ({ tr, state, dispatch }) => {
          const effectiveMode: GhostMode =
            mode ?? { kind: "insert", pos: state.selection.head };
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "set",
                mode: effectiveMode,
                text,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      setGhostVariations:
        (count: number, mode: GhostMode) =>
        ({ tr, dispatch }) => {
          if (count < 1) return false;
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "setVariations",
                mode,
                count,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      setGhostVariationText:
        (idx: number, text: string) =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "setVariationText",
                idx,
                text,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      setGhostVariationDone:
        (idx: number, error?: string) =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "setVariationDone",
                idx,
                error,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      switchGhostVariation:
        (direction: -1 | 1) =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "switchVariation",
                direction,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      acceptGhostText:
        () =>
        ({ tr, state, dispatch }) => {
          const ghost = ghostPluginKey.getState(state);
          if (!ghost) return false;
          const current = ghost.variations[ghost.currentIdx];
          if (current.error) return false; // accept on error variation = no-op
          if (dispatch) {
            // Force plain-text commit (no mark inheritance from surrounding
            // selection). schema.text(text) constructs a text node with no marks;
            // replaceWith replaces the target range with that node.
            const node = state.schema.text(current.text);
            let nextTr;
            if (ghost.mode.kind === "insert") {
              nextTr = tr.replaceWith(ghost.mode.pos, ghost.mode.pos, node);
            } else {
              nextTr = tr.replaceWith(ghost.mode.from, ghost.mode.to, node);
            }
            nextTr.setMeta(ghostPluginKey, { kind: "drop" } satisfies GhostMeta);
            dispatch(nextTr);
          }
          return true;
        },
      dropGhostText:
        () =>
        ({ tr, state, dispatch }) => {
          const ghost = ghostPluginKey.getState(state);
          if (!ghost) return false;
          if (dispatch) {
            dispatch(tr.setMeta(ghostPluginKey, { kind: "drop" } satisfies GhostMeta));
          }
          return true;
        },
    } as Partial<RawCommands>;
  },
});

/** Read-only: is a ghost currently active? Used by useGhostText / AIPromptBar. */
export function hasActiveGhost(editor: { state?: any; view?: { state: any } } | null | undefined): boolean {
  if (!editor) return false;
  const state = editor.state ?? editor.view?.state;
  if (!state) return false;
  return ghostPluginKey.getState(state) !== null;
}
