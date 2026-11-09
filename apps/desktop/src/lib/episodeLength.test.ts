import { describe, expect, it } from "vitest";
import { episodeLength } from "./episodeLength";

const at = (min: number, target: number, max: number) => ({
  episode_char_min: min,
  episode_char_target: target,
  episode_char_max: max,
});

describe("episodeLength", () => {
  it("measures progress toward the target when the work sets no minimum", () => {
    expect(episodeLength(3925, at(0, 5000, 0))).toEqual({ state: "progress", delta: 0, goal: 5000, complete: false });
    expect(episodeLength(5000, at(0, 5000, 0))).toEqual({ state: "progress", delta: 0, goal: 5000, complete: true });
  });

  it("calls an episode under the minimum short, by how much", () => {
    expect(episodeLength(3925, at(5000, 5300, 0))).toEqual({ state: "short", delta: -1075, goal: 5000, complete: false });
    expect(episodeLength(0, at(5000, 5300, 0)).state).toBe("short");
  });

  it("counts reaching either bound exactly as within it", () => {
    expect(episodeLength(5000, at(5000, 5300, 0))).toEqual({ state: "ok", delta: 0, goal: 5000, complete: true });
    expect(episodeLength(5500, at(4500, 5000, 5500)).state).toBe("ok");
  });

  it("calls an episode past the maximum long, by how much", () => {
    expect(episodeLength(5820, at(4500, 5000, 5500))).toEqual({ state: "long", delta: 320, goal: 4500, complete: true });
    expect(episodeLength(5820, at(0, 5000, 5500))).toMatchObject({ state: "long", delta: 320, goal: 5000 });
  });
});
