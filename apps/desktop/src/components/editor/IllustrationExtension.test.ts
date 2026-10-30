import { Editor } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("../../lib/rpc", () => ({
  illustrations: { get: () => new Promise(() => {}) },
}));

import { IllustrationExtension } from "./IllustrationExtension";

const doc = {
  type: "doc",
  content: [
    { type: "paragraph", content: [{ type: "text", text: "비가 내렸다." }] },
    { type: "illustration", attrs: { id: "pic-1", caption: "우산" } },
    { type: "paragraph", content: [{ type: "text", text: "그는 걸었다." }] },
  ],
};

let editor: Editor | null = null;
afterEach(() => {
  editor?.destroy();
  editor = null;
});

function make(content: object | string, withNode = true) {
  editor = new Editor({
    extensions: [StarterKit, ...(withNode ? [IllustrationExtension] : [])],
    content,
  });
  return editor;
}

describe("IllustrationExtension", () => {
  it("keeps a picture through load and save, between the same paragraphs", () => {
    expect(make(doc).getJSON()).toEqual(doc);
  });

  it("inserts a picture at the selection as its own block", () => {
    const e = make({ type: "doc", content: [{ type: "paragraph", content: [{ type: "text", text: "첫 문단" }] }] });
    e.commands.setTextSelection(e.state.doc.content.size);
    e.commands.insertIllustration("pic-2", "표지");
    const blocks = (e.getJSON().content ?? []).map((b) => b.type);
    expect(blocks).toContain("illustration");
    expect(e.getJSON().content?.find((b) => b.type === "illustration")?.attrs).toEqual({ id: "pic-2", caption: "표지" });
  });

  it("reads a pasted figure back as the node", () => {
    const e = make('<p>a</p><figure data-illustration-id="pic-3" data-caption="밤"></figure>');
    expect(e.getJSON().content?.[1]).toEqual({ type: "illustration", attrs: { id: "pic-3", caption: "밤" } });
  });

  // Why the node is in every editor's base set: without it the editor drops
  // the picture on load, and the next save takes it out of the scene.
  it("is what keeps the picture: an editor without it loses the node", () => {
    const without = make(doc, false).getJSON();
    expect(JSON.stringify(without)).not.toContain("pic-1");
  });
});
