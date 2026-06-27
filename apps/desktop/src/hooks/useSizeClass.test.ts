import { describe, expect, it } from "vitest";
import { DESKTOP_QUERY, IPAD_QUERY, resolveSizeClass } from "./useSizeClass";

describe("resolveSizeClass", () => {
  it("keeps touch-capable iPad widths in the iPad tier", () => {
    expect(IPAD_QUERY).toContain("(max-width: 1366px)");
    expect(IPAD_QUERY).toContain("(any-pointer: coarse)");
    expect(DESKTOP_QUERY).toContain("(min-width: 1367px)");
    expect(DESKTOP_QUERY).toContain("(not (any-pointer: coarse))");
  });

  it("prefers desktop when desktop query matches", () => {
    expect(resolveSizeClass({ desktop: true, ipad: false })).toBe("desktop");
    // A very wide external display can match both; desktop wins at that point.
    expect(resolveSizeClass({ desktop: true, ipad: true })).toBe("desktop");
  });

  it("returns ipad when only the ipad query matches", () => {
    expect(resolveSizeClass({ desktop: false, ipad: true })).toBe("ipad");
  });

  it("falls back to compact when neither matches", () => {
    expect(resolveSizeClass({ desktop: false, ipad: false })).toBe("compact");
  });
});
