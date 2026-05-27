import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

/**
 * FocusExtension: when active, dims every top-level paragraph/heading/
 * blockquote except the one containing the current selection. Adds the
 * CSS class `tiptap-dim` to dimmed blocks.
 */
const focusKey = new PluginKey("linetta-focus");

function buildDecorations(state: any): DecorationSet {
  const { doc, selection } = state;
  const decorations: Decoration[] = [];
  // The current paragraph is the deepest block ancestor of the head.
  let currentBlockPos = -1;
  doc.descendants((node: any, pos: number) => {
    if (!node.isBlock || node.isLeaf) return true;
    if (pos <= selection.head && selection.head <= pos + node.nodeSize) {
      currentBlockPos = pos;
    }
    return true;
  });
  doc.descendants((node: any, pos: number) => {
    if (!node.isBlock || node.isLeaf) return true;
    if (pos !== currentBlockPos) {
      decorations.push(
        Decoration.node(pos, pos + node.nodeSize, { class: "tiptap-dim" }),
      );
    }
    return true;
  });
  return DecorationSet.create(doc, decorations);
}

export const FocusExtension = Extension.create({
  name: "linettaFocus",
  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: focusKey,
        state: {
          init: (_, state) => buildDecorations(state),
          apply: (_tr, _old, _oldState, newState) => buildDecorations(newState),
        },
        props: {
          decorations(state) {
            return (this as any).getState(state);
          },
        },
      }),
    ];
  },
});
