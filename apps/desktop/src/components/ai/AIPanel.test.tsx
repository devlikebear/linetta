import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AIPanel } from "./AIPanel";
import { DEFAULT_AI_CONTEXT_SELECTION } from "./AIContextChecklist";
import type { AIContextPreview, ContextCounts } from "../../lib/types";

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

const preview: AIContextPreview = {
  counts,
  selectedItemCount: 4,
  sections: [
    {
      id: "current_scene",
      label: "현재 씬 본문",
      present: true,
      selected: true,
      count: 1,
      preview: "현재 씬 본문입니다.",
    },
    {
      id: "plot",
      label: "플롯 (스토리라인&비트)",
      present: true,
      selected: true,
      count: 3,
      preview: "[현재 씬]\n  · [첫 장면] #1 마지막 기회 — 장소로 향한다",
    },
  ],
};

function renderPanel(overrides = {}) {
  const props = {
    mode: "insert" as const,
    canChooseMode: true,
    options: { tone: "my" as const, short_form: false },
    contextItemCount: 0,
    contextPreview: preview,
    contextSelection: DEFAULT_AI_CONTEXT_SELECTION,
    variations: [],
    currentIdx: 0,
    status: { kind: "idle" as const },
    onModeChange: vi.fn(),
    onOptionsChange: vi.fn(),
    onContextSelectionChange: vi.fn(),
    onRun: vi.fn(),
    onSwitch: vi.fn(),
    onAccept: vi.fn(),
    onCancel: vi.fn(),
    onContextClick: vi.fn(),
    showChecklist: false,
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

  it("locks accept and retry while the current result is still streaming", () => {
    const { props, container } = renderPanel({
      variations: [{ text: "아직 쓰는 중", done: false }],
      status: { kind: "running" as const },
    });

    const retry = screen.getByRole("button", { name: "다시" });
    const accept = screen.getByRole("button", { name: /수락/ });
    expect(retry).toBeDisabled();
    expect(accept).toBeDisabled();

    fireEvent.keyDown(container.firstElementChild!, { key: "Tab" });
    expect(props.onAccept).not.toHaveBeenCalled();
  });

  it("lets writers disable injected context and preview plot beats", async () => {
    const user = userEvent.setup();
    const { props } = renderPanel({
      showChecklist: true,
      contextItemCount: 4,
    });

    await user.click(screen.getByRole("checkbox", { name: "플롯 (스토리라인&비트)" }));
    expect(props.onContextSelectionChange).toHaveBeenCalledWith({
      ...DEFAULT_AI_CONTEXT_SELECTION,
      plot: false,
    });

    await user.click(screen.getByRole("button", { name: "플롯 (스토리라인&비트) 미리보기" }));
    expect(screen.getByText(/마지막 기회/)).toBeInTheDocument();
    expect(screen.getByText(/첫 장면/)).toBeInTheDocument();
  });
});
