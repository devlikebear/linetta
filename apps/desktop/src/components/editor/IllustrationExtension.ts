import { Node, mergeAttributes } from "@tiptap/core";
import { ReactNodeViewRenderer } from "@tiptap/react";

import { IllustrationNodeView } from "./IllustrationNodeView";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    illustration: {
      insertIllustration: (id: string, caption: string) => ReturnType;
    };
  }
}

/* A picture from the work's gallery, as a block between paragraphs. It holds
 * the illustration's id and a caption of its own, never the bytes: the
 * engine's export, word count, search and agent tools all depend on that
 * shape (engine/internal/illustref). It is part of every editor's base set,
 * not passed in by one screen, because an editor without it drops the node
 * when it loads the scene and the next save takes the picture out. */
export const IllustrationExtension = Node.create({
  name: "illustration",

  group: "block",
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    return {
      id: {
        default: null,
        parseHTML: (el) => (el as HTMLElement).getAttribute("data-illustration-id"),
        renderHTML: (attrs) => ({ "data-illustration-id": attrs.id }),
      },
      caption: {
        default: "",
        parseHTML: (el) => (el as HTMLElement).getAttribute("data-caption") ?? "",
        renderHTML: (attrs) => ({ "data-caption": attrs.caption }),
      },
    };
  },

  parseHTML() {
    return [{ tag: "figure[data-illustration-id]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return ["figure", mergeAttributes(HTMLAttributes, { class: "illustration-node" })];
  },

  addNodeView() {
    return ReactNodeViewRenderer(IllustrationNodeView, {
      // Typing in the caption box is the input's, not the editor's.
      stopEvent: ({ event }) => (event.target as HTMLElement | null)?.tagName === "INPUT",
    });
  },

  addCommands() {
    return {
      insertIllustration:
        (id, caption) =>
        ({ commands }) =>
          commands.insertContent({ type: this.name, attrs: { id, caption } }),
    };
  },
});
