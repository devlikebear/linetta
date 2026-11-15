import { describe, expect, it } from "vitest";
import { forceLayout } from "./forceLayout";

const base = {
  ids: ["a", "b", "c", "d"],
  edges: [{ from: "a", to: "b" }, { from: "b", to: "c" }],
  width: 600,
  height: 400,
};

describe("forceLayout", () => {
  it("lays the same graph out the same way every time, inside the box", () => {
    const one = forceLayout(base);
    expect(forceLayout(base)).toEqual(one);
    for (const p of Object.values(one)) {
      expect(p.x).toBeGreaterThanOrEqual(28);
      expect(p.x).toBeLessThanOrEqual(572);
      expect(p.y).toBeGreaterThanOrEqual(28);
      expect(p.y).toBeLessThanOrEqual(372);
    }
  });

  it("draws tied nodes closer than untied ones and keeps fixed positions", () => {
    const pos = forceLayout(base);
    const dist = (a: string, b: string) => Math.hypot(pos[a].x - pos[b].x, pos[a].y - pos[b].y);
    expect(dist("a", "b")).toBeLessThan(dist("a", "d"));
    const pinned = forceLayout({ ...base, fixed: { a: { x: 50, y: 60 } } });
    expect(pinned.a).toEqual({ x: 50, y: 60 });
  });
});
