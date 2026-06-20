import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useIdleTimer } from "./useIdleTimer";

describe("useIdleTimer", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("fires onIdle after idleMs of no activity", () => {
    const onIdle = vi.fn();
    const { result } = renderHook(() => useIdleTimer(2000, onIdle));
    act(() => result.current.markActivity());
    act(() => vi.advanceTimersByTime(1999));
    expect(onIdle).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    expect(onIdle).toHaveBeenCalledTimes(1);
  });

  it("resets the timer on each markActivity", () => {
    const onIdle = vi.fn();
    const { result } = renderHook(() => useIdleTimer(2000, onIdle));
    act(() => result.current.markActivity());
    act(() => vi.advanceTimersByTime(1500));
    act(() => result.current.markActivity());
    act(() => vi.advanceTimersByTime(1500));
    expect(onIdle).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(500));
    expect(onIdle).toHaveBeenCalledTimes(1);
  });

  it("cancel prevents a pending fire", () => {
    const onIdle = vi.fn();
    const { result } = renderHook(() => useIdleTimer(2000, onIdle));
    act(() => result.current.markActivity());
    act(() => result.current.cancel());
    act(() => vi.advanceTimersByTime(3000));
    expect(onIdle).not.toHaveBeenCalled();
  });
});
