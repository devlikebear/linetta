import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../lib/i18n";
import { CompanionPanel } from "./CompanionPanel";

const companionState = vi.hoisted(() => ({
  value: {
    messages: [] as {
      role: "user" | "assistant";
      content: string;
      proposal?: import("../../hooks/useCompanion").ChatMessage["proposal"];
      choices?: import("../../hooks/useCompanion").ChatMessage["choices"];
      errored?: boolean;
    }[],
    streaming: "",
    thinking: "",
    reasoning: "",
    status: "idle",
    send: vi.fn(),
    cancel: vi.fn(),
    clear: vi.fn(),
    compact: vi.fn(),
  },
}));
const mocks = vi.hoisted(() => ({
  settingsGet: vi.fn(),
}));

vi.mock("../../hooks/useCompanion", () => ({
  useCompanion: () => companionState.value,
}));

vi.mock("../../lib/rpc", () => ({
  settings: {
    get: mocks.settingsGet,
  },
}));

function renderPanel(props: Partial<ComponentProps<typeof CompanionPanel>> = {}) {
  return render(
    <I18nProvider>
      <CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} {...props} />
    </I18nProvider>,
  );
}

describe("CompanionPanel", () => {
  beforeEach(() => {
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    companionState.value = {
      messages: [],
      streaming: "",
      thinking: "",
      reasoning: "",
      status: "idle",
      send: vi.fn(),
      cancel: vi.fn(),
      clear: vi.fn(),
      compact: vi.fn(),
    };
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("shows a working indicator while streaming even before any prose", () => {
    companionState.value = { ...companionState.value, status: "streaming", streaming: "", thinking: "" };
    renderPanel();
    expect(screen.getByText("생각 중…")).toBeInTheDocument();
  });

  it("shows prompt examples in the empty state and copies one into the draft", async () => {
    const user = userEvent.setup();
    renderPanel();

    expect(screen.getByText("무엇부터 맡길까요?")).toBeInTheDocument();
    expect(screen.getByText("프롬프트 예시")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /아웃라인/ })).toBeInTheDocument();

    const example = screen.getByRole("button", {
      name: /최근 스페이스 오페라 장르 레퍼런스/,
    });
    await user.click(example);

    expect((screen.getByPlaceholderText(/메시지/) as HTMLTextAreaElement).value).toContain("web_search");
    expect(companionState.value.send).not.toHaveBeenCalled();
  });

  it("grows the draft textarea when a picked example wraps to multiple lines", async () => {
    const user = userEvent.setup();
    Object.defineProperty(HTMLTextAreaElement.prototype, "scrollHeight", {
      configurable: true,
      get() {
        return this.value.includes("아웃라인") ? 72 : 32;
      },
    });

    renderPanel();

    const input = screen.getByPlaceholderText(/메시지/) as HTMLTextAreaElement;
    expect(input.style.height).toBe("32px");

    await user.click(screen.getByRole("button", { name: /아웃라인/ }));

    expect(input.value).toContain("아웃라인");
    expect(input.style.height).toBe("72px");
  });

  it("toggles built-in tool help from the companion header", async () => {
    const user = userEvent.setup();
    renderPanel();

    expect(screen.queryByText("컴패니언이 사용할 수 있는 도구")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "컴패니언 도움말" }));

    expect(screen.getByText("컴패니언이 사용할 수 있는 도구")).toBeInTheDocument();
    expect(screen.getByText("web_search · 최신 자료나 장르 레퍼런스 찾기")).toBeInTheDocument();
    expect(screen.getByText("web_fetch · 특정 URL 본문 확인")).toBeInTheDocument();
    expect(screen.getByText("linetta_apply_ops · 개요, 스토리라인, 비트, 인물, 관계, 장소, 씬, 기억 갱신")).toBeInTheDocument();
  });

  it("renders provider reasoning in a collapsible block while streaming", () => {
    companionState.value = {
      ...companionState.value,
      status: "streaming",
      streaming: "초안",
      reasoning: "주인공의 동기를 먼저 정한다",
    };
    renderPanel();
    expect(screen.getByText("추론 중…")).toBeInTheDocument();
    expect(screen.getByText("주인공의 동기를 먼저 정한다")).toBeInTheDocument();
  });

  it("sends non-empty messages", async () => {
    const user = userEvent.setup();
    renderPanel();

    await user.type(screen.getByPlaceholderText(/메시지/), "도와줘");
    await user.click(screen.getByRole("button", { name: "전송" }));

    expect(companionState.value.send).toHaveBeenCalledWith("도와줘");
  });

  it("waits for the editor flush before sending messages", async () => {
    const user = userEvent.setup();
    let resolveFlush!: () => void;
    const beforeSend = vi.fn(() => new Promise<void>((resolve) => { resolveFlush = resolve; }));
    renderPanel({ beforeSend });

    await user.type(screen.getByPlaceholderText(/메시지/), "본문 보고 분석해줘");
    await user.click(screen.getByRole("button", { name: "전송" }));

    await waitFor(() => expect(beforeSend).toHaveBeenCalledOnce());
    expect(companionState.value.send).not.toHaveBeenCalled();

    resolveFlush();
    await waitFor(() => expect(companionState.value.send).toHaveBeenCalledWith("본문 보고 분석해줘"));
  });

  it("shows thinking state and hides query/proposal fences from live prose", () => {
    companionState.value = {
      ...companionState.value,
      status: "streaming",
      thinking: "씬 조회 중",
      streaming: "확인 중\n```linetta-query\n{}\n```\n숨겨질 내용",
    };

    renderPanel();

    expect(screen.getByText(/씬 조회 중/)).toBeInTheDocument();
    expect(screen.getByText("확인 중")).toBeInTheDocument();
    expect(screen.queryByText(/linetta-query/)).not.toBeInTheDocument();
  });

  it("renders proposal cards from assistant messages", () => {
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "이렇게 해볼게요",
        proposal: {
          run_id: "r1",
          valid: true,
          summary: "제안",
          ops: [{ op: "create_thread", name: "추적자" }],
        },
      }],
    };

    renderPanel();

    expect(screen.getByText("제안")).toBeInTheDocument();
    expect(screen.getByText("스토리라인 생성: 추적자")).toBeInTheDocument();
  });

  it("renders fact-card proposal operations", () => {
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "자료집에 저장할게요.",
        proposal: {
          run_id: "r1",
          valid: true,
          summary: "자료집 저장",
          ops: [{
            op: "create_fact_card",
            claim: "런던 일반 경찰은 항상 총기를 휴대한다",
            result: "일반 경찰은 통상 비무장이다.",
            status: "verified",
            sources: [{ url: "https://www.met.police.uk/" }],
          }],
        },
      }],
    };

    renderPanel();

    expect(screen.getByText("자료집 저장")).toBeInTheDocument();
    expect(screen.getByText("자료집 카드 생성: 런던 일반 경찰은 항상 총기를 휴대한다")).toBeInTheDocument();
  });

  it("renders choice buttons and sends the picked option on click", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [{
        role: "assistant",
        content: "어떤 제목으로 할까요?",
        choices: {
          run_id: "r1",
          prompt: "새 제목?",
          options: ["「부엌」", "「온기」"],
          allow_custom: true,
        },
      }],
    };

    renderPanel();

    expect(screen.getByText("새 제목?")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "직접 입력" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "「온기」" }));
    expect(companionState.value.send).toHaveBeenCalledWith("「온기」");
  });

  it("copies visible chat messages to the clipboard", async () => {
    const user = userEvent.setup();
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
    companionState.value = {
      ...companionState.value,
      messages: [
        { role: "user", content: "이 장면 이상해?" },
        { role: "assistant", content: "동기가 조금 더 필요해요." },
      ],
    };

    renderPanel();

    await user.click(screen.getByRole("button", { name: "대화 복사" }));

    expect(writeText).toHaveBeenCalledWith(expect.stringContaining("나:\n이 장면 이상해?"));
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining("컴패니언:\n동기가 조금 더 필요해요."));
  });

  it("compacts and clears chat through panel actions", async () => {
    const user = userEvent.setup();
    companionState.value = {
      ...companionState.value,
      messages: [
        { role: "user", content: "요약해줘" },
        { role: "assistant", content: "요약했습니다." },
      ],
    };

    renderPanel();

    await user.click(screen.getByRole("button", { name: "대화 압축" }));
    await user.click(screen.getByRole("button", { name: "대화 클리어" }));

    expect(companionState.value.compact).toHaveBeenCalledOnce();
    expect(companionState.value.clear).toHaveBeenCalledOnce();
  });

  it("renders companion controls in English when selected", async () => {
    mocks.settingsGet.mockResolvedValue({ language: "en" });

    renderPanel();

    expect(await screen.findByText("Writing companion")).toBeInTheDocument();
    expect(screen.getByText("Ask anything or shape the plot together.")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Message... (Enter to send, Shift+Enter for line break)")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send" })).toBeInTheDocument();
  });
});
