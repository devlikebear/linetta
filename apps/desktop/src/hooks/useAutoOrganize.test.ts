import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ORGANIZE_IDLE_MS, useAutoOrganize } from "./useAutoOrganize";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

function setup() {
  const organize = vi.fn().mockResolvedValue({});
  const flush = vi.fn().mockResolvedValue(undefined);
  const hook = renderHook((p: { nodeId: string; episodeId: string; projectId?: string }) =>
    useAutoOrganize({ projectId: p.projectId ?? "p1", nodeId: p.nodeId, episodeId: p.episodeId, organize, flush }),
  { initialProps: { nodeId: "s1", episodeId: "ep1" } as { nodeId: string; episodeId: string; projectId?: string } });
  return { organize, flush, hook };
}

describe("useAutoOrganize", () => {
  it("tells the engine about the scene left behind when the episode changes", async () => {
    const { organize, flush, hook } = setup();
    hook.rerender({ nodeId: "s2", episodeId: "ep1" });
    expect(organize).not.toHaveBeenCalled();
    hook.rerender({ nodeId: "s3", episodeId: "ep2" });
    await act(async () => { await Promise.resolve(); });
    expect(flush).toHaveBeenCalled();
    expect(organize).toHaveBeenCalledWith("p1", "s2", "leave");
  });

  it("does not count opening another work as leaving an episode", async () => {
    const { organize, hook } = setup();
    hook.rerender({ nodeId: "x1", episodeId: "other", projectId: "p2" });
    await act(async () => { await Promise.resolve(); });
    expect(organize).not.toHaveBeenCalled();
  });

  it("tells the engine after a long rest that followed typing, once", async () => {
    const { organize, hook } = setup();
    act(() => { vi.advanceTimersByTime(ORGANIZE_IDLE_MS * 2); });
    expect(organize).not.toHaveBeenCalled();
    act(() => { hook.result.current(); });
    await act(async () => { vi.advanceTimersByTime(ORGANIZE_IDLE_MS); await Promise.resolve(); });
    expect(organize).toHaveBeenCalledWith("p1", "s1", "idle");
    act(() => { vi.advanceTimersByTime(ORGANIZE_IDLE_MS * 2); });
    expect(organize).toHaveBeenCalledTimes(1);
  });

  it("lets a refusal go without a word", async () => {
    const { organize, hook } = setup();
    organize.mockRejectedValue(new Error("agent_busy"));
    hook.rerender({ nodeId: "s3", episodeId: "ep2" });
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(organize).toHaveBeenCalled();
  });
});
