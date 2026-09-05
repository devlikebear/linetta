import { act, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ProviderStatus } from "../../lib/types";

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
  agentUndo: vi.fn(),
}));

// Same approach as useMcpChanges.test.tsx: a hoisted listener map standing in
// for Tauri's real event bus, and an `emit` helper that calls the registered
// listener inside `act` so the resulting state update is flushed before the
// assertion runs.
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
  agent: { undo: rpc.agentUndo },
}));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars
        ? `${key}:${Object.entries(vars)
            .map(([name, value]) => `${name}=${value}`)
            .join(",")}`
        : key,
  }),
}));

import { AgentPanel } from "./AgentPanel";

function row(overrides: Partial<ProviderStatus> = {}): ProviderStatus {
  return {
    id: "anthropic",
    auth: "api_key",
    active: true,
    configured: false,
    consented: false,
    ...overrides,
  };
}

function renderPanel(onClose = vi.fn()) {
  return render(
    <MemoryRouter>
      <AgentPanel onClose={onClose} />
    </MemoryRouter>,
  );
}

/** Renders with a ready (configured + consented) provider and waits for the
 *  log to mount, so message tests can start emitting engine events right
 *  away. */
async function renderReady(onClose = vi.fn()) {
  rpc.providersList.mockImplementation(() =>
    Promise.resolve([row({ configured: true, consented: true })]),
  );
  const view = renderPanel(onClose);
  await screen.findByTestId("agent-log");
  return view;
}

async function emit(event: string, payload: unknown) {
  const cb = ev.listeners.get(event);
  if (!cb) throw new Error(`${event} listener was never registered`);
  await act(async () => {
    cb({ payload });
  });
}

