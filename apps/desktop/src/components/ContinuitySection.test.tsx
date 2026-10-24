import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import type { ContinuityCheck } from "../lib/types";
import { ContinuitySection } from "./ContinuitySection";

const mocks = vi.hoisted(() => ({
  listeners: new Map<string, (e: { payload: unknown }) => void>(),
  list: vi.fn(),
  setStatus: vi.fn(),
}));
vi.mock("@tauri-apps/api/event", () => ({
  listen: (name: string, cb: (e: { payload: unknown }) => void) => {
    mocks.listeners.set(name, cb);
    return Promise.resolve(() => mocks.listeners.delete(name));
  },
}));
vi.mock("../lib/rpc", () => ({
  // I18nProvider reads the app language through the same module.
  settings: { get: vi.fn().mockResolvedValue({ language: "ko" }) },
  continuity: { list: mocks.list, setStatus: mocks.setStatus },
}));

function check(overrides: Partial<ContinuityCheck> = {}): ContinuityCheck {
  return {
    id: "check-1",
    project_id: "p",
    node_id: "n1",
    breadcrumb: "1부 / 1장 / 씬 1",
    content_version: 1,
    source: "agent",
    note: "",
    checked_at: Date.now() - 2 * 60000,
    stale: false,
    observations: [],
    ...overrides,
  };
}

function observation(overrides: Partial<ContinuityCheck["observations"][number]> = {}) {
  return {
    id: "obs-1",
    check_id: "check-1",
    project_id: "p",
    node_id: "n1",
    category: "character" as const,
    kind: "contradiction" as const,
    severity: "medium" as const,
    summary: "하린이 왼손잡이라는 설정과 맞지 않는다",
    suggestion: "",
    paragraph_from: 1,
    paragraph_to: 1,
    quote: "하린은 오른손으로 검을 뽑았다.",
    refs: [{ type: "entity" as const, id: "e-harin", label: "하린" }],
    status: "open" as const,
    status_at: Date.now(),
    created_at: Date.now(),
    ...overrides,
  };
}

function renderSection(props: Partial<Parameters<typeof ContinuitySection>[0]> = {}) {
  return render(
    <I18nProvider>
      <ContinuitySection projectId="p" nodeId="n1" {...props} />
    </I18nProvider>,
  );
}

