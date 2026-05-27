import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

/**
 * FocusExtension: dims top-level blocks (paragraphs / headings / blockquotes)
 * that fall OUTSIDE the writer's current "paragraph group". A group is the
 * contiguous run of non-empty top-level blocks around the cursor, bounded on
 * either side by an empty paragraph (the writer's visual paragraph break) or
 * the document edge. Empty paragraphs themselves are never highlighted — they
 * act as separators.
 */
const focusKey = new PluginKey("linetta-focus");

interface TopBlock {
  pos: number;
  size: number;
  empty: boolean;
}

function buildDecorations(state: any): DecorationSet {
  const { doc, selection } = state;

  // Collect top-level blocks. `descendants` returning false stops recursion
  // into children — we want the blockquote itself (not its inner paragraphs)
  // when one exists, so we always return false after recording a block.
  const blocks: TopBlock[] = [];
  doc.descendants((node: any, pos: number, parent: any) => {
    if (parent !== doc) return false;
    if (!node.isBlock || node.isLeaf) return false;
    blocks.push({
      pos,
      size: node.nodeSize,
      empty: node.content.size === 0,
    });
    return false;
  });

  // Find the block containing the cursor.
  let currentIdx = -1;
  for (let i = 0; i < blocks.length; i++) {
    const b = blocks[i];
    if (b.pos <= selection.head && selection.head <= b.pos + b.size) {
      currentIdx = i;
      break;
    }
  }
  if (currentIdx === -1) return DecorationSet.empty;

  // If the cursor itself is on an empty separator, nothing is in-group;
  // every non-empty block dims. (Writer hasn't started typing the next
  // paragraph yet.)
  let startIdx = currentIdx;
  let endIdx = currentIdx;
  if (!blocks[currentIdx].empty) {
    while (startIdx > 0 && !blocks[startIdx - 1].empty) startIdx--;
    while (endIdx < blocks.length - 1 && !blocks[endIdx + 1].empty) endIdx++;
  }

  const decorations: Decoration[] = [];
  for (let i = 0; i < blocks.length; i++) {
    if (i < startIdx || i > endIdx || blocks[currentIdx].empty) {
      const b = blocks[i];
      decorations.push(
        Decoration.node(b.pos, b.pos + b.size, { class: "tiptap-dim" }),
      );
    }
  }
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
