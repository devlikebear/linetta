import { describe, expect, it } from "vitest";

import { readSource } from "../../test/readSource";

describe("Tiptap editor CSS", () => {
  it("keeps the empty paragraph caret visible", async () => {
    const css = await readSource("components/editor/Tiptap.css");

    expect(css).toContain("caret-color: var(--ink)");
    expect(css).toContain(".tiptap-editor .ProseMirror p");
    expect(css).toContain("min-height: calc(var(--edit-leading) * 1em)");
    expect(css).toContain(".tiptap-wrap.empty-focused .tiptap-editor .ProseMirror");
    expect(css).toContain("caret-color: transparent");
    expect(css).toContain("@keyframes linetta-caret-blink");
  });

  it("does not apply first-letter styling inside ZEN mode", async () => {
    const css = await readSource("App.css");

    expect(css).not.toContain(".zen-col .prose p:first-of-type::first-letter");
  });
});
