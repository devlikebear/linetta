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

/** A passage opened from elsewhere (a continuity citation): the quote when it
 *  was found, else the cited paragraphs. Kept apart from the find bar's
 *  matches so neither clears the other. `null` clears it. */
export interface RevealHighlightMeta {
  from: number;
  to: number;
  kind: "quote" | "paragraph";
}

export const RevealHighlightPluginKey = new PluginKey<DecorationSet>("linettaRevealHighlight");

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
      new Plugin({
        key: RevealHighlightPluginKey,
        state: {
          init: () => DecorationSet.empty,
          apply: (tr, old, _oldState, newState) => {
            const meta = tr.getMeta(RevealHighlightPluginKey) as RevealHighlightMeta | null | undefined;
            if (meta === undefined) return old.map(tr.mapping, tr.doc);
            if (meta === null) return DecorationSet.empty;
            return DecorationSet.create(newState.doc, [
              Decoration.inline(meta.from, meta.to, { class: `tiptap-reveal tiptap-reveal-${meta.kind}` }),
            ]);
          },
        },
        props: {
          decorations(state) {
            return RevealHighlightPluginKey.getState(state) ?? DecorationSet.empty;
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
