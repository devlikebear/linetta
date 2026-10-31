import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentAsk } from "../../lib/types";

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, unknown>) =>
      vars
        ? `${key}:${Object.entries(vars)
            .map(([name, value]) => `${name}=${value}`)
            .join(",")}`
        : key,
  }),
}));

import { AskCard, formatAskAnswers } from "./AskCard";

const ask: AgentAsk = {
  questions: [
    {
      header: "연도",
      question: "누가 **천마**를 쓰러뜨렸나요?",
      options: [
        { label: "백가 선대", description: "묵로는 두 번째 주인", recommended: true },
        { label: "묵로 본인" },
      ],
    },
    {
      header: "수정",
      question: "어디를 손볼까요?",
      multi_select: true,
      options: [{ label: "1화 복선" }, { label: "2화 표현" }],
    },
  ],
};

function answerBoth() {
  fireEvent.click(screen.getByLabelText(/백가 선대/));
  fireEvent.click(screen.getByLabelText("1화 복선"));
}

describe("formatAskAnswers", () => {
  it("writes a line per question, with free answers after the picks", () => {
    expect(
      formatAskAnswers(ask.questions, [
        { chosen: ["묵로 본인"], other: false, text: "" },
        { chosen: ["1화 복선", "2화 표현"], other: true, text: "  3화도  " },
      ]),
    ).toBe("연도: 묵로 본인\n수정: 1화 복선, 2화 표현, 3화도");
  });

  it("drops a free answer whose box was switched off", () => {
    expect(formatAskAnswers(ask.questions.slice(1), [{ chosen: ["1화 복선"], other: false, text: "지운 답" }])).toBe(
      "수정: 1화 복선",
    );
  });
});

describe("AskCard", () => {
  it("sends nothing until every question has an answer", () => {
    const onSubmit = vi.fn();
    render(<AskCard ask={ask} open disabled={false} onSubmit={onSubmit} />);
    const submit = screen.getByTestId("agent-ask-submit");

    expect(submit).toBeDisabled();
    fireEvent.click(screen.getByLabelText(/백가 선대/));
    expect(screen.getByTestId("agent-ask-progress")).toHaveTextContent("done=1,total=2");
    expect(submit).toBeDisabled();

    fireEvent.click(screen.getByLabelText("1화 복선"));
    expect(submit).toBeEnabled();
    fireEvent.click(submit);
    expect(onSubmit).toHaveBeenCalledWith("연도: 백가 선대\n수정: 1화 복선");
  });

  it("keeps one pick on a single-choice question and several on a multi-choice one", () => {
    const onSubmit = vi.fn();
    render(<AskCard ask={ask} open disabled={false} onSubmit={onSubmit} />);
    fireEvent.click(screen.getByLabelText(/백가 선대/));
    fireEvent.click(screen.getByLabelText("묵로 본인"));
    fireEvent.click(screen.getByLabelText("1화 복선"));
    fireEvent.click(screen.getByLabelText("2화 표현"));
    fireEvent.click(screen.getByLabelText("1화 복선"));
    fireEvent.click(screen.getByLabelText("1화 복선"));
    fireEvent.click(screen.getByTestId("agent-ask-submit"));
    expect(onSubmit).toHaveBeenCalledWith("연도: 묵로 본인\n수정: 2화 표현, 1화 복선");
  });

  it("takes the writer's own answer in place of the options", () => {
    const onSubmit = vi.fn();
    render(<AskCard ask={ask} open disabled={false} onSubmit={onSubmit} />);
    fireEvent.click(screen.getByLabelText("1화 복선"));
    fireEvent.click(screen.getByTestId("agent-ask-other-0"));

    // An empty box is not an answer.
    expect(screen.getByTestId("agent-ask-submit")).toBeDisabled();
    fireEvent.change(screen.getByTestId("agent-ask-input-0"), { target: { value: "제3의 인물" } });
    fireEvent.click(screen.getByTestId("agent-ask-submit"));
    expect(onSubmit).toHaveBeenCalledWith("연도: 제3의 인물\n수정: 1화 복선");
  });

  it("switches a typed answer off when an option is picked instead", () => {
    const onSubmit = vi.fn();
    render(<AskCard ask={ask} open disabled={false} onSubmit={onSubmit} />);
    fireEvent.click(screen.getByTestId("agent-ask-other-0"));
    fireEvent.change(screen.getByTestId("agent-ask-input-0"), { target: { value: "제3의 인물" } });
    answerBoth();
    expect(screen.queryByTestId("agent-ask-input-0")).toBeNull();
    fireEvent.click(screen.getByTestId("agent-ask-submit"));
    expect(onSubmit).toHaveBeenCalledWith("연도: 백가 선대\n수정: 1화 복선");
  });

  it("marks the agent's recommendation and shows the option's meaning", () => {
    render(<AskCard ask={ask} open disabled={false} onSubmit={vi.fn()} />);
    expect(screen.getByText("agentPanel.ask.recommended")).toBeInTheDocument();
    expect(screen.getByText("묵로는 두 번째 주인")).toBeInTheDocument();
    expect(screen.getByText("천마").tagName).toBe("STRONG");
  });

  it("holds the answers while a turn is running", () => {
    const onSubmit = vi.fn();
    render(<AskCard ask={ask} open disabled onSubmit={onSubmit} />);
    answerBoth();
    expect(screen.getByTestId("agent-ask-submit")).toBeDisabled();
    fireEvent.submit(screen.getByTestId("agent-ask"));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("folds into the questions once it has been answered", () => {
    render(<AskCard ask={ask} open={false} disabled={false} onSubmit={vi.fn()} />);
    expect(screen.getByTestId("agent-ask-closed")).toHaveTextContent("agentPanel.ask.closed:count=2");
    expect(screen.queryByTestId("agent-ask-submit")).toBeNull();
  });
});