describe("ContinuitySection", () => {
  beforeEach(() => {
    mocks.listeners.clear();
    mocks.list.mockReset().mockResolvedValue({ checks: [] });
    mocks.setStatus.mockReset();
  });

  it("lists a check's observations, ref chip included", async () => {
    mocks.list.mockResolvedValue({ checks: [check({ observations: [observation()] })] });
    renderSection({ onOpenEntity: vi.fn() });

    expect(await screen.findByText("하린이 왼손잡이라는 설정과 맞지 않는다")).toBeInTheDocument();
    expect(screen.getByText(/하린은 오른손으로 검을 뽑았다/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "하린" })).toBeInTheDocument();
  });

  it("shows a clean check as 'no issues' rather than an empty list", async () => {
    mocks.list.mockResolvedValue({ checks: [check({ observations: [] })] });
    renderSection();

    expect(await screen.findByText(/문제 없음/)).toBeInTheDocument();
  });

  it("shows a stale badge when the scene changed since the check", async () => {
    mocks.list.mockResolvedValue({ checks: [check({ stale: true, observations: [observation()] })] });
    renderSection();

    expect(await screen.findByText("본문이 바뀜 · 다시 점검")).toBeInTheDocument();
  });

  it("moves an observation to resolved and reflects it without a refetch", async () => {
    mocks.list.mockResolvedValue({ checks: [check({ observations: [observation()] })] });
    mocks.setStatus.mockResolvedValue({ ...observation(), status: "resolved" });
    const user = userEvent.setup();
    renderSection();

    await screen.findByText("하린이 왼손잡이라는 설정과 맞지 않는다");
    await user.click(screen.getByRole("button", { name: "해결됨" }));

    await waitFor(() => expect(mocks.setStatus).toHaveBeenCalledWith("obs-1", "resolved"));
    // The "열림" tab is still selected, so a resolved observation drops out
    // of the current view — it moved, it did not merely change label.
    expect(screen.queryByText("하린이 왼손잡이라는 설정과 맞지 않는다")).not.toBeInTheDocument();
  });

  it("filters by status tab", async () => {
    mocks.list.mockResolvedValue({
      checks: [
        check({
          observations: [
            observation({ id: "obs-open", status: "open" }),
            observation({ id: "obs-dismissed", status: "dismissed", summary: "무시된 관찰" }),
          ],
        }),
      ],
    });
    const user = userEvent.setup();
    renderSection();

    await screen.findByText("하린이 왼손잡이라는 설정과 맞지 않는다");
    expect(screen.queryByText("무시된 관찰")).not.toBeInTheDocument();

    await user.click(screen.getByTestId("continuity-status-dismissed"));
    expect(await screen.findByText("무시된 관찰")).toBeInTheDocument();
    expect(screen.queryByText("하린이 왼손잡이라는 설정과 맞지 않는다")).not.toBeInTheDocument();
  });

  it("switches scope between this scene and the whole work", async () => {
    mocks.list.mockResolvedValue({ checks: [] });
    const user = userEvent.setup();
    renderSection();

    await waitFor(() => expect(mocks.list).toHaveBeenCalledWith("p", "n1"));

    await user.click(screen.getByTestId("continuity-scope-work"));
    await waitFor(() => expect(mocks.list).toHaveBeenCalledWith("p", undefined));
  });

  it("shows the empty-work message when nothing has ever been checked", async () => {
    mocks.list.mockResolvedValue({ checks: [] });
    renderSection();

    expect(await screen.findByText(/아직 점검한 기록이 없습니다/)).toBeInTheDocument();
  });

  it("refetches when the engine reports a new report_continuity call", async () => {
    mocks.list.mockResolvedValue({ checks: [] });
    renderSection();

    await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));
    mocks.list.mockResolvedValue({ checks: [check({ observations: [observation()] })] });

    act(() => {
      mocks.listeners.get("mcp-changed")?.({ payload: { project_id: "p", tool: "linetta_report_continuity" } });
    });

    await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2));
    expect(await screen.findByText("하린이 왼손잡이라는 설정과 맞지 않는다")).toBeInTheDocument();
  });

  it("ignores an mcp-changed event for a different work", async () => {
    mocks.list.mockResolvedValue({ checks: [] });
    renderSection();
    await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));

    act(() => {
      mocks.listeners.get("mcp-changed")?.({ payload: { project_id: "other", tool: "linetta_report_continuity" } });
    });

    // Give any stray refetch a turn to happen before asserting it did not.
    await Promise.resolve();
    expect(mocks.list).toHaveBeenCalledTimes(1);
  });

  it("opens the scene from a scene ref chip", async () => {
    const onOpenNode = vi.fn();
    mocks.list.mockResolvedValue({
      checks: [
        check({
          observations: [
            observation({ refs: [{ type: "scene", id: "n-prev", label: "1부 / 서장" }] }),
          ],
        }),
      ],
    });
    const user = userEvent.setup();
    renderSection({ onOpenNode });

    const chip = await screen.findByRole("button", { name: "1부 / 서장" });
    await user.click(chip);
    expect(onOpenNode).toHaveBeenCalledWith("n-prev");
  });

  it("opens the Fact Book from a fact ref chip with its resolved claim", async () => {
    const onOpenFact = vi.fn();
    mocks.list.mockResolvedValue({
      checks: [
        check({
          observations: [
            observation({ refs: [{ type: "fact", id: "f-1", label: "하린은 왼손잡이다" }] }),
          ],
        }),
      ],
    });
    const user = userEvent.setup();
    renderSection({ onOpenFact });

    const chip = await screen.findByRole("button", { name: "하린은 왼손잡이다" });
    await user.click(chip);
    expect(onOpenFact).toHaveBeenCalledWith("f-1", "하린은 왼손잡이다");
  });

  it("renders a ref chip as plain text when no handler is wired", async () => {
    mocks.list.mockResolvedValue({ checks: [check({ observations: [observation()] })] });
    renderSection({ onOpenEntity: undefined });

    await screen.findByText("하린이 왼손잡이라는 설정과 맞지 않는다");
    expect(screen.queryByRole("button", { name: "하린" })).not.toBeInTheDocument();
    expect(screen.getByText("하린")).toBeInTheDocument();
  });

  it("surfaces a load error", async () => {
    mocks.list.mockRejectedValue(new Error("network down"));
    renderSection();

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
  });

  it("does not show the scope toggle when there is no open scene", async () => {
    mocks.list.mockResolvedValue({ checks: [] });
    render(
      <I18nProvider>
        <ContinuitySection projectId="p" />
      </I18nProvider>,
    );

    await waitFor(() => expect(mocks.list).toHaveBeenCalledWith("p", undefined));
    expect(screen.queryByTestId("continuity-scope-scene")).not.toBeInTheDocument();
  });

  it("groups several checks of the same scene, keeping the latest check's own metadata", async () => {
    mocks.list.mockResolvedValue({
      checks: [
        check({ id: "check-2", checked_at: Date.now(), observations: [] }),
        check({ id: "check-1", checked_at: Date.now() - 600000, observations: [observation({ status: "dismissed" })] }),
      ],
    });
    const user = userEvent.setup();
    renderSection();

    // "문제 없음" is about the latest check (zero observations), yet the
    // earlier check's still-open judgement must still surface once the
    // writer looks at the "무시" tab.
    expect(await screen.findByText(/문제 없음/)).toBeInTheDocument();
    await user.click(screen.getByTestId("continuity-status-dismissed"));
    expect(await screen.findByText("하린이 왼손잡이라는 설정과 맞지 않는다")).toBeInTheDocument();
  });
});
