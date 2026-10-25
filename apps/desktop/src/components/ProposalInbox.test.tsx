import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { ToastProvider } from "./ToastProvider";
import { ProposalInbox } from "./ProposalInbox";
import type { SuggestionBatch, SuggestionItem } from "../lib/types";

const events = vi.hoisted(() => ({ listeners: new Map<string, (event: { payload: unknown }) => void>() }));
vi.mock("@tauri-apps/api/event", () => ({
  listen: (name: string, listener: (event: { payload: unknown }) => void) => {
    events.listeners.set(name, listener);
    return Promise.resolve(() => events.listeners.delete(name));
  },
}));

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  accept: vi.fn(),
  reject: vi.fn(),
  undo: vi.fn(),
  extractionProgress: vi.fn(),
  settingsGet: vi.fn(),
}));

vi.mock("../lib/rpc", () => ({
  settings: { get: mocks.settingsGet },
  proposals: {
    list: mocks.list,
    accept: mocks.accept,
    reject: mocks.reject,
    undo: mocks.undo,
    pendingCount: vi.fn().mockResolvedValue({ count: 0 }),
    extractionEstimate: vi.fn(),
    extractionProgress: mocks.extractionProgress,
  },
}));

function item(overrides: Partial<SuggestionItem> = {}): SuggestionItem {
  return {
    id: "item-1",
    proposal_id: "p-1",
    ordinal: 0,
    op_type: "create_entity",
    op: { op: "create_entity", name: "하린", kind: "character", role: "검술 사범" },
    summary: "새 인물: 하린 — 검술 사범",
    depends_on: [],
    blocked_by_rejected: false,
    evidence: [],
    from_node_ids: [],
    duplicates: [],
    status: "pending",
    result: {},
    status_at: 1000,
    ...overrides,
  };
}

function batch(items: SuggestionItem[], overrides: Partial<SuggestionBatch> = {}): SuggestionBatch {
  return {
    id: "p-1",
    project_id: "project-1",
    purpose: "general",
    source: "agent",
    summary: "묶음",
    created_at: 1000,
    items,
    ...overrides,
  };
}

function renderInbox(props: Partial<React.ComponentProps<typeof ProposalInbox>> = {}) {
  return render(
    <ToastProvider>
      <I18nProvider>
        <ProposalInbox projectId="project-1" onClose={vi.fn()} {...props} />
      </I18nProvider>
    </ToastProvider>,
  );
}

