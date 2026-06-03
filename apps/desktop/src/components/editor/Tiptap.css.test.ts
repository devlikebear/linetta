import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "../..");

async function readSource(path: string) {
  return readFile(resolve(srcRoot, path), "utf8");
}

describe("Tiptap editor CSS", () => {
  it("keeps the empty paragraph caret visible", async () => {
    const css = await readSource("components/editor/Tiptap.css");

    expect(css).toContain("caret-color: var(--ink)");
    expect(css).toContain(".tiptap-editor .ProseMirror p");
    expect(css).toContain("min-height: 1.92em");
    expect(css).toContain(".tiptap-wrap.empty-focused .tiptap-editor .ProseMirror");
    expect(css).toContain("caret-color: transparent");
    expect(css).toContain("@keyframes linetta-caret-blink");
  });

  it("does not apply first-letter styling inside ZEN mode", async () => {
    const css = await readSource("App.css");

    expect(css).not.toContain(".zen-col .prose p:first-of-type::first-letter");
  });
});
