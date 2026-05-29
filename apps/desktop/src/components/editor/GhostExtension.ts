import { Extension, type RawCommands } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import "./GhostExtension.css";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    ghost: {
      /**
       * Set or replace the ghost text. mode defaults to "insert" at the current
       * selection's head; pass {kind: "replace", from, to} to commit by replacing
       * a range when accepted.
       */
      setGhostText: (text: string, mode?: GhostMode) => ReturnType;
      /** Accept the ghost text — insert into (or replace) the document. */
      acceptGhostText: () => ReturnType;
      /** Drop the ghost text — clear decoration without inserting. */
      dropGhostText: () => ReturnType;
    };
  }
}

export type GhostMode =
  | { kind: "insert"; pos: number }
  | { kind: "replace"; from: number; to: number };

export interface GhostState {
  /** Where the ghost text will be committed: at a single position, or replacing a range. */
  mode: GhostMode;
  /** Accumulated text streamed so far. */
  text: string;
  /** True once the stream has completed (cursor stops blinking). */
  done: boolean;
}

export const ghostPluginKey = new PluginKey<GhostState | null>("linetta-ghost");

export const GhostExtension = Extension.create({
  name: "linettaGhost",

  addProseMirrorPlugins() {
    return [
      new Plugin<GhostState | null>({
        key: ghostPluginKey,
        state: {
          init: () => null,
          apply(tr, prev) {
            const meta = tr.getMeta(ghostPluginKey) as
              | { kind: "set"; mode: GhostMode; text: string }
              | { kind: "drop" }
              | { kind: "done" }
              | undefined;

            if (meta?.kind === "set") {
              return { mode: meta.mode, text: meta.text, done: false };
            }
            if (meta?.kind === "drop") {
              return null;
            }
            if (meta?.kind === "done" && prev) {
              return { ...prev, done: true };
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
                const span = document.createElement("span");
                span.className = "ai-ghost" + (ghost.done ? " done" : "");
                span.textContent = ghost.text;
                return span;
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
        if (!ghost) return false; // Let Tab fall through to other handlers.
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
              tr.setMeta(ghostPluginKey, { kind: "set", mode: effectiveMode, text }),
            );
          }
          return true;
        },
      acceptGhostText:
        () =>
        ({ tr, state, dispatch }) => {
          const ghost = ghostPluginKey.getState(state);
          if (!ghost) return false;
          if (dispatch) {
            // Force plain-text commit (no mark inheritance from surrounding
            // selection). schema.text(text) constructs a text node with no marks;
            // replaceWith replaces the target range with that node.
            const node = state.schema.text(ghost.text);
            let nextTr;
            if (ghost.mode.kind === "insert") {
              nextTr = tr.replaceWith(ghost.mode.pos, ghost.mode.pos, node);
            } else {
              nextTr = tr.replaceWith(ghost.mode.from, ghost.mode.to, node);
            }
            nextTr.setMeta(ghostPluginKey, { kind: "drop" });
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
            dispatch(tr.setMeta(ghostPluginKey, { kind: "drop" }));
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
