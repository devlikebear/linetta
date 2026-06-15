import { act, fireEvent, render, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { TiptapEditor, type TiptapHandle } from "./Tiptap";

const emptyDoc = {
  type: "doc",
  content: [{ type: "paragraph" }],
};

const textDoc = {
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text: "비 온 뒤 흙냄새가 났다." }] }],
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

  it("emits selected text details from the editor context menu", async () => {
    const ref = createRef<TiptapHandle>();
    const onSelectionContextMenu = vi.fn();
    const { container } = render(
      <TiptapEditor
        ref={ref}
        initialDoc={textDoc}
        onChange={vi.fn()}
        onSelectionContextMenu={onSelectionContextMenu}
      />,
    );
    await waitFor(() => expect(ref.current?.editor).toBeTruthy());

    act(() => {
      ref.current?.setSelection({ from: 1, to: 8 });
    });
    fireEvent.contextMenu(container.querySelector(".ProseMirror")!);

    expect(onSelectionContextMenu).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ from: 1, to: 8, text: expect.stringContaining("비 온 뒤") }),
    );
  });

  it("does not emit the editor context menu for an empty selection", async () => {
    const ref = createRef<TiptapHandle>();
    const onSelectionContextMenu = vi.fn();
    const { container } = render(
      <TiptapEditor
        ref={ref}
        initialDoc={textDoc}
        onChange={vi.fn()}
        onSelectionContextMenu={onSelectionContextMenu}
      />,
    );
    await waitFor(() => expect(ref.current?.editor).toBeTruthy());

    act(() => {
      ref.current?.setSelection({ from: 1, to: 1 });
    });
    fireEvent.contextMenu(container.querySelector(".ProseMirror")!);

    expect(onSelectionContextMenu).not.toHaveBeenCalled();
  });

  it("finds and navigates text matches in the current scene", async () => {
    const ref = createRef<TiptapHandle>();
    render(<TiptapEditor ref={ref} initialDoc={{
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text: "민호는 웃었다. 민호는 고개를 들었다." }] }],
    }} onChange={vi.fn()} />);
    await waitFor(() => expect(ref.current?.editor).toBeTruthy());

    let result: { count: number; activeIndex: number } | undefined;
    act(() => {
      result = ref.current?.findText("민호");
    });
    expect(result).toEqual({ count: 2, activeIndex: 0 });
    expect(ref.current?.getSelection()).toEqual({ from: 1, to: 3 });

    act(() => {
      ref.current?.nextMatch();
    });
    expect(ref.current?.getSelection()).toEqual({ from: 10, to: 12 });

    act(() => {
      ref.current?.prevMatch();
    });
    expect(ref.current?.getSelection()).toEqual({ from: 1, to: 3 });
  });

  it("replaces the active match and all matches in the current scene", async () => {
    const ref = createRef<TiptapHandle>();
    const onChange = vi.fn();
    render(<TiptapEditor ref={ref} initialDoc={{
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text: "민호는 민호의 시계를 보았다." }] }],
    }} onChange={onChange} />);
    await waitFor(() => expect(ref.current?.editor).toBeTruthy());

    act(() => {
      ref.current?.findText("민호");
      ref.current?.replaceActiveMatch("민준");
    });
    await waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(ref.current?.editor?.state.doc.textContent).toBe("민준는 민호의 시계를 보았다.");

    act(() => {
      ref.current?.replaceAllMatches("민호", "민준");
    });
    await waitFor(() => expect(ref.current?.editor?.state.doc.textContent).toBe("민준는 민준의 시계를 보았다."));
  });
});
