import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { getSchema } from "@tiptap/core";
import type { Node as PMNode } from "@tiptap/pm/model";
import StarterKit from "@tiptap/starter-kit";
import { describe, expect, it } from "vitest";
import { buildMentionExtension } from "../components/editor/MentionExtension";
import { IllustrationExtension } from "../components/editor/IllustrationExtension";
import { sceneEditorExtensions } from "../components/editor/sceneExtensions";
import { findQuoteInParagraphs, normalizeQuote, paragraphRanges } from "./paragraphRanges";

// The engine's style.Paragraphs is tested against these same files
// (TestParagraphsMatchTheSharedFixtures): a citation's paragraph number has
// to land on the paragraph the engine meant.
const fixtureDir = resolve(__dirname, "../../../../engine/internal/style/testdata/paragraphs");

interface Fixture {
  doc: object;
  paragraphs: string[];
  quotes?: { from: number; to: number; quote: string; found: boolean }[];
}

const schema = getSchema([
  // The same set Tiptap.tsx mounts for every scene editor.
  StarterKit.configure({}),
  IllustrationExtension,
  ...sceneEditorExtensions(buildMentionExtension({ search: async () => [], onStateChange: () => {} })),
]);

// What a reader sees between two positions: a mention as its label, a hard
// break as a newline, other atoms as nothing.
function visibleText(doc: PMNode, from: number, to: number): string {
  return doc.textBetween(from, to, "\n", (leaf) => {
    if (leaf.type.name === "mention") return String(leaf.attrs.label ?? "");
    if (leaf.type.name === "hardBreak") return "\n";
    return "";
  });
}

const files = readdirSync(fixtureDir).filter((f) => f.endsWith(".json")).sort();

describe("paragraphRanges", () => {
  it("finds the shared fixtures", () => {
    expect(files.length).toBeGreaterThan(0);
  });

  for (const file of files) {
    const fixture = JSON.parse(readFileSync(resolve(fixtureDir, file), "utf8")) as Fixture;
    const doc = schema.nodeFromJSON(fixture.doc);

    it(`${file}: numbers paragraphs like style.Paragraphs`, () => {
      const ranges = paragraphRanges(doc);
      expect(ranges.map((r) => r.text)).toEqual(fixture.paragraphs);
      expect(ranges.map((r) => r.n)).toEqual(fixture.paragraphs.map((_, i) => i + 1));
      for (const r of ranges) {
        expect(r.from).toBeLessThan(r.to);
        // The span covers the paragraph's visible text, edge newlines aside.
        expect(normalizeQuote(visibleText(doc, r.from, r.to))).toBe(normalizeQuote(r.text));
      }
    });

    for (const q of fixture.quotes ?? []) {
      it(`${file}: quote ${JSON.stringify(q.quote)} in ¶${q.from}-${q.to} found=${q.found}`, () => {
        const match = findQuoteInParagraphs(paragraphRanges(doc), q.from, q.to, q.quote);
        if (!q.found) {
          expect(match).toBeNull();
          return;
        }
        expect(match).not.toBeNull();
        expect(normalizeQuote(visibleText(doc, match!.from, match!.to))).toBe(normalizeQuote(q.quote));
      });
    }
  }
});

describe("findQuoteInParagraphs", () => {
  const doc = schema.nodeFromJSON({
    type: "doc",
    content: [
      { type: "paragraph", content: [{ type: "text", text: "첫 문단." }] },
      {
        type: "paragraph",
        content: [
          { type: "text", text: "수아는 " },
          { type: "text", marks: [{ type: "bold" }], text: "은빛" },
          { type: "text", text: " 열쇠를 쥐었다." },
        ],
      },
    ],
  });
  const ranges = paragraphRanges(doc);

  it("lands on the quoted characters across mark boundaries", () => {
    const match = findQuoteInParagraphs(ranges, 2, 2, "은빛 열쇠");
    expect(match).not.toBeNull();
    expect(doc.textBetween(match!.from, match!.to)).toBe("은빛 열쇠");
  });

  it("rejects ranges the scene does not have", () => {
    expect(findQuoteInParagraphs(ranges, 0, 1, "첫")).toBeNull();
    expect(findQuoteInParagraphs(ranges, 2, 1, "첫")).toBeNull();
    expect(findQuoteInParagraphs(ranges, 2, 3, "은빛")).toBeNull();
  });

  it("finds nothing for an empty quote", () => {
    expect(findQuoteInParagraphs(ranges, 1, 2, "   ")).toBeNull();
  });
});
