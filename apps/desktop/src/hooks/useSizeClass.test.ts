import { describe, expect, it } from "vitest";
import { resolveSizeClass } from "./useSizeClass";

describe("resolveSizeClass", () => {
  it("prefers desktop when desktop query matches", () => {
    expect(resolveSizeClass({ desktop: true, ipad: false })).toBe("desktop");
    // a 12.9" iPad in landscape matches BOTH (min-width:1181) and coarse → desktop wins
    expect(resolveSizeClass({ desktop: true, ipad: true })).toBe("desktop");
  });

  it("returns ipad when only the ipad query matches", () => {
    expect(resolveSizeClass({ desktop: false, ipad: true })).toBe("ipad");
  });

  it("falls back to compact when neither matches", () => {
    expect(resolveSizeClass({ desktop: false, ipad: false })).toBe("compact");
  });
});
