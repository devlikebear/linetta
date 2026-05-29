import { Extension, type RawCommands } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import "./GhostExtension.css";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    ghost: {
      /** Set or replace the ghost text at the current selection's head. */
      setGhostText: (text: string) => ReturnType;
      /** Accept the ghost text — insert into the document. */
      acceptGhostText: () => ReturnType;
      /** Drop the ghost text — clear decoration without inserting. */
      dropGhostText: () => ReturnType;
    };
  }
}

export interface GhostState {
  /** Position the ghost is anchored to (head of selection when setGhostText was called). */
  pos: number;
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
              | { kind: "set"; pos: number; text: string }
              | { kind: "drop" }
              | { kind: "done" }
              | undefined;

            if (meta?.kind === "set") {
              return { pos: meta.pos, text: meta.text, done: false };
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
            const widget = Decoration.widget(
              ghost.pos,
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

  addCommands() {
    return {
      setGhostText:
        (text: string) =>
        ({ tr, state, dispatch }) => {
          const pos = state.selection.head;
          if (dispatch) {
            dispatch(tr.setMeta(ghostPluginKey, { kind: "set", pos, text }));
          }
          return true;
        },
      acceptGhostText:
        () =>
        ({ tr, state, dispatch }) => {
          const ghost = ghostPluginKey.getState(state);
          if (!ghost) return false;
          if (dispatch) {
            const insertTr = tr.insertText(ghost.text, ghost.pos);
            insertTr.setMeta(ghostPluginKey, { kind: "drop" });
            dispatch(insertTr);
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
