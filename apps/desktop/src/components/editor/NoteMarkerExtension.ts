import { Node, mergeAttributes, type RawCommands } from "@tiptap/core";
import { dispatchAppEvent } from "../../lib/appEvents";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    noteMarker: {
      addNoteMarker: (noteId: string) => ReturnType;
      removeNoteMarker: (noteId: string) => ReturnType;
    };
  }
}

export const NoteMarkerExtension = Node.create({
  name: "noteMarker",

  inline: true,
  group: "inline",
  atom: true,
  selectable: true,
  draggable: false,

  addAttributes() {
    return {
      noteId: {
        default: null,
        parseHTML: (el) => (el as HTMLElement).getAttribute("data-note-id"),
        renderHTML: (attrs) => ({ "data-note-id": attrs.noteId }),
      },
    };
  },

  parseHTML() {
    return [{ tag: "span.note-marker[data-note-id]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "span",
      mergeAttributes(HTMLAttributes, { class: "note-marker" }),
      "☘︎",
    ];
  },

  addNodeView() {
    return ({ node }) => {
      const dom = document.createElement("span");
      dom.className = "note-marker";
      dom.setAttribute("data-note-id", node.attrs.noteId ?? "");
      dom.setAttribute("contenteditable", "false");
      dom.textContent = "☘︎";

      const onEnter = () => {
        dispatchAppEvent("linetta:note-hover", { noteId: node.attrs.noteId, target: dom });
      };
      const onLeave = () => {
        dispatchAppEvent("linetta:note-hover-end", { noteId: node.attrs.noteId });
      };
      const onClick = (e: MouseEvent) => {
        e.preventDefault();
        e.stopPropagation();
        dispatchAppEvent("linetta:note-click", { noteId: node.attrs.noteId, target: dom });
      };

      dom.addEventListener("mouseenter", onEnter);
      dom.addEventListener("mouseleave", onLeave);
      dom.addEventListener("mousedown", onClick);

      return {
        dom,
        destroy() {
          dom.removeEventListener("mouseenter", onEnter);
          dom.removeEventListener("mouseleave", onLeave);
          dom.removeEventListener("mousedown", onClick);
        },
      };
    };
  },

  addCommands(): Partial<RawCommands> {
    return {
      addNoteMarker:
        (noteId: string) =>
        ({ chain }) =>
          chain()
            .focus()
            .insertContent({ type: "noteMarker", attrs: { noteId } })
            .run(),
      removeNoteMarker:
        (noteId: string) =>
        ({ state, dispatch, tr }) => {
          let foundPos = -1;
          let foundSize = 0;
          state.doc.descendants((n, pos) => {
            if (foundPos !== -1) return false;
            if (n.type.name === "noteMarker" && n.attrs.noteId === noteId) {
              foundPos = pos;
              foundSize = n.nodeSize;
              return false;
            }
            return true;
          });
          if (foundPos === -1) return false;
          if (dispatch) {
            dispatch(tr.delete(foundPos, foundPos + foundSize));
          }
          return true;
        },
    };
  },
});
