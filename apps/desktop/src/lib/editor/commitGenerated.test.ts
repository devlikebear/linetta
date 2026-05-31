import { describe, expect, it, vi } from "vitest";
import { commitGenerated, textToParagraphs } from "./commitGenerated";

function fakeEditor() {
  const chain = {
    setContent: vi.fn(() => chain),
    insertContentAt: vi.fn(() => chain),
    run: vi.fn(() => true),
  };
  return {
    chain: vi.fn(() => chain),
    calls: chain,
  };
}

describe("textToParagraphs", () => {
  it("maps blank lines to paragraphs and single newlines to hard breaks", () => {
    expect(textToParagraphs("A\nB\n\nC")).toEqual([
      {
        type: "paragraph",
        content: [
          { type: "text", text: "A" },
          { type: "hardBreak" },
          { type: "text", text: "B" },
        ],
      },
      { type: "paragraph", content: [{ type: "text", text: "C" }] },
    ]);
  });
});

describe("commitGenerated", () => {
  it("replaces the whole document for replaceAll", () => {
    const editor = fakeEditor();
    commitGenerated(editor as never, "replaceAll", { from: 1, to: 5 }, "Hello");

    expect(editor.calls.setContent).toHaveBeenCalledWith({
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text: "Hello" }] }],
    });
    expect(editor.calls.run).toHaveBeenCalled();
  });

  it("uses frozen ranges for replace and insert modes", () => {
    const replaceEditor = fakeEditor();
    commitGenerated(replaceEditor as never, "replace", { from: 2, to: 8 }, "New");
    expect(replaceEditor.calls.insertContentAt).toHaveBeenCalledWith(
      { from: 2, to: 8 },
      [{ type: "paragraph", content: [{ type: "text", text: "New" }] }],
    );

    const insertEditor = fakeEditor();
    commitGenerated(insertEditor as never, "insert", { from: 4, to: 4 }, "More");
    expect(insertEditor.calls.insertContentAt).toHaveBeenCalledWith(
      4,
      [{ type: "paragraph", content: [{ type: "text", text: "More" }] }],
    );
  });
});