describe("AgentPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
  });

  it("renders the unconfigured notice when no provider is active at all", async () => {
    // mockImplementation, not mockResolvedValue: a fresh object per call so a
    // second render sharing the mock cannot silently share state (#94 review).
    rpc.providersList.mockImplementation(() => Promise.resolve([]));
    renderPanel();

    expect(await screen.findByTestId("agent-unconfigured")).toBeTruthy();
    expect(screen.queryByTestId("agent-log")).toBeNull();
  });

  it("treats a configured-but-not-consented provider as not ready", async () => {
    // A credential without consent is refused server-side
    // (provider_consent_required) — telling the writer up front beats
    // sending a turn and rendering the error.
    rpc.providersList.mockImplementation(() =>
      Promise.resolve([row({ configured: true, consented: false })]),
    );
    renderPanel();

    expect(await screen.findByTestId("agent-unconfigured")).toBeTruthy();
    expect(screen.queryByTestId("agent-log")).toBeNull();
  });

  it("treats a consented-but-not-configured provider as not ready", async () => {
    rpc.providersList.mockImplementation(() =>
      Promise.resolve([row({ configured: false, consented: true })]),
    );
    renderPanel();

    expect(await screen.findByTestId("agent-unconfigured")).toBeTruthy();
  });

  it("renders the log once the active provider is configured and consented", async () => {
    rpc.providersList.mockImplementation(() =>
      Promise.resolve([row({ configured: true, consented: true })]),
    );
    renderPanel();

    expect(await screen.findByTestId("agent-log")).toBeTruthy();
    expect(screen.queryByTestId("agent-unconfigured")).toBeNull();
  });

  it("renders neither branch while the provider check is in flight", () => {
    // ready starts out unknown (tri-state), not "false" — defaulting to
    // false would flash the unconfigured notice on every open, even for a
    // fully-configured writer.
    rpc.providersList.mockImplementation(() => new Promise(() => {}));
    renderPanel();

    expect(screen.queryByTestId("agent-log")).toBeNull();
    expect(screen.queryByTestId("agent-unconfigured")).toBeNull();
  });

  it("reads readiness off the active row, not row 0, when the active row is ready", async () => {
    // Two rows; the ready one is not first. rows[0] would show the notice
    // even though the active provider is fine.
    rpc.providersList.mockImplementation(() =>
      Promise.resolve([
        row({ id: "openai", active: false, configured: false, consented: false }),
        row({ id: "anthropic", active: true, configured: true, consented: true }),
      ]),
    );
    renderPanel();

    expect(await screen.findByTestId("agent-log")).toBeTruthy();
    expect(screen.queryByTestId("agent-unconfigured")).toBeNull();
  });

  it("does not show a false-ready panel when row 0 is ready but the active row is not", async () => {
    // The direction that matters most: rows[0] is fully ready, but it is not
    // the active provider. Using rows[0] here would render the log for a
    // writer who cannot actually send a turn.
    rpc.providersList.mockImplementation(() =>
      Promise.resolve([
        row({ id: "openai", active: false, configured: true, consented: true }),
        row({ id: "anthropic", active: true, configured: false, consented: false }),
      ]),
    );
    renderPanel();

    expect(await screen.findByTestId("agent-unconfigured")).toBeTruthy();
    expect(screen.queryByTestId("agent-log")).toBeNull();
  });

  it("does not offer provider setup of its own", async () => {
    // Provider choice, key entry, and consent are #94's screen; a second
    // copy here would drift from it.
    rpc.providersList.mockImplementation(() => Promise.resolve([]));
    renderPanel();

    await screen.findByTestId("agent-unconfigured");
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  });

  it("links the unconfigured notice to Settings rather than building a picker", async () => {
    rpc.providersList.mockImplementation(() => Promise.resolve([]));
    renderPanel();

    const link = await screen.findByRole("link");
    expect(link.getAttribute("href")).toBe("/settings");
  });

  it("closes on the close button", async () => {
    rpc.providersList.mockImplementation(() => Promise.resolve([]));
    const onClose = vi.fn();
    renderPanel(onClose);

    await screen.findByTestId("agent-unconfigured");
    screen.getByRole("button", { name: "common.close" }).click();
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("stops a click inside the panel from reaching the workspace's selection handler", async () => {
    // Every other right-hand panel stops onMouseDown propagation so clicking
    // inside does not clear the workspace's selection.
    rpc.providersList.mockImplementation(() => Promise.resolve([]));
    const { container } = renderPanel();
    await screen.findByTestId("agent-unconfigured");

    const onMouseDown = vi.fn();
    document.addEventListener("mousedown", onMouseDown);
    const aside = container.querySelector(".agent-panel");
    expect(aside).toBeTruthy();
    aside!.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    document.removeEventListener("mousedown", onMouseDown);
    expect(onMouseDown).not.toHaveBeenCalled();
  });
});

