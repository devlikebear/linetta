import Mention from "@tiptap/extension-mention";

export interface MentionItem {
  /** Existing entity id, or undefined for the "new entity" sentinel. */
  id?: string;
  /** Display name. For the sentinel: the typed-but-unmatched query. */
  name: string;
  /** Optional role for hint display. */
  role?: string;
  /** True for the "new entity" sentinel; the consumer creates it before insert. */
  isNew?: boolean;
}

export interface MentionPickerState {
  open: boolean;
  query: string;
  position: { left: number; top: number };
  items: MentionItem[];
  selectedIndex: number;
  /** Run the picker's currently-selected item. */
  pick: () => void;
  /** Run a specific item (by index). */
  pickAt: (index: number) => void;
  /** Move the selection up/down. */
  move: (delta: number) => void;
}

interface BuildOpts {
  search: (query: string) => Promise<MentionItem[]>;
  onStateChange: (state: MentionPickerState | null) => void;
}

export function buildMentionExtension(opts: BuildOpts) {
  return Mention.configure({
    HTMLAttributes: { class: "mention" },
    renderText({ node }) {
      return `@${node.attrs.label ?? ""}`;
    },
    suggestion: {
      char: "@",
      items: async ({ query }: { query: string }) => {
        const matched = await opts.search(query);
        if (query.trim().length > 0 && !matched.some((m) => !m.isNew && m.name === query)) {
          matched.push({ name: query, isNew: true });
        }
        return matched;
      },
      command: ({ editor, range, props }: { editor: any; range: any; props: any }) => {
        editor
          .chain()
          .focus()
          .insertContentAt(range, [
            { type: "mention", attrs: { id: props.id, label: props.label } },
            { type: "text", text: " " },
          ])
          .run();
      },
      render: () => {
        let currentItems: MentionItem[] = [];
        let currentRange: { from: number; to: number } = { from: 0, to: 0 };
        let currentEditor: any | null = null;
        let currentQuery = "";
        let currentClientRect: (() => DOMRect | null) | null = null;
        let selectedIndex = 0;

        const recompute = () => {
          const rect = currentClientRect?.();
          opts.onStateChange({
            open: true,
            query: currentQuery,
            position: rect
              ? { left: rect.left, top: rect.bottom + 4 }
              : { left: 0, top: 0 },
            items: currentItems,
            selectedIndex,
            pick: () => pickAt(selectedIndex),
            pickAt,
            move: (delta: number) => {
              if (currentItems.length === 0) return;
              selectedIndex = (selectedIndex + delta + currentItems.length) % currentItems.length;
              recompute();
            },
          });
        };

        const pickAt = (index: number) => {
          const item = currentItems[index];
          if (!item || !currentEditor) return;
          if (item.id && !item.isNew) {
            currentEditor
              .chain()
              .focus()
              .deleteRange(currentRange)
              .insertContent([
                { type: "mention", attrs: { id: item.id, label: item.name } },
                { type: "text", text: " " },
              ])
              .run();
            opts.onStateChange(null);
          } else {
            // Hand off to the Workspace via a custom DOM event so it can create
            // the entity, then splice in the mention.
            window.dispatchEvent(
              new CustomEvent("linetta:mention-pick-new", {
                detail: { query: item.name, range: currentRange, editor: currentEditor },
              }),
            );
            opts.onStateChange(null);
          }
        };

        return {
          onStart: (props: any) => {
            currentItems = props.items as MentionItem[];
            currentRange = props.range;
            currentEditor = props.editor;
            currentQuery = props.query;
            currentClientRect = props.clientRect ?? null;
            selectedIndex = 0;
            recompute();
          },
          onUpdate: (props: any) => {
            currentItems = props.items as MentionItem[];
            currentRange = props.range;
            currentEditor = props.editor;
            currentQuery = props.query;
            currentClientRect = props.clientRect ?? null;
            if (selectedIndex >= currentItems.length) selectedIndex = 0;
            recompute();
          },
          onKeyDown: (props: any) => {
            if (props.event.key === "ArrowDown") {
              if (currentItems.length === 0) return false;
              selectedIndex = (selectedIndex + 1) % currentItems.length;
              recompute();
              return true;
            }
            if (props.event.key === "ArrowUp") {
              if (currentItems.length === 0) return false;
              selectedIndex = (selectedIndex - 1 + currentItems.length) % currentItems.length;
              recompute();
              return true;
            }
            if (props.event.key === "Enter") {
              pickAt(selectedIndex);
              return true;
            }
            if (props.event.key === "Escape") {
              opts.onStateChange(null);
              return true;
            }
            return false;
          },
          onExit: () => {
            opts.onStateChange(null);
          },
        };
      },
    },
  });
}
