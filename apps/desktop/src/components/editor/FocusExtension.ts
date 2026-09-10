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
export const focusKey = new PluginKey<FocusPluginState>("linetta-focus");

interface TopBlock {
  pos: number;
  size: number;
  empty: boolean;
}

interface FocusPluginState {
  enabled: boolean;
  decorations: DecorationSet;
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

/**
 * `enabled` is a Tiptap extension option (set once, at construction) so the
 * plugin's initial state matches the `focus` prop on first mount. After that
 * the editor instance is kept alive across `focus` toggles (#103 — recreating
 * it on every toggle re-injected the stale `initialDoc` prop instead of the
 * live document) and Tiptap.tsx flips `enabled` by dispatching a transaction
 * meta on `focusKey` instead.
 */
export const FocusExtension = Extension.create<{ enabled: boolean }>({
  name: "linettaFocus",
  addOptions() {
    return { enabled: true };
  },
  addProseMirrorPlugins() {
    const initialEnabled = this.options.enabled;
    return [
      new Plugin<FocusPluginState>({
        key: focusKey,
        state: {
          init: (_, state) => ({
            enabled: initialEnabled,
            decorations: initialEnabled ? buildDecorations(state) : DecorationSet.empty,
          }),
          apply(tr, old, _oldState, newState) {
            const meta = tr.getMeta(focusKey) as { enabled?: boolean } | undefined;
            const enabled = typeof meta?.enabled === "boolean" ? meta.enabled : old.enabled;
            if (!enabled) {
              return old.enabled === enabled && old.decorations === DecorationSet.empty
                ? old
                : { enabled, decorations: DecorationSet.empty };
            }
            if (!tr.docChanged && !tr.selectionSet && enabled === old.enabled) return old;
            return { enabled, decorations: buildDecorations(newState) };
          },
        },
        props: {
          decorations(state) {
            return focusKey.getState(state)?.decorations ?? DecorationSet.empty;
          },
        },
      }),
    ];
  },
});