describe("AgentPanel messages and streaming (#95 Task 4)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function assistantBubble() {
    return screen.getByTestId("agent-log").querySelector(".msg.bot .msg-bubble");
  }

  it("accumulates agent.delta into one reply", async () => {
    await renderReady();

    await emit("agent-delta", { run_id: "r1", text: "Hello" });
    await emit("agent-delta", { run_id: "r1", text: ", world" });
    // useSmoothStream returns the target verbatim once the run is no longer
    // active, so finishing the turn is the simplest way to assert on the
    // fully-accumulated text without driving the reveal animation by hand.
    await emit("agent-done", { run_id: "r1", usage: { input: 4, output: 6 } });

    const log = screen.getByTestId("agent-log");
    expect(log.querySelectorAll(".msg.bot")).toHaveLength(1);
    expect(assistantBubble()?.textContent).toBe("Hello, world");
  });

  it("reveals the reply a few characters at a time while the run is still streaming", async () => {
    // Same technique as useSmoothStream.test.ts: stub rAF so frames only
    // advance when this test flushes them.
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
      frames.push(cb);
      return frames.length;
    });
    vi.stubGlobal("cancelAnimationFrame", () => {});

    await renderReady();
    await emit("agent-delta", { run_id: "r1", text: "x".repeat(100) });

    // Still mid-run: the hook has not been given a chance to catch up yet.
    expect(assistantBubble()?.textContent).not.toBe("x".repeat(100));

    for (let i = 0; i < 40 && frames.length > 0; i++) {
      const due = frames.splice(0, frames.length);
      await act(async () => {
        due.forEach((cb) => cb(0));
      });
    }

    expect(assistantBubble()?.textContent).toBe("x".repeat(100));
  });

  it("ignores events from a run that is not the current one", async () => {
    await renderReady();

    await emit("agent-delta", { run_id: "r1", text: "Real reply" });
    // A second, unrelated run's events arrive interleaved — e.g. a late
    // delivery from a run the writer already abandoned. None of it may
    // touch what is on screen for r1.
    await emit("agent-tool", { run_id: "stale-run", name: "linetta_search_scenes", state: "started" });
    await emit("agent-delta", { run_id: "stale-run", text: "Ghost reply" });
    await emit("agent-done", { run_id: "stale-run", usage: { input: 9, output: 9 } });
    await emit("agent-done", { run_id: "r1", usage: { input: 1, output: 2 } });

    const log = screen.getByTestId("agent-log");
    expect(log.querySelectorAll(".msg.bot")).toHaveLength(1);
    expect(assistantBubble()?.textContent).toBe("Real reply");
  });

  it("renders markdown in a reply", async () => {
    await renderReady();

    await emit("agent-delta", {
      run_id: "r1",
      text: "**bold** and a [link](https://example.com/x)",
    });
    await emit("agent-done", { run_id: "r1", usage: { input: 1, output: 1 } });

    const bubble = assistantBubble();
    expect(bubble?.querySelector("strong")?.textContent).toBe("bold");
    const link = bubble?.querySelector("a");
    expect(link?.getAttribute("href")).toBe("https://example.com/x");
    expect(link?.getAttribute("target")).toBe("_blank");
    expect(link?.getAttribute("rel")).toBe("noreferrer");
  });

});

