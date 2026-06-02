import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextReveal, useSmoothStream } from "./useSmoothStream";

describe("nextReveal", () => {
  it("advances partway toward the target (min 2 chars)", () => {
    expect(nextReveal("ab", "abcdefghij")).toBe("abcd"); // remaining 8 → step max(2, ceil(1.44)) = 2
    expect(nextReveal("", "abc")).toBe("ab"); // remaining 3 → step 2, not yet complete
  });

  it("reveals a fraction of a large remaining gap", () => {
    // remaining 100 → step ceil(18) = 18
    expect(nextReveal("", "x".repeat(100))).toBe("x".repeat(18));
  });

  it("returns the target unchanged once caught up", () => {
    expect(nextReveal("abc", "abc")).toBe("abc");
  });

  it("snaps to target when it diverges (stream reset)", () => {
    expect(nextReveal("old long text", "")).toBe("");
    expect(nextReveal("abc", "xyz")).toBe("xyz");
  });
});

describe("useSmoothStream", () => {
  let frames: FrameRequestCallback[] = [];
  beforeEach(() => {
    frames = [];
    vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
      frames.push(cb);
      return frames.length;
    });
    vi.stubGlobal("cancelAnimationFrame", () => {});
  });
  afterEach(() => vi.unstubAllGlobals());

  const flush = (n: number) => {
    for (let i = 0; i < n; i++) {
      const cbs = frames;
      frames = [];
      act(() => cbs.forEach((cb) => cb(0)));
    }
  };

  it("converges to the target over frames while active", () => {
    const { result } = renderHook(() => useSmoothStream("안녕하세요 반갑습니다", true));
    flush(40);
    expect(result.current).toBe("안녕하세요 반갑습니다");
  });

  it("returns the full target immediately when inactive", () => {
    const { result } = renderHook(() => useSmoothStream("완성된 텍스트", false));
    expect(result.current).toBe("완성된 텍스트");
  });
});
