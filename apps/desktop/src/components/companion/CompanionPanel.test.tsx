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
      errored?: boolean;
    }[],
    streaming: "",
    thinking: "",
    status: "idle",
    send: vi.fn(),
    cancel: vi.fn(),
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
      status: "idle",
      send: vi.fn(),
      cancel: vi.fn(),
    };
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
});
