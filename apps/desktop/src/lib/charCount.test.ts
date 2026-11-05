import { describe, expect, it } from "vitest";
import { charCountBasisOf, countDocChars, countTextChars } from "./charCount";

describe("charCount", () => {
  it("counts text with and without spaces the way the engine does", () => {
    expect(countTextChars("안녕 하세요", "with_spaces")).toBe(6);
    expect(countTextChars("안녕 하세요", "without_spaces")).toBe(5);
    // Tabs, newlines, ideographic and no-break spaces are all whitespace.
    expect(countTextChars(" a\tb\nc　d ", "without_spaces")).toBe(4);
    // An emoji is one character, not two UTF-16 units.
    expect(countTextChars("🙂 x", "with_spaces")).toBe(3);
  });

  it("counts a Tiptap doc's text nodes under the basis", () => {
    const doc = {
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "가 나" }] },
        { type: "paragraph", content: [{ type: "text", text: "다 라 마" }] },
      ],
    };
    expect(countDocChars(doc, "with_spaces")).toBe(8);
    expect(countDocChars(doc, "without_spaces")).toBe(5);
  });

  it("reads a missing basis as with spaces", () => {
    expect(charCountBasisOf({})).toBe("with_spaces");
    expect(charCountBasisOf(null)).toBe("with_spaces");
    expect(charCountBasisOf({ char_count_basis: "without_spaces" })).toBe("without_spaces");
  });
});
