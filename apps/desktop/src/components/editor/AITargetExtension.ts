import { Extension, type RawCommands } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import "./AITargetExtension.css";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    aiTarget: {
      /** Show the target highlight for the AI panel. */
      setAITarget: (mode: AITargetMode, from: number, to: number) => ReturnType;
      /** Remove the target highlight. */
      clearAITarget: () => ReturnType;
    };
  }
}

export type AITargetMode = "replace" | "insert" | "replaceAll";

export interface AITargetState {
  mode: AITargetMode;
  from: number;
  to: number;
}

export const aiTargetPluginKey = new PluginKey<AITargetState | null>("linetta-ai-target");

type AITargetMeta =
  | { kind: "set"; mode: AITargetMode; from: number; to: number }
  | { kind: "clear" };

export const AITargetExtension = Extension.create({
  name: "linettaAITarget",

  addProseMirrorPlugins() {
    return [
      new Plugin<AITargetState | null>({
        key: aiTargetPluginKey,
        state: {
          init: () => null,
          apply(tr, prev) {
            const meta = tr.getMeta(aiTargetPluginKey) as AITargetMeta | undefined;
            if (meta?.kind === "set") {
              return { mode: meta.mode, from: meta.from, to: meta.to };
            }
            if (meta?.kind === "clear") {
              return null;
            }
            // Map stored positions through doc changes (defensive; editor is
            // normally locked while a target is active so docChanged is rare).
            if (prev && tr.docChanged) {
              return {
                ...prev,
                from: tr.mapping.map(prev.from),
                to: tr.mapping.map(prev.to),
              };
            }
            return prev;
          },
        },
        props: {
          decorations(state) {
            const t = this.getState(state);
            if (!t) return DecorationSet.empty;
            const size = state.doc.content.size;
            if (t.mode === "insert") {
              const pos = Math.min(Math.max(0, t.from), size);
              return DecorationSet.create(state.doc, [
                Decoration.widget(
                  pos,
                  () => {
                    const el = document.createElement("span");
                    el.className = "ai-target-caret";
                    return el;
                  },
                  { side: 0 },
                ),
              ]);
            }
            const cls = t.mode === "replaceAll" ? "ai-target-all" : "ai-target-replace";
            const from = Math.max(1, Math.min(t.from, size));
            const to = Math.max(1, Math.min(t.to, size));
            if (to <= from) return DecorationSet.empty;
            return DecorationSet.create(state.doc, [
              Decoration.inline(from, to, { class: cls }),
            ]);
          },
        },
      }),
    ];
  },

  addCommands() {
    return {
      setAITarget:
        (mode: AITargetMode, from: number, to: number) =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            dispatch(
              tr.setMeta(aiTargetPluginKey, { kind: "set", mode, from, to } satisfies AITargetMeta),
            );
          }
          return true;
        },
      clearAITarget:
        () =>
        ({ tr, state, dispatch }) => {
          if (aiTargetPluginKey.getState(state) === null) return false;
          if (dispatch) {
            dispatch(tr.setMeta(aiTargetPluginKey, { kind: "clear" } satisfies AITargetMeta));
          }
          return true;
        },
    } as Partial<RawCommands>;
  },
});
