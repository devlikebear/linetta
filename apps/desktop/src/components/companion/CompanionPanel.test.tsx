import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
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

vi.mock("../../hooks/useCompanion", () => ({
  useCompanion: () => companionState.value,
}));

describe("CompanionPanel", () => {
  beforeEach(() => {
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
    render(<CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} />);
    expect(screen.getByText("생각 중…")).toBeInTheDocument();
  });

  it("renders provider reasoning in a collapsible block while streaming", () => {
    companionState.value = {
      ...companionState.value,
      status: "streaming",
      streaming: "초안",
      reasoning: "주인공의 동기를 먼저 정한다",
    };
    render(<CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} />);
    expect(screen.getByText("추론 중…")).toBeInTheDocument();
    expect(screen.getByText("주인공의 동기를 먼저 정한다")).toBeInTheDocument();
  });

  it("sends non-empty messages", async () => {
    const user = userEvent.setup();
    render(<CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} />);

    await user.type(screen.getByPlaceholderText(/메시지/), "도와줘");
    await user.click(screen.getByRole("button", { name: "전송" }));

    expect(companionState.value.send).toHaveBeenCalledWith("도와줘");
  });

  it("shows thinking state and hides query/proposal fences from live prose", () => {
    companionState.value = {
      ...companionState.value,
      status: "streaming",
      thinking: "씬 조회 중",
      streaming: "확인 중\n```linetta-query\n{}\n```\n숨겨질 내용",
    };

    render(<CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} />);

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

    render(<CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} />);

    expect(screen.getByText("제안")).toBeInTheDocument();
    expect(screen.getByText("스토리라인 생성: 추적자")).toBeInTheDocument();
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

    render(<CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} />);

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

    render(<CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} />);

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

    render(<CompanionPanel projectId="p1" nodeIdRef={{ current: "n1" }} onClose={vi.fn()} onApplied={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "대화 압축" }));
    await user.click(screen.getByRole("button", { name: "대화 클리어" }));

    expect(companionState.value.compact).toHaveBeenCalledOnce();
    expect(companionState.value.clear).toHaveBeenCalledOnce();
  });
});
