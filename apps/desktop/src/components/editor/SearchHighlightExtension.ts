import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

export interface SearchHighlightMatch {
  from: number;
  to: number;
}

interface SearchHighlightMeta {
  matches: SearchHighlightMatch[];
  activeIndex: number;
}

export const SearchHighlightPluginKey = new PluginKey<DecorationSet>("linettaSearchHighlight");

export const SearchHighlightExtension = Extension.create({
  name: "searchHighlight",

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: SearchHighlightPluginKey,
        state: {
          init: () => DecorationSet.empty,
          apply: (tr, oldDecorations, _oldState, newState) => {
            const meta = tr.getMeta(SearchHighlightPluginKey) as SearchHighlightMeta | null | undefined;
            if (meta !== undefined) {
              return buildDecorations(newState.doc, meta?.matches ?? [], meta?.activeIndex ?? -1);
            }
            return oldDecorations.map(tr.mapping, tr.doc);
          },
        },
        props: {
          decorations(state) {
            return SearchHighlightPluginKey.getState(state) ?? DecorationSet.empty;
          },
        },
      }),
    ];
  },
});

function buildDecorations(doc: any, matches: SearchHighlightMatch[], activeIndex: number): DecorationSet {
  if (matches.length === 0) return DecorationSet.empty;

  const decorations = matches.map((match, index) => Decoration.inline(
    match.from,
    match.to,
    {
      class: index === activeIndex
        ? "tiptap-search-match tiptap-search-match-active"
        : "tiptap-search-match",
    },
    { inclusiveStart: false, inclusiveEnd: false },
  ));

  return DecorationSet.create(doc, decorations);
}