describe("ProposalInbox", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.settingsGet.mockResolvedValue({ language: "ko" });
    events.listeners.clear();
  });

  it("lists items with their summary", async () => {
    mocks.list.mockResolvedValue({ proposals: [batch([item()])] });
    renderInbox();
    expect(await screen.findByText("새 인물: 하린 — 검술 사범")).toBeTruthy();
  });

  it("shows evidence quotes and breadcrumbs", async () => {
    mocks.list.mockResolvedValue({
      proposals: [
        batch([
          item({ evidence: [{ node_id: "n1", paragraph: 1, quote: "하린은 검을 뽑았다", breadcrumb: "1부 / 1장 / 씬 1" }] }),
        ]),
      ],
    });
    renderInbox();
    expect(await screen.findByText("“하린은 검을 뽑았다”")).toBeTruthy();
    expect(screen.getByText("1부 / 1장 / 씬 1")).toBeTruthy();
  });

  it("applies the selected item", async () => {
    mocks.list.mockResolvedValue({ proposals: [batch([item()])] });
    mocks.accept.mockResolvedValue({ items: [item({ status: "accepted", result: { created_id: "e-1" } })] });
    const user = userEvent.setup();
    renderInbox();
    await screen.findByText("새 인물: 하린 — 검술 사범");
    await user.click(screen.getByRole("checkbox"));
    await user.click(screen.getByRole("button", { name: "선택 항목 적용" }));
    await waitFor(() => expect(mocks.accept).toHaveBeenCalledWith("project-1", ["item-1"], undefined));
  });

  // story-assist Phase 4 (4.C.2): accepting a purpose=forecast structural
  // batch returns undo_batch_ids; the inbox offers a revert button for it
  // until the next accept/reject, or until it is used.
  it("offers a revert button after accepting a structural batch, and it calls proposals.undo", async () => {
    mocks.list.mockResolvedValue({ proposals: [batch([item()])] });
    mocks.accept.mockResolvedValue({
      items: [item({ status: "accepted", result: { created_id: "n-1" }, undo_batch_id: "batch-1" })],
      undo_batch_ids: ["batch-1"],
    });
    mocks.undo.mockResolvedValue({ ok: true });
    const user = userEvent.setup();
    renderInbox();
    await screen.findByText("새 인물: 하린 — 검술 사범");
    await user.click(screen.getByRole("checkbox"));
    await user.click(screen.getByRole("button", { name: "선택 항목 적용" }));

    const undoButton = await screen.findByTestId("proposal-inbox-undo");
    await user.click(undoButton);
    expect(mocks.undo).toHaveBeenCalledWith("project-1", "batch-1");
    await waitFor(() => expect(screen.queryByTestId("proposal-inbox-undo")).not.toBeInTheDocument());
  });

  it("does not offer a revert button when accept returns no undo batch", async () => {
    mocks.list.mockResolvedValue({ proposals: [batch([item()])] });
    mocks.accept.mockResolvedValue({ items: [item({ status: "accepted", result: { created_id: "e-1" } })] });
    const user = userEvent.setup();
    renderInbox();
    await screen.findByText("새 인물: 하린 — 검술 사범");
    await user.click(screen.getByRole("checkbox"));
    await user.click(screen.getByRole("button", { name: "선택 항목 적용" }));
    await waitFor(() => expect(mocks.accept).toHaveBeenCalled());
    expect(screen.queryByTestId("proposal-inbox-undo")).not.toBeInTheDocument();
  });

  it("auto-selects an unmet dependency and notes it", async () => {
    const entityItem = item({ id: "e-item", summary: "새 인물: 하린" });
    const relItem = item({
      id: "rel-item",
      op_type: "create_relationship",
      op: { op: "create_relationship", from_ref: "e1", to_ref: "e1", label: "스승" },
      summary: "관계: 하린 → 하린 (스승)",
      depends_on: ["e-item"],
    });
    mocks.list.mockResolvedValue({ proposals: [batch([entityItem, relItem])] });
    const user = userEvent.setup();
    renderInbox();
    await screen.findByText("관계: 하린 → 하린 (스승)");
    expect(screen.getByText("'새 인물: 하린' 생성이 먼저 필요")).toBeTruthy();

    const relRow = screen.getByText("관계: 하린 → 하린 (스승)").closest(".pi-row") as HTMLElement;
    const relCheckbox = relRow.querySelector("input[type=checkbox]") as HTMLInputElement;
    await user.click(relCheckbox);

    const entityRow = screen.getByText("새 인물: 하린").closest(".pi-row") as HTMLElement;
    const entityCheckbox = entityRow.querySelector("input[type=checkbox]") as HTMLInputElement;
    expect(entityCheckbox.checked).toBe(true);
  });

  it("converts a duplicate candidate into an alias of the existing entity", async () => {
    mocks.list.mockResolvedValue({
      proposals: [
        batch([
          item({
            duplicates: [{ entity_id: "existing-1", name: "김민호", kind: "character", matched: "exact" }],
          }),
        ]),
      ],
    });
    mocks.accept.mockResolvedValue({ items: [item({ status: "accepted", result: { created_id: "existing-1" } })] });
    const user = userEvent.setup();
    renderInbox();
    await screen.findByText("같은 대상일 수 있음: 김민호(인물)");
    await user.click(screen.getByRole("button", { name: "기존 김민호에 별칭으로" }));
    await user.click(screen.getByRole("button", { name: "선택 항목 적용" }));
    await waitFor(() =>
      expect(mocks.accept).toHaveBeenCalledWith("project-1", ["item-1"], {
        "item-1": { op: "update_entity", entity_id: "existing-1", aliases: ["하린"] },
      }),
    );
  });

  it("applies an edit before accepting", async () => {
    mocks.list.mockResolvedValue({ proposals: [batch([item()])] });
    mocks.accept.mockResolvedValue({ items: [item({ status: "accepted", result: { created_id: "e-1" } })] });
    const user = userEvent.setup();
    renderInbox();
    await screen.findByText("새 인물: 하린 — 검술 사범");
    await user.click(screen.getByRole("button", { name: "편집" }));
    const nameInput = screen.getByDisplayValue("하린");
    await user.clear(nameInput);
    await user.type(nameInput, "하린(수정)");
    await user.click(screen.getByRole("checkbox"));
    await user.click(screen.getByRole("button", { name: "선택 항목 적용" }));
    await waitFor(() =>
      expect(mocks.accept).toHaveBeenCalledWith(
        "project-1",
        ["item-1"],
        expect.objectContaining({ "item-1": expect.objectContaining({ name: "하린(수정)" }) }),
      ),
    );
  });

  it("shows a failure on the item after a partial accept failure", async () => {
    // The list refetch after accept reflects what the engine actually
    // stored — the failed status and its error — not the UI's optimistic
    // guess, so the mock's second call simulates that.
    mocks.list
      .mockResolvedValueOnce({ proposals: [batch([item()])] })
      .mockResolvedValueOnce({
        proposals: [batch([item({ status: "failed", result: { error: "create_entity: name required" } })])],
      });
    mocks.accept.mockResolvedValue({
      items: [item({ status: "failed", result: { error: "create_entity: name required" } })],
    });
    const user = userEvent.setup();
    renderInbox();
    await screen.findByText("새 인물: 하린 — 검술 사범");
    await user.click(screen.getByRole("checkbox"));
    await user.click(screen.getByRole("button", { name: "선택 항목 적용" }));
    expect(await screen.findByText("create_entity: name required")).toBeTruthy();
  });

  it("refetches when the engine reports a propose or accept/reject event", async () => {
    mocks.list.mockResolvedValue({ proposals: [batch([item()])] });
    renderInbox();
    await screen.findByText("새 인물: 하린 — 검술 사범");
    expect(mocks.list).toHaveBeenCalledTimes(1);

    act(() => {
      events.listeners.get("mcp-changed")?.({ payload: { project_id: "project-1", tool: "linetta_propose_story_ops" } });
    });
    await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2));

    act(() => {
      events.listeners.get("proposals-changed")?.({ payload: { project_id: "project-1" } });
    });
    await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(3));
  });

  it("ignores events for a different project", async () => {
    mocks.list.mockResolvedValue({ proposals: [batch([item()])] });
    renderInbox();
    await screen.findByText("새 인물: 하린 — 검술 사범");
    expect(mocks.list).toHaveBeenCalledTimes(1);
    act(() => {
      events.listeners.get("mcp-changed")?.({ payload: { project_id: "other-project", tool: "linetta_propose_story_ops" } });
    });
    await new Promise((r) => setTimeout(r, 0));
    expect(mocks.list).toHaveBeenCalledTimes(1);
  });

  it("shows the empty state when there is nothing pending", async () => {
    mocks.list.mockResolvedValue({ proposals: [] });
    renderInbox();
    expect(await screen.findByText("쌓인 제안이 없습니다.")).toBeTruthy();
  });
});
