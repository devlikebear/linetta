import { act, renderHook as baseRenderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
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

const rpc = vi.hoisted(() => ({ history: vi.fn(), send: vi.fn(), cancel: vi.fn(), clear: vi.fn(), compact: vi.fn(), settingsGet: vi.fn() }));
vi.mock("../lib/rpc", () => ({
  companion: { history: rpc.history, send: rpc.send, cancel: rpc.cancel, clear: rpc.clear, compact: rpc.compact },
  settings: { get: rpc.settingsGet },
}));

import { stripProposalBlock } from "../lib/companionDisplay";
import { I18nProvider } from "../lib/i18n";
import { __resetCompanionSessionStoreForTests, classifyAISetupIssue, useCompanion } from "./useCompanion";

// useCompanion reads the app language via useI18n, so every hook render
// needs the provider. Shadow renderHook to inject it once.
const i18nWrapper = ({ children }: { children: ReactNode }) => <I18nProvider>{children}</I18nProvider>;
const renderHook: typeof baseRenderHook = (cb, options) => baseRenderHook(cb, { wrapper: i18nWrapper, ...options });

function fire(event: string, payload: unknown) {
  const cb = ev.listeners.get(event);
  if (!cb) throw new Error(`no listener registered for ${event}`);
  act(() => cb({ payload }));
}

describe("stripProposalBlock", () => {
  it("removes standalone web tool argument echoes from displayed assistant prose", () => {
    expect(stripProposalBlock([
      '{"count":5,"provider":"brave","query":"blacksmith historical role"}',
      "확인된 출처를 기준으로 처리했어요.",
    ].join("\n"))).toBe("확인된 출처를 기준으로 처리했어요.");
  });

  it("removes a leading web tool argument echo even when prose follows on the same line", () => {
    expect(stripProposalBlock(
      '{"count":5,"provider":"brave","query":"blacksmith historical role"}좋아, 확인된 출처를 기준으로 처리했어요.',
    )).toBe("좋아, 확인된 출처를 기준으로 처리했어요.");
  });
});

describe("classifyAISetupIssue", () => {
  it("classifies missing key, auth, model, and spend-limit provider failures", () => {
    expect(classifyAISetupIssue("api key is required for auth mode api-key")).toBe("missing_key");
    expect(classifyAISetupIssue("401 unauthorized")).toBe("auth_required");
    expect(classifyAISetupIssue("model not found: latest-writing-model")).toBe("model_unavailable");
    expect(classifyAISetupIssue("insufficient credits or spend limit reached")).toBe("rate_or_spend_limit");
    expect(classifyAISetupIssue("openrouter status 402: OpenRouter 크레딧 또는 키 한도가 부족합니다.")).toBe("rate_or_spend_limit");
  });

  it("leaves non-setup companion failures unclassified", () => {
    expect(classifyAISetupIssue("본문 변경이 만들어지지 않았습니다.")).toBeUndefined();
  });
});

describe("useCompanion streaming", () => {
  beforeEach(() => {
    __resetCompanionSessionStoreForTests();
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.settingsGet.mockResolvedValue({ language: "ko" });
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

  it("keeps a running companion response when the panel unmounts before done", async () => {
    const first = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      await first.result.current.send("긴 응답 부탁해");
    });
    expect(first.result.current.status).toBe("streaming");

    first.unmount();

    fire("companion-delta", { project_id: "p1", run_id: "r1", text: "계속 " });
    fire("companion-done", { project_id: "p1", run_id: "r1", full_text: "계속 완성했어요." });

    const second = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => {
      expect(second.result.current.messages[second.result.current.messages.length - 1]).toMatchObject({
        role: "assistant",
        content: "계속 완성했어요.",
      });
    });
    expect(second.result.current.status).toBe("idle");
  });

  it("recovers leaked apply-ops JSON into a proposal when finalizing a turn", async () => {
    const inlineOps = [{
      op: "create_fact_card",
      claim: "운하 갑문 구조",
      result: "갑문은 수위를 맞춰 선박을 이동시키는 구조물이다.",
      status: "verified",
      sources: [{ url: "https://example.com/lock-gate", title: "Lock gate" }],
    }];
    const fullText = `${JSON.stringify({
      summary: "현실 팩트카드 저장",
      ops_json: JSON.stringify(inlineOps),
    })}저장 제안을 확인해 주세요.`;
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      await result.current.send("운하 갑문 구조 확인");
    });
    fire("companion-done", { run_id: "r1", full_text: fullText });

    const message = result.current.messages[result.current.messages.length - 1];
    expect(message.content).toBe("저장 제안을 확인해 주세요.");
    expect(message.proposal).toMatchObject({
      valid: true,
      summary: "현실 팩트카드 저장",
      ops: [expect.objectContaining({ op: "create_fact_card", claim: "운하 갑문 구조" })],
    });
    expect(message.content).not.toContain("ops_json");
    expect(message.content).not.toContain("create_fact_card");
  });

  it("passes the selected context sections when sending a companion turn", async () => {
    const selection = { ...DEFAULT_AI_CONTEXT_SELECTION, current_scene: false, memories: false };
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }, undefined, selection));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      await result.current.send("본문은 빼고 봐줘");
    });

    expect(rpc.send).toHaveBeenCalledWith("p1", "n1", "본문은 빼고 봐줘", { context: selection, language: "ko" });
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

    expect(rpc.send).toHaveBeenCalledWith("p1", "n1", "이미지 참고해줘", { context: selection, images: [image], language: "ko" });
  });

  it("loads persisted transcript separately for each scene scope", async () => {
    rpc.history.mockImplementation((_projectId: string, nodeId?: string | null, scope?: string) => {
      if (scope === "scene" && nodeId === "n1") {
        return Promise.resolve([{ role: "assistant", content: "씬 1 대화", timestamp: 1, node_id: "n1", scope: "scene" }]);
      }
      if (scope === "scene" && nodeId === "n2") {
        return Promise.resolve([{ role: "assistant", content: "씬 2 대화", timestamp: 2, node_id: "n2", scope: "scene" }]);
      }
      return Promise.resolve([]);
    });

    const first = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(first.result.current.messages[0]?.content).toBe("씬 1 대화"));

    const second = renderHook(() => useCompanion("p1", { current: "n2" }));
    await waitFor(() => expect(second.result.current.messages[0]?.content).toBe("씬 2 대화"));

    expect(first.result.current.messages[0]?.content).toBe("씬 1 대화");
    expect(rpc.history).toHaveBeenCalledWith("p1", "n1", "scene");
    expect(rpc.history).toHaveBeenCalledWith("p1", "n2", "scene");
  });

  it("switches from project history to scene history when the current node id becomes available", async () => {
    rpc.history.mockImplementation((_projectId: string, nodeId?: string | null, scope?: string) => {
      if (scope === "scene" && nodeId === "n1") {
        return Promise.resolve([{ role: "assistant", content: "복원된 씬 대화", timestamp: 1, node_id: "n1", scope: "scene" }]);
      }
      return Promise.resolve([]);
    });

    const hook = renderHook(
      ({ nodeId }) => useCompanion("p1", nodeId, undefined, undefined, undefined, "scene"),
      { initialProps: { nodeId: null as string | null } },
    );
    await waitFor(() => expect(rpc.history).toHaveBeenCalledWith("p1", null, "project"));

    hook.rerender({ nodeId: "n1" });

    await waitFor(() => expect(rpc.history).toHaveBeenCalledWith("p1", "n1", "scene"));
    await waitFor(() => expect(hook.result.current.messages[0]?.content).toBe("복원된 씬 대화"));
  });

  it("uses project scope without a node target when requested", async () => {
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }, undefined, undefined, undefined, "project"));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      await result.current.send("작품 전체 플롯 봐줘");
    });

    expect(rpc.history).toHaveBeenCalledWith("p1", null, "project");
    expect(rpc.send).toHaveBeenCalledWith("p1", "", "작품 전체 플롯 봐줘", { scope: "project", language: "ko" });
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
      language: "ko",
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

  it("ignores duplicate sends while a companion run is pending", async () => {
    let resolveSend: (value: { run_id: string }) => void = () => {};
    rpc.send.mockReturnValue(new Promise((resolve) => { resolveSend = resolve; }));
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      void result.current.send("같은 요청");
      void result.current.send("같은 요청");
    });

    expect(rpc.send).toHaveBeenCalledTimes(1);
    expect(result.current.messages.filter((m) => m.role === "user" && m.content === "같은 요청")).toHaveLength(1);

    await act(async () => {
      resolveSend({ run_id: "r1" });
    });
  });

  it("forwards an explicit companion intent when provided", async () => {
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(ev.listeners.has("companion-done")).toBe(true));

    await act(async () => {
      await result.current.send("현재 씬 본문 써줘", [], { kind: "scene_write", target_node_id: "n1", apply_policy: "direct" });
    });

    expect(rpc.send).toHaveBeenCalledWith("p1", "n1", "현재 씬 본문 써줘", {
      intent: { kind: "scene_write", target_node_id: "n1", apply_policy: "direct" },
      language: "ko",
    });
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

  it("stores the last user message for retry when a companion run errors", async () => {
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(ev.listeners.has("companion-error")).toBe(true));

    await act(async () => {
      await result.current.send("현재 씬 본문 써줘");
    });

    fire("companion-error", { run_id: "r1", message: "본문 변경이 만들어지지 않았습니다." });
    const last = result.current.messages[result.current.messages.length - 1];
    expect(last).toMatchObject({
      role: "assistant",
      content: "본문 변경이 만들어지지 않았습니다.",
      errored: true,
      retryText: "현재 씬 본문 써줘",
    });
  });

  it("marks provider setup errors without losing the raw message or retry text", async () => {
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(ev.listeners.has("companion-error")).toBe(true));

    await act(async () => {
      await result.current.send("현재 씬 본문 써줘");
    });

    fire("companion-error", { run_id: "r1", message: "api key is required for auth mode api-key" });
    const last = result.current.messages[result.current.messages.length - 1];
    expect(last).toMatchObject({
      role: "assistant",
      content: "api key is required for auth mode api-key",
      rawError: "api key is required for auth mode api-key",
      errored: true,
      aiSetupIssue: "missing_key",
      retryText: "현재 씬 본문 써줘",
    });
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

    const appliedPayload = {
      run_id: "r1",
      summary: "아웃라인 수정",
      applied: 1,
      changed_nodes: [{ node_id: "n1", op: "set_scene_text", content_version: 2, char_count: 120 }],
    };
    fire("companion-applied", appliedPayload);
    expect(onApplied).toHaveBeenCalledOnce();
    expect(onApplied).toHaveBeenCalledWith(appliedPayload);
  });

  it("clears transcript through rpc and local state", async () => {
    rpc.history.mockResolvedValue([{ role: "user", content: "남은 대화", timestamp: 1 }]);
    const { result } = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(result.current.messages).toHaveLength(1));

    await act(async () => {
      await result.current.clear();
    });

    expect(rpc.clear).toHaveBeenCalledWith("p1", "n1", "scene");
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

    expect(rpc.compact).toHaveBeenCalledWith("p1", "n1", "scene", "ko");
    expect(result.current.messages).toEqual([
      { role: "assistant", content: "이전 컴패니언 대화 요약\n- 나: 긴 질문" },
    ]);
  });

  it("retries loading persisted transcript after an initial history failure", async () => {
    rpc.history
      .mockRejectedValueOnce(new Error("engine warming up"))
      .mockResolvedValueOnce([
        { role: "user", content: "재시작 전 질문", timestamp: 1 },
        { role: "assistant", content: "재시작 후에도 보여야 하는 답", timestamp: 2 },
      ]);

    const first = renderHook(() => useCompanion("p1", { current: "n1" }));
    await waitFor(() => expect(rpc.history).toHaveBeenCalledTimes(1));
    expect(first.result.current.messages).toEqual([]);
    first.unmount();

    const second = renderHook(() => useCompanion("p1", { current: "n1" }));

    await waitFor(() => expect(second.result.current.messages).toHaveLength(2));
    expect(rpc.history).toHaveBeenCalledTimes(2);
    expect(second.result.current.messages[1]).toMatchObject({
      role: "assistant",
      content: "재시작 후에도 보여야 하는 답",
    });
  });
});
