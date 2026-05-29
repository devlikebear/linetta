import type { Editor } from "@tiptap/react";

export type CommitMode = "replace" | "insert" | "replaceAll";

export interface CommitTarget {
  /** Frozen selection captured at Cmd+I time. */
  from: number;
  to: number;
}

/** A ProseMirror paragraph node holding plain inline content (text + hardBreaks). */
interface ParagraphNode {
  type: "paragraph";
  content?: InlineNode[];
}
type InlineNode = { type: "text"; text: string } | { type: "hardBreak" };

/**
 * Convert plain text into an array of ProseMirror paragraph nodes.
 * - Blank lines (\n\n+) separate paragraphs.
 * - Single newlines within a paragraph become hardBreak nodes.
 * - No marks are applied (plain text commit).
 * An empty paragraph (no content) is represented as { type: "paragraph" }.
 */
export function textToParagraphs(text: string): ParagraphNode[] {
  const normalized = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  const blocks = normalized.split(/\n{2,}/);
  const paragraphs: ParagraphNode[] = [];
  for (const block of blocks) {
    const lines = block.split("\n");
    const content: InlineNode[] = [];
    lines.forEach((line, i) => {
      if (i > 0) content.push({ type: "hardBreak" });
      if (line.length > 0) content.push({ type: "text", text: line });
    });
    paragraphs.push(content.length > 0 ? { type: "paragraph", content } : { type: "paragraph" });
  }
  if (paragraphs.length === 0) paragraphs.push({ type: "paragraph" });
  return paragraphs;
}

/**
 * Commit generated text into the editor as plain text (no mark inheritance).
 * - replaceAll: replace the entire document with the paragraphs (single undo step).
 * - replace: replace [target.from, target.to] with the paragraphs.
 * - insert: insert the paragraphs at target.from.
 */
export function commitGenerated(
  editor: Editor,
  mode: CommitMode,
  target: CommitTarget,
  text: string,
): void {
  const paragraphs = textToParagraphs(text);
  if (mode === "replaceAll") {
    editor.chain().setContent({ type: "doc", content: paragraphs }).run();
    return;
  }
  if (mode === "replace") {
    editor.chain().insertContentAt({ from: target.from, to: target.to }, paragraphs).run();
    return;
  }
  // insert
  editor.chain().insertContentAt(target.from, paragraphs).run();
}