describe("AgentPanel tool lines and undo (#95 Task 5)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
  });

  function toolLines(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>('[data-testid="tool-line"]'));
  }

  it("merges the started and done events into one line", async () => {
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_search_manuscript", state: "started" });
    expect(toolLines()).toHaveLength(1);

    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_search_manuscript",
      state: "done",
      summary: "4-2 씬 / 스토리 컨텍스트",
    });

    // Still one line — the second event resolved the first, it did not add
    // a second row.
    const lines = toolLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].textContent).toBe("agentPanel.tool.read · 4-2 씬 / 스토리 컨텍스트");
  });

  it("shows a running tool call as still in progress, distinct from a resolved one", async () => {
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_write_scene", state: "started" });

    const running = toolLines();
    expect(running).toHaveLength(1);
    // No summary while running: the "started" event's summary is the raw
    // call arguments, not a human phrase — never render the full arguments.
    expect(running[0].textContent).toBe("agentPanel.tool.writing");
    expect(running[0].className).toContain("tool-running");

    await emit("agent-tool", { run_id: "r1", name: "linetta_write_scene", state: "done", summary: "4-2 씬" });

    const resolved = toolLines();
    expect(resolved).toHaveLength(1);
    expect(resolved[0].textContent).toBe("agentPanel.tool.wrote · 4-2 씬");
    expect(resolved[0].className).toContain("tool-ok");
  });

  it("never falls back to the started event's raw-arguments text, even if the resolving event carries no summary of its own", async () => {
    await renderReady();

    // The engine's "started" summary is the tool's raw call arguments (see
    // runTool in engine/internal/agent/loop.go) — for linetta_write_scene
    // that argument set includes the scene's full body text. A wire payload
    // carrying it is exactly the shape this line must never surface.
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_write_scene",
      state: "started",
      summary: '{"project_id":"p1","node_id":"4-2","content":"the whole scene body..."}',
    });
    // The resolving event, unusually, carries no summary of its own.
    await emit("agent-tool", { run_id: "r1", name: "linetta_write_scene", state: "done" });

    const lines = toolLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].textContent).toBe("agentPanel.tool.wrote");
    expect(lines[0].textContent).not.toContain("content");
    expect(lines[0].textContent).not.toContain("scene body");
  });

  it("shows an error tool call distinctly from a done one", async () => {
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_write_scene", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_write_scene",
      state: "error",
      summary: "version conflict",
    });

    const lines = toolLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].textContent).toBe("agentPanel.tool.writeFailed · version conflict");
    expect(lines[0].className).toContain("tool-error");
    expect(lines[0].className).not.toContain("tool-ok");
  });

  it("offers undo only on a line that carries a batch id", async () => {
    await renderReady();

    // A read tool never produces a batch id.
    await emit("agent-tool", { run_id: "r1", name: "linetta_search_manuscript", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_search_manuscript",
      state: "done",
      summary: "3 scenes",
    });

    // A write whose result carried a batch id.
    await emit("agent-tool", { run_id: "r1", name: "linetta_apply_story_ops", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_apply_story_ops",
      state: "done",
      summary: "4-2 씬",
      batch_id: "batch-1",
    });

    // The writer stopped this turn mid-commit: the write happened (this is
    // still a write-tool line), but no batch id came back — nothing to
    // undo it with, so no button.
    await emit("agent-tool", { run_id: "r1", name: "linetta_write_scene", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_write_scene",
      state: "error",
      summary: "the writer stopped this turn",
    });

    const lines = toolLines();
    expect(lines).toHaveLength(3);
    expect(lines[0].querySelector("button")).toBeNull();
    expect(lines[1].querySelector("button")?.textContent).toBe("agentPanel.tool.undo");
    expect(lines[2].querySelector("button")).toBeNull();
  });

  it("marks the line undone after agent.undo succeeds", async () => {
    rpc.agentUndo.mockImplementation(() => Promise.resolve({ ok: true }));
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_apply_story_ops", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_apply_story_ops",
      state: "done",
      summary: "4-2 씬",
      batch_id: "batch-1",
    });

    const button = screen.getByRole("button", { name: "agentPanel.tool.undo" });
    await act(async () => {
      button.click();
    });

    expect(rpc.agentUndo).toHaveBeenCalledWith("batch-1");
    expect(screen.queryByRole("button", { name: "agentPanel.tool.undo" })).toBeNull();
    expect(toolLines()[0].textContent).toBe("agentPanel.tool.wrote · 4-2 씬agentPanel.tool.undone");
  });

  it("explains an expired undo window instead of failing silently", async () => {
    // agent_undo_unavailable: the service keeps only the last 8 undo
    // batches, so this is the ordinary result of a restart or a few more
    // turns, not a mistake the writer made (#94 already mapped and
    // translated this reason code).
    rpc.agentUndo.mockImplementation(() => Promise.reject({ data: { reason: "agent_undo_unavailable" } }));
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_apply_story_ops", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_apply_story_ops",
      state: "done",
      summary: "4-2 씬",
      batch_id: "batch-1",
    });

    const button = screen.getByRole("button", { name: "agentPanel.tool.undo" });
    await act(async () => {
      button.click();
    });

    // The button is gone — undo is not retryable here — and the translated
    // reason renders beside the line, contiguous with the rest of it.
    expect(screen.queryByRole("button", { name: "agentPanel.tool.undo" })).toBeNull();
    expect(toolLines()[0].textContent).toBe(
      "agentPanel.tool.wrote · 4-2 씬 errors.agentUndoUnavailable",
    );
    // This must not have escalated into a panel-level error: the log (and
    // this line within it) is still the thing on screen.
    expect(screen.getByTestId("agent-log")).toBeTruthy();
    expect(screen.queryByTestId("agent-unconfigured")).toBeNull();
  });
});
