import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AIPanel } from "./AIPanel";
import type { ContextCounts } from "../../lib/types";

const counts: ContextCounts = {
  nearbyScenes: 0,
  hasOutline: false,
  hasSynopsis: false,
  relatedScenes: 0,
  entities: 0,
  relationships: 0,
  plotBeats: 0,
  notes: 0,
  projectMetaFields: 0,
  hasStyleNotes: false,
};

function renderPanel(overrides = {}) {
  const props = {
    mode: "insert" as const,
    canChooseMode: true,
    options: { tone: "my" as const, short_form: false },
    contextItemCount: 0,
    variations: [],
    currentIdx: 0,
    status: { kind: "idle" as const },
    onModeChange: vi.fn(),
    onOptionsChange: vi.fn(),
    onRun: vi.fn(),
    onSwitch: vi.fn(),
    onAccept: vi.fn(),
    onCancel: vi.fn(),
    onContextClick: vi.fn(),
    showChecklist: false,
    checklistCounts: counts,
    ...overrides,
  };
  const rendered = render(<AIPanel {...props} />);
  return { props, ...rendered };
}

describe("AIPanel", () => {
  it("guards empty prompts and runs non-empty prompts", async () => {
    const user = userEvent.setup();
    const { props } = renderPanel();

    await user.click(screen.getByRole("button", { name: "생성 ⌘↵" }));
    expect(props.onRun).not.toHaveBeenCalled();

    await user.type(screen.getByPlaceholderText("프롬프트를 입력하세요…"), "이어 써줘");
    await user.keyboard("{Enter}");

    expect(props.onRun).toHaveBeenCalledWith("이어 써줘", false);
  });

  it("shows a generating indicator while running before any text", () => {
    renderPanel({ status: { kind: "running" }, variations: [] });
    expect(screen.getByText("AI 생성 중…")).toBeInTheDocument();
  });

  it("accepts a result with Tab and switches variations with arrows", () => {
    const { props, container } = renderPanel({
      variations: [
        { text: "A", done: true },
        { text: "B", done: true },
      ],
      currentIdx: 0,
      status: { kind: "done" as const },
    });

    fireEvent.keyDown(container.firstElementChild!, { key: "Tab" });
    fireEvent.keyDown(container.firstElementChild!, { key: "ArrowRight" });

    expect(props.onAccept).toHaveBeenCalledOnce();
    expect(props.onSwitch).toHaveBeenCalledWith(1);
  });
});
