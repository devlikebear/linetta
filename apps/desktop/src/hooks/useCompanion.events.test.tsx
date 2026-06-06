import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_AI_CONTEXT_SELECTION } from "../components/ai/AIContextChecklist";

// Capture the Tauri event listeners so the test can fire engine events.
const ev = vi.hoisted(() => ({ listeners: new Map<string, (e: { payload: unknown }) => void>() }));
vi.mock("@tauri-apps/api/event", () => ({
  listen: (event: string, cb: (e: { payload: unknown }) => void) => {
    ev.listeners.set(event, cb);
    return Promise.resolve(() => ev.listeners.delete(event));
  },
}));

const rpc = vi.hoisted(() => ({ history: vi.fn(), send: vi.fn(), cancel: vi.fn(), clear: vi.fn(), compact: vi.fn() }));
vi.mock("../lib/rpc", () => ({
  companion: { history: rpc.history, send: rpc.send, cancel: rpc.cancel, clear: rpc.clear, compact: rpc.compact },
}));

import { useCompanion } from "./useCompanion";

function fire(event: string, payload: unknown) {
  const cb = ev.listeners.get(event);
  if (!cb) throw new Error(`no listener registered for ${event}`);
  act(() => cb({ payload }));
}

describe("useCompanion streaming", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.history.mockResolvedValue([]);
    rpc.send.mockResolvedValue({ run_id: "r1" });
    rpc.cancel.mockResolvedValue({ ok: true });
    rpc.clear.mockResolvedValue({ ok: true });
    rpc.compact.mockResolvedValue([]);
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

  it("passes the selected context sections when sending a companion turn", async () => {
    const selection = { ...DEFAULT_AI_CONTEXT_SELECTION, current_scene: false, memories: false };
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }, undefined, selection));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      await result.current.send("본문은 빼고 봐줘");
    });

    expect(rpc.send).toHaveBeenCalledWith("p1", "n1", "본문은 빼고 봐줘", { context: selection });
  });

  it("passes companion image attachments with the selected context", async () => {
    const selection = { ...DEFAULT_AI_CONTEXT_SELECTION, current_scene: false };
    const image = {
      name: "scene.png",
      media_type: "image/png",
      data: "AQID",
      size: 3,
    };
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }, undefined, selection));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      await result.current.send("이미지 참고해줘", [image]);
    });

    expect(rpc.send).toHaveBeenCalledWith("p1", "n1", "이미지 참고해줘", { context: selection, images: [image] });
  });

  it("passes the selected outline structure to companion turns", async () => {
    const selection = { ...DEFAULT_AI_CONTEXT_SELECTION };
    const outlineStructure = "웹소설: 권 > 화 > 씬 (예: 1권 > 1화 > 씬 1)";
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }, undefined, selection, outlineStructure));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      await result.current.send("아웃라인 작성해줘");
    });

    expect(rpc.send).toHaveBeenCalledWith("p1", "n1", "아웃라인 작성해줘", {
      context: selection,
      outline_structure: outlineStructure,
    });
  });

  it("accepts fast engine events that arrive before companion.send resolves", async () => {
    let resolveSend: (value: { run_id: string }) => void = () => {};
    rpc.send.mockReturnValue(new Promise((resolve) => { resolveSend = resolve; }));
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      void result.current.send("빠른 응답");
    });

    fire("companion-delta", { run_id: "r-fast", text: "이미 " });
    fire("companion-done", { run_id: "r-fast", full_text: "이미 끝났어요." });
    expect(result.current.status).toBe("idle");
    expect(result.current.messages[result.current.messages.length - 1]).toMatchObject({ role: "assistant", content: "이미 끝났어요." });

    await act(async () => {
      resolveSend({ run_id: "r-fast" });
    });
    expect(result.current.status).toBe("idle");
  });

  it("accumulates companion-reasoning and clears it on done", async () => {
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(ev.listeners.has("companion-reasoning")).toBe(true));

    await act(async () => {
      await result.current.send("개요 써줘");
    });

    fire("companion-reasoning", { run_id: "r1", text: "먼저 " });
    fire("companion-reasoning", { run_id: "r1", text: "구조를 잡는다" });
    expect(result.current.reasoning).toBe("먼저 구조를 잡는다");

    fire("companion-done", { run_id: "r1", full_text: "개요입니다." });
    expect(result.current.reasoning).toBe("");
  });

  it("notifies the workspace when apply-ops changes the project", async () => {
    const onApplied = vi.fn();
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }, onApplied));
    await waitFor(() => expect(ev.listeners.has("companion-applied")).toBe(true));

    await act(async () => {
      await result.current.send("아웃라인 수정해줘");
    });

    fire("companion-applied", { run_id: "other", summary: "다른 실행", applied: 1 });
    expect(onApplied).not.toHaveBeenCalled();

    fire("companion-applied", { run_id: "r1", summary: "아웃라인 수정", applied: 1 });
    expect(onApplied).toHaveBeenCalledOnce();
  });

  it("clears transcript through rpc and local state", async () => {
    rpc.history.mockResolvedValue([{ role: "user", content: "남은 대화", timestamp: 1 }]);
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(result.current.messages).toHaveLength(1));

    await act(async () => {
      await result.current.clear();
    });

    expect(rpc.clear).toHaveBeenCalledWith("p1");
    expect(result.current.messages).toEqual([]);
    expect(result.current.status).toBe("idle");
  });

  it("replaces local transcript with compacted history", async () => {
    rpc.history.mockResolvedValue([
      { role: "user", content: "긴 질문", timestamp: 1 },
      { role: "assistant", content: "긴 응답", timestamp: 2 },
    ]);
    rpc.compact.mockResolvedValue([
      { role: "assistant", content: "이전 컴패니언 대화 요약\n- 나: 긴 질문", timestamp: 3 },
    ]);
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(result.current.messages).toHaveLength(2));

    await act(async () => {
      await result.current.compact();
    });

    expect(rpc.compact).toHaveBeenCalledWith("p1");
    expect(result.current.messages).toEqual([
      { role: "assistant", content: "이전 컴패니언 대화 요약\n- 나: 긴 질문" },
    ]);
  });
});
