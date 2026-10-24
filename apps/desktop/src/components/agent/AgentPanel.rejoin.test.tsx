import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentHistoryRow, AgentRunStatus } from "../../lib/types";

// What a panel does about a turn it was not there for the start of: the
// writer closed the panel, or left the workspace, and the engine kept going
// (#93). Kept apart from AgentPanel.test.tsx because every test here needs
// agent.status, which none of those do — they pin the fallback for an engine
// that cannot answer it.

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
  agentRun: vi.fn(),
  agentCancel: vi.fn(),
  agentHistory: vi.fn(),
  agentStatus: vi.fn(),
  settingsGet: vi.fn(),
}));

const ev = vi.hoisted(() => ({
  listeners: new Map<string, (e: { payload: unknown }) => void>(),
}));

vi.mock("@tauri-apps/api/event", () => ({
  listen: (event: string, cb: (e: { payload: unknown }) => void) => {
    ev.listeners.set(event, cb);
    return Promise.resolve(() => ev.listeners.delete(event));
  },
}));

vi.mock("../../lib/rpc", () => ({
  providers: { list: rpc.providersList },
  agent: {
    run: rpc.agentRun,
    cancel: rpc.agentCancel,
    history: rpc.agentHistory,
    status: rpc.agentStatus,
  },
  settings: { get: rpc.settingsGet },
}));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

import { AgentPanel } from "./AgentPanel";

const PROJECT_ID = "p1";

function historyRow(id: string, role: AgentHistoryRow["role"], content: string, runId: string): AgentHistoryRow {
  return { id, project_id: PROJECT_ID, run_id: runId, role, status: "done", content, created_at: 1 };
}

function toolRow(id: string, name: string, runId: string): AgentHistoryRow {
  return historyRow(id, "tool", JSON.stringify({ name, ok: true }), runId);
}

const idle: AgentRunStatus = { running: false, seq: 0 };

async function renderPanel() {
  const view = render(
    <MemoryRouter>
      <AgentPanel onClose={() => {}} projectId={PROJECT_ID} nodeId="n1" />
    </MemoryRouter>,
  );
  // providers.list, then the restore's status → history → status.
  await act(async () => {
    for (let i = 0; i < 10; i++) await Promise.resolve();
  });
  return view;
}

async function emit(event: string, payload: Record<string, unknown>) {
  const cb = ev.listeners.get(event);
  if (!cb) throw new Error(`${event} listener was never registered`);
  await act(async () => {
    cb({ payload: { project_id: PROJECT_ID, ...payload } });
    for (let i = 0; i < 5; i++) await Promise.resolve();
  });
}

function logText(): string {
  return screen.getByTestId("agent-log").textContent ?? "";
}

function noticeTexts(): string[] {
  return Array.from(screen.getByTestId("agent-log").querySelectorAll('[data-testid="agent-notice"]')).map(
    (el) => el.textContent ?? "",
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  ev.listeners.clear();
  rpc.providersList.mockImplementation(() =>
    Promise.resolve([{ id: "anthropic", auth: "api_key", active: true, configured: true, consented: true }]),
  );
  rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
  rpc.agentStatus.mockImplementation(() => Promise.resolve({ ...idle }));
  rpc.agentCancel.mockImplementation(() => Promise.resolve({ ok: true }));
  rpc.settingsGet.mockImplementation(() => Promise.resolve({ review_tools_enabled: true }));
});

