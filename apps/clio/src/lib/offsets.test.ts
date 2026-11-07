import { describe, expect, it } from "vitest";
import { codePointIndex, cpToUtf16, findingRange, isRanged } from "./offsets";

describe("offsets", () => {
  it("is the identity for BMP text", () => {
    expect(codePointIndex("안녕")).toEqual([0, 1, 2]);
    expect(findingRange("안녕하세요", { start: 1, end: 3 })).toEqual({ start: 1, end: 3 });
  });

  it("shifts past surrogate pairs", () => {
    // "🚀" is 1 code point, 2 UTF-16 units.
    const text = "🚀 결론적으로";
    expect(codePointIndex(text).slice(0, 4)).toEqual([0, 2, 3, 4]);
    // code points 2..5 are "결론적" -> UTF-16 3..6
    expect(findingRange(text, { start: 2, end: 5 })).toEqual({ start: 3, end: 6 });
    expect(text.slice(3, 6)).toBe("결론적");
  });

  it("handles a range that covers an emoji", () => {
    const text = "a🚀b👨‍👩‍👧c";
    const r = findingRange(text, { start: 1, end: 2 });
    expect(r).toEqual({ start: 1, end: 3 });
    expect(text.slice(r!.start, r!.end)).toBe("🚀");
  });

  it("clamps offsets beyond the text", () => {
    expect(cpToUtf16("🚀a", 99)).toBe(3);
    expect(findingRange("ab", { start: 1, end: 50 })).toEqual({ start: 1, end: 2 });
    expect(findingRange("ab", { start: 5, end: 9 })).toBeNull();
  });

  it("treats 0,0 as whole-post", () => {
    expect(isRanged({ start: 0, end: 0 })).toBe(false);
    expect(findingRange("abc", { start: 0, end: 0 })).toBeNull();
    expect(isRanged({ start: 0, end: 2 })).toBe(true);
  });
});
