import { render, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TiptapEditor } from "./Tiptap";

const emptyDoc = {
  type: "doc",
  content: [{ type: "paragraph" }],
};

const rect = {
  x: 0,
  y: 0,
  width: 1,
  height: 24,
  top: 0,
  right: 1,
  bottom: 24,
  left: 0,
  toJSON: () => ({}),
} as DOMRect;

function rectList(): DOMRectList {
  const list = [rect] as unknown as DOMRectList;
  Object.defineProperty(list, "item", {
    value: (index: number) => list[index] ?? null,
  });
  return list;
}

describe("TiptapEditor", () => {
  it("marks an empty focused editor so the visible caret affordance can render", async () => {
    const user = userEvent.setup();
    const { container } = render(<TiptapEditor initialDoc={emptyDoc} onChange={vi.fn()} />);

    const wrap = container.querySelector(".tiptap-wrap");
    const editor = container.querySelector(".ProseMirror") as HTMLElement | null;
    expect(editor).toBeTruthy();
    Object.defineProperty(document, "elementFromPoint", {
      value: () => editor,
      configurable: true,
    });
    Object.defineProperty(HTMLElement.prototype, "getClientRects", {
      value: () => rectList(),
      configurable: true,
    });
    Object.defineProperty(Text.prototype, "getClientRects", {
      value: () => rectList(),
      configurable: true,
    });
    Object.defineProperty(Range.prototype, "getClientRects", {
      value: () => rectList(),
      configurable: true,
    });
    Object.defineProperty(Range.prototype, "getBoundingClientRect", {
      value: () => rect,
      configurable: true,
    });
    Object.defineProperty(window, "scrollBy", {
      value: vi.fn(),
      configurable: true,
    });

    await user.click(editor!);

    await waitFor(() => expect(wrap).toHaveClass("empty-focused"));

    await user.type(editor!, "a");

    await waitFor(() => expect(wrap).not.toHaveClass("empty-focused"));
  });
});
