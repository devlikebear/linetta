import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useKeyedDebouncedCallback } from "./useDebouncedCallback";

describe("useKeyedDebouncedCallback", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("keeps pending work isolated by scene key", () => {
    vi.useFakeTimers();
    const save = vi.fn();
    const { result } = renderHook(() => useKeyedDebouncedCallback(save, 800));

    act(() => {
      result.current("scene-a", { text: "A draft" });
      result.current("scene-b", { text: "B draft" });
      vi.advanceTimersByTime(800);
    });

    expect(save).toHaveBeenCalledTimes(2);
    expect(save).toHaveBeenNthCalledWith(1, "scene-a", { text: "A draft" });
    expect(save).toHaveBeenNthCalledWith(2, "scene-b", { text: "B draft" });
  });

  it("only replaces pending work for the same scene", () => {
    vi.useFakeTimers();
    const save = vi.fn();
    const { result } = renderHook(() => useKeyedDebouncedCallback(save, 800));

    act(() => {
      result.current("scene-a", { text: "old" });
      result.current("scene-a", { text: "latest" });
      vi.advanceTimersByTime(800);
    });

    expect(save).toHaveBeenCalledOnce();
    expect(save).toHaveBeenCalledWith("scene-a", { text: "latest" });
  });

  it("flush invokes the pending callback immediately, before the delay elapses", () => {
    vi.useFakeTimers();
    const save = vi.fn();
    const { result } = renderHook(() => useKeyedDebouncedCallback(save, 800));

    act(() => {
      result.current("scene-a", { text: "mid-sentence" });
    });
    expect(save).not.toHaveBeenCalled();

    act(() => {
      result.current.flush("scene-a");
    });
    expect(save).toHaveBeenCalledOnce();
    expect(save).toHaveBeenCalledWith("scene-a", { text: "mid-sentence" });

    // Nothing left pending — advancing time fires nothing more.
    act(() => {
      vi.advanceTimersByTime(800);
    });
    expect(save).toHaveBeenCalledOnce();
  });

  it("unmount flushes pending work instead of dropping it (#104)", () => {
    vi.useFakeTimers();
    const save = vi.fn();
    const { result, unmount } = renderHook(() => useKeyedDebouncedCallback(save, 800));

    act(() => {
      result.current("scene-a", { text: "last keystrokes" });
      vi.advanceTimersByTime(300);
    });
    expect(save).not.toHaveBeenCalled();

    unmount();
    expect(save).toHaveBeenCalledOnce();
    expect(save).toHaveBeenCalledWith("scene-a", { text: "last keystrokes" });

    // No dangling timer left to fire and no leftover to double-fire.
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(save).toHaveBeenCalledOnce();
  });
});
