import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../lib/i18n";
import type { OutlineChangePreview } from "../../lib/types";
import { OutlinePreviewCard } from "./OutlinePreviewCard";

const mocks = vi.hoisted(() => ({
  applyOps: vi.fn(),
  undoApply: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  settings: {
    get: vi.fn().mockResolvedValue({ language: "ko" }),
  },
  companion: {
    applyOps: mocks.applyOps,
    undoApply: mocks.undoApply,
  },
}));

function previewFixture(overrides: Partial<OutlineChangePreview> = {}): OutlineChangePreview {
  return {
    summary: "1부 아웃라인 구성",
    counts: { created: 3, renamed: 1, deleted: 0, moved: 0, other: 2 },
    tree: [
      { ref: "p1", label: "1부", title: "항구의 복수극", kind: "container", depth: 0, action: "create" },
      { ref: "c1", label: "1화", kind: "container", depth: 1, action: "create" },
      { ref: "s1", label: "씬 1", title: "안개 낀 항구", kind: "leaf", depth: 2, action: "create" },
      { node_id: "n9", label: "3화", depth: 0, action: "rename" },
    ],
    ops: [
      { op: "create_outline_node", ref: "p1", kind: "container", label: "1부" },
      { op: "create_outline_node", ref: "c1", kind: "container", parent_node_ref: "p1", label: "1화" },
    ],
    ...overrides,
  };
}

function renderCard(preview = previewFixture(), onApplied = vi.fn()) {
  render(
    <I18nProvider>
      <OutlinePreviewCard
        preview={preview}
        projectId="project-1"
        nodeIdRef={{ current: "node-1" }}
        onApplied={onApplied}
      />
    </I18nProvider>,
  );
  return { onApplied };
}

describe("OutlinePreviewCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.applyOps.mockResolvedValue({ applied: 4, failures: [], undo_batch_id: "batch-1" });
    mocks.undoApply.mockResolvedValue({ ok: true });
  });

  it("shows how much the change touches before anything is applied", () => {
    renderCard();

    expect(screen.getByText("1부 아웃라인 구성")).toBeInTheDocument();
    expect(screen.getByText("추가 3개 · 이름 변경 1개 · 그 외 변경 2개")).toBeInTheDocument();
    expect(screen.getByText("항구의 복수극")).toBeInTheDocument();
    expect(screen.getByText("씬 1")).toBeInTheDocument();
    expect(mocks.applyOps).not.toHaveBeenCalled();
  });

  it("keeps apply and discard as separate actions", async () => {
    const user = userEvent.setup();
    renderCard();

    await user.click(screen.getByRole("button", { name: "취소" }));

    expect(screen.getByText("변경을 적용하지 않았습니다.")).toBeInTheDocument();
    expect(mocks.applyOps).not.toHaveBeenCalled();
  });

  it("applies the batch and then offers a single undo", async () => {
    const user = userEvent.setup();
    const { onApplied } = renderCard();

    await user.click(screen.getByRole("button", { name: "적용" }));

    await waitFor(() => expect(screen.getByText("변경 4건을 적용했어요.")).toBeInTheDocument());
    expect(mocks.applyOps).toHaveBeenCalledWith("project-1", "node-1", "", previewFixture().ops);
    expect(onApplied).toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: /되돌리기/ }));

    await waitFor(() => expect(screen.getByText("아웃라인을 적용 전으로 되돌렸어요.")).toBeInTheDocument());
    expect(mocks.undoApply).toHaveBeenCalledWith("batch-1");
    expect(screen.queryByRole("button", { name: /되돌리기/ })).not.toBeInTheDocument();
  });

  it("reports a rolled-back apply instead of claiming success", async () => {
    const user = userEvent.setup();
    const { onApplied } = renderCard();
    mocks.applyOps.mockResolvedValue({
      applied: 0,
      failures: [{ index: 2, op: "rename_outline_node", error: "node not found" }],
      rolled_back: true,
    });

    await user.click(screen.getByRole("button", { name: "적용" }));

    await waitFor(() =>
      expect(screen.getByText("적용 도중 오류가 생겨 아웃라인을 원래대로 되돌렸습니다.")).toBeInTheDocument(),
    );
    expect(onApplied).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "적용" })).toBeInTheDocument();
  });

  it("collapses a long tree behind a show-all control", async () => {
    const user = userEvent.setup();
    const tree = Array.from({ length: 20 }, (_, i) => ({
      ref: `s${i}`,
      label: `씬 ${i + 1}`,
      depth: 1,
      action: "create" as const,
    }));
    renderCard(previewFixture({ tree, counts: { created: 20, renamed: 0, deleted: 0, moved: 0, other: 0 } }));

    expect(screen.queryByText("씬 20")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "8개 더 보기" }));

    expect(screen.getByText("씬 20")).toBeInTheDocument();
  });
});