describe("AgentPanel rejoining a running turn", () => {
  const midTurn = [
    historyRow("1", "user", "모든 씬을 고쳐줘", "r1"),
    historyRow("2", "assistant", "먼저 작품을 읽을게요.", "r1"),
    toolRow("3", "linetta_read_scene", "r1"),
  ];

  it("shows the turn as running, with a stop button, when the engine says it is", async () => {
    rpc.agentHistory.mockImplementation(() => Promise.resolve(midTurn));
    rpc.agentStatus.mockImplementation(() =>
      Promise.resolve({ running: true, run_id: "r1", started_at: Date.now() - 65_000, seq: 4 }),
    );
    await renderPanel();

    expect(logText()).toContain("먼저 작품을 읽을게요.");
    // Not the hedge, and not a send button the engine would refuse.
    expect(noticeTexts()).toEqual([]);
    expect(screen.getByTestId("agent-stop")).toBeTruthy();
    expect(screen.queryByTestId("agent-send")).toBeNull();
    // The working line, on the engine's clock rather than this mount's.
    const working = screen.getByTestId("agent-working");
    expect(working.textContent).toContain("agentPanel.working.thinking");
    expect(working.textContent).toContain("1:05");

    await act(async () => {
      fireEvent.click(screen.getByTestId("agent-stop"));
      for (let i = 0; i < 5; i++) await Promise.resolve();
    });
    expect(rpc.agentCancel).toHaveBeenCalledWith("r1");
  });

  it("draws the reply streamed so far and continues it", async () => {
    rpc.agentHistory.mockImplementation(() => Promise.resolve(midTurn.slice(0, 1)));
    rpc.agentStatus.mockImplementation(() =>
      Promise.resolve({ running: true, run_id: "r1", started_at: 1, seq: 2, text: "먼저 작품을 " }),
    );
    await renderPanel();
    expect(screen.getByTestId("agent-working").textContent).toContain("agentPanel.working.writing");

    // The rest of that message arrives and continues the same bubble. (The
    // tool call that follows ends the reveal, so the full text is on screen
    // without driving animation frames.)
    await emit("agent-delta", { run_id: "r1", seq: 3, text: "읽을게요." });
    await emit("agent-tool", { run_id: "r1", seq: 4, name: "linetta_read_scene", state: "started" });
    const bubbles = screen.getByTestId("agent-log").querySelectorAll(".msg.bot");
    expect(bubbles).toHaveLength(1);
    expect(bubbles[0].textContent).toBe("먼저 작품을 읽을게요.");
  });

  it("drops events its snapshot already accounts for and applies the ones after it", async () => {
    rpc.agentHistory.mockImplementation(() => Promise.resolve(midTurn));
    rpc.agentStatus.mockImplementation(() =>
      Promise.resolve({ running: true, run_id: "r1", started_at: 1, seq: 4, tool: "linetta_write_scene" }),
    );
    await renderPanel();
    // One restored read, one write still running.
    expect(screen.getAllByTestId("tool-line")).toHaveLength(2);
    expect(screen.getByTestId("agent-working").textContent).toContain("agentPanel.working.tool");

    // A straggler from before the snapshot: the read's own "done".
    await emit("agent-tool", { run_id: "r1", seq: 3, name: "linetta_read_scene", state: "done" });
    await emit("agent-tool", { run_id: "r1", seq: 4, name: "linetta_write_scene", state: "started" });
    expect(screen.getAllByTestId("tool-line")).toHaveLength(2);

    // The write resolves the running line rather than adding one.
    await emit("agent-tool", { run_id: "r1", seq: 5, name: "linetta_write_scene", state: "done", snapshot_id: "s1" });
    const lines = screen.getAllByTestId("tool-line");
    expect(lines).toHaveLength(2);
    expect(lines[1].className).toContain("tool-ok");
  });

  it("settles on the transcript when the rejoined turn ends, and hands the composer back", async () => {
    rpc.agentHistory.mockImplementation(() => Promise.resolve(midTurn));
    rpc.agentStatus.mockImplementation(() => Promise.resolve({ running: true, run_id: "r1", started_at: 1, seq: 4 }));
    await renderPanel();

    const finished = [...midTurn, historyRow("4", "assistant", "세 씬을 모두 고쳤어요.", "r1")];
    rpc.agentHistory.mockImplementation(() => Promise.resolve(finished));
    await emit("agent-delta", { run_id: "r1", seq: 5, text: "세 씬을 모두 고쳤어요." });
    await emit("agent-done", { run_id: "r1", seq: 6, usage: { input: 10, output: 20 } });

    expect(screen.queryByTestId("agent-working")).toBeNull();
    expect(screen.getByTestId("agent-send")).toBeTruthy();
    // Once, not once from the stream and again from the transcript.
    expect(logText().split("세 씬을 모두 고쳤어요.")).toHaveLength(2);
    expect(screen.getByTestId("agent-usage")).toBeTruthy();
  });

  it("keeps the reason a rejoined turn ended with", async () => {
    rpc.agentHistory.mockImplementation(() => Promise.resolve(midTurn));
    rpc.agentStatus.mockImplementation(() => Promise.resolve({ running: true, run_id: "r1", started_at: 1, seq: 4 }));
    await renderPanel();

    await emit("agent-error", { run_id: "r1", seq: 5, reason: "agent_output_truncated", message: "x" });
    expect(noticeTexts()).toEqual(["errors.agentOutputTruncated"]);
    expect(screen.getByTestId("agent-send")).toBeTruthy();
  });

  it("says a conversation that stops mid-turn was interrupted when nothing is running", async () => {
    rpc.agentHistory.mockImplementation(() => Promise.resolve(midTurn));
    await renderPanel();

    expect(noticeTexts()).toEqual(["agentPanel.restore.interrupted"]);
    expect(screen.queryByTestId("agent-working")).toBeNull();
    expect(screen.getByTestId("agent-send")).toBeTruthy();
  });
});

describe("AgentPanel log order within a turn", () => {
  it("puts what the model says after a tool call under that tool line", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r2" }));
    await renderPanel();

    fireEvent.change(screen.getByTestId("agent-composer"), { target: { value: "요약해줘" } });
    await act(async () => {
      fireEvent.click(screen.getByTestId("agent-send"));
      for (let i = 0; i < 5; i++) await Promise.resolve();
    });
    expect(screen.getByTestId("agent-working")).toBeTruthy();

    await emit("agent-delta", { run_id: "r2", seq: 1, text: "먼저 읽을게요." });
    await emit("agent-tool", { run_id: "r2", seq: 2, name: "linetta_read_scene", state: "started" });
    await emit("agent-tool", { run_id: "r2", seq: 3, name: "linetta_read_scene", state: "done" });
    await emit("agent-delta", { run_id: "r2", seq: 4, text: "요약했어요." });
    await emit("agent-done", { run_id: "r2", seq: 5, usage: { input: 1, output: 2 } });

    const log = screen.getByTestId("agent-log");
    const order = Array.from(log.children).map((el) =>
      el.getAttribute("data-testid") === "tool-line" ? "tool" : el.className,
    );
    expect(order).toEqual(["msg user", "msg bot", "tool", "msg bot"]);
    const bubbles = log.querySelectorAll(".msg.bot");
    expect(bubbles[0].textContent).toContain("먼저 읽을게요.");
    expect(bubbles[1].textContent).toContain("요약했어요.");
    // The turn's cost sits under the last thing it said.
    expect(bubbles[1].querySelector('[data-testid="agent-usage"]')).toBeTruthy();
    expect(bubbles[0].querySelector('[data-testid="agent-usage"]')).toBeNull();
    expect(screen.queryByTestId("agent-working")).toBeNull();
  });
});
