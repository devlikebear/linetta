import { describe, expect, it } from "vitest";
import { X_LIMIT, xLength } from "./xLength";

describe("xLength", () => {
  it("counts latin characters as 1", () => {
    expect(xLength("hello, world")).toBe(12);
  });

  it("counts Hangul, CJK, Japanese and fullwidth as 2", () => {
    expect(xLength("안녕")).toBe(4);
    expect(xLength("漢字")).toBe(4);
    expect(xLength("ひらがな")).toBe(8);
    expect(xLength("ＡＢ")).toBe(4);
  });

  it("counts mixed text", () => {
    expect(xLength("글 abc")).toBe(2 + 1 + 3);
  });

  it("counts every URL as 23 regardless of length", () => {
    expect(xLength("https://example.com/a/very/long/path?with=query")).toBe(23);
    expect(xLength("a https://x.co b http://y.com/z")).toBe(2 + 23 + 3 + 23);
  });

  it("counts an emoji as one 2-weight character, not per UTF-16 unit", () => {
    expect(xLength("🚀")).toBe(2);
    expect(xLength("👨‍👩‍👧")).toBe(2 * 3 + 1 * 2);
  });

  it("is over the limit at 141 Hangul characters", () => {
    expect(xLength("가".repeat(140))).toBe(X_LIMIT);
    expect(xLength("가".repeat(141))).toBeGreaterThan(X_LIMIT);
  });

  it("is zero for empty text", () => {
    expect(xLength("")).toBe(0);
  });
});
