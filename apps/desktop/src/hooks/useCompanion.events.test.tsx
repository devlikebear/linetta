import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Capture the Tauri event listeners so the test can fire engine events.
const ev = vi.hoisted(() => ({ listeners: new Map<string, (e: { payload: unknown }) => void>() }));
vi.mock("@tauri-apps/api/event", () => ({
  listen: (event: string, cb: (e: { payload: unknown }) => void) => {
    ev.listeners.set(event, cb);
    return Promise.resolve(() => ev.listeners.delete(event));
  },
}));

const rpc = vi.hoisted(() => ({ history: vi.fn(), send: vi.fn() }));
vi.mock("../lib/rpc", () => ({
  companion: { history: rpc.history, send: rpc.send },
}));

import { useCompanion } from "./useCompanion";

function fire(event: string, payload: unknown) {
  const cb = ev.listeners.get(event);
  if (!cb) throw new Error(`no listener registered for ${event}`);
  act(() => cb({ payload }));
}

describe("useCompanion streaming", () => {
  beforeEach(() => {
    ev.listeners.clear();
    rpc.history.mockResolvedValue([]);
    rpc.send.mockResolvedValue({ run_id: "r1" });
  });

  it("accumulates companion-delta into streaming live, then finalizes on done", async () => {
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(ev.listeners.has("companion-delta")).toBe(true));

    await act(async () => {
      await result.current.send("안녕하세요");
    });
    expect(result.current.status).toBe("streaming");

    fire("companion-delta", { run_id: "r1", text: "안" });
    fire("companion-delta", { run_id: "r1", text: "녕" });
    fire("companion-delta", { run_id: "r1", text: "하세요" });
    expect(result.current.streaming).toBe("안녕하세요");

    fire("companion-done", { run_id: "r1", full_text: "안녕하세요! 반가워요." });
    expect(result.current.streaming).toBe("");
    const msgs = result.current.messages;
    expect(msgs[msgs.length - 1]).toMatchObject({
      role: "assistant",
      content: "안녕하세요! 반가워요.",
    });
  });
});
