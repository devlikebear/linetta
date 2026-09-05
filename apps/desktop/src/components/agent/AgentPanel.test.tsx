import { StrictMode } from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readSource } from "../../test/readSource";
import type { AgentHistoryRow, ProviderStatus } from "../../lib/types";

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
  agentUndo: vi.fn(),
  agentRun: vi.fn(),
  agentCancel: vi.fn(),
  agentHistory: vi.fn(),
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
  agent: { undo: rpc.agentUndo, run: rpc.agentRun, cancel: rpc.agentCancel, history: rpc.agentHistory },
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

/** The text of one function's body, braces included, for the few assertions
 *  that have to read source (see the mount-ref test below).
 *
 *  Brace counting, not a slice to the next declaration: the boundary is the
 *  function's own, so nothing about what sits after it in the file is pinned.
 *  It does not know about braces inside strings, template literals or
 *  comments — fine for the balanced TSX bodies asserted on here, and it
 *  throws rather than returning a wrong span if that ever stops holding. */
function functionBody(src: string, signature: string): string {
  const start = src.indexOf(signature);
  if (start === -1) throw new Error(`${signature} not found in source`);
  const open = src.indexOf("{", start);
  if (open === -1) throw new Error(`${signature} has no body`);
  let depth = 0;
  for (let i = open; i < src.length; i++) {
    if (src[i] === "{") depth++;
    else if (src[i] === "}" && --depth === 0) return src.slice(open, i + 1);
  }
  throw new Error(`unbalanced braces after ${signature}`);
}

const PROJECT_ID = "project-1";
const NODE_ID = "node-1";

function renderPanel(onClose = vi.fn()) {
  return render(
    <MemoryRouter>
      <AgentPanel onClose={onClose} projectId={PROJECT_ID} nodeId={NODE_ID} />
    </MemoryRouter>,
  );
}

function composer() {
  return screen.getByTestId("agent-composer") as HTMLTextAreaElement;
}

/** Types into the composer and presses the given key. Two `act`s — a change
 *  then a keydown — not one, so a controlled-input update and its listener
 *  registration both land before the assertion runs. */
async function type(text: string) {
  await act(async () => {
    fireEvent.change(composer(), { target: { value: text } });
  });
}

async function pressEnter(shiftKey = false) {
  await act(async () => {
    fireEvent.keyDown(composer(), { key: "Enter", shiftKey });
  });
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

/** Emits one wire event.
 *
 *  Every agent.* payload carries `project_id` (#95 Task 7 review round 3) and
 *  the panel refuses anything that does not name the work it is showing, so a
 *  payload written without one gets PROJECT_ID — the project every test here
 *  renders at unless it says otherwise. The tests about one work's events
 *  reaching another's panel pass it explicitly, which is the only way to write
 *  the leak they are pinning. */
async function emit(event: string, payload: unknown) {
  const cb = ev.listeners.get(event);
  if (!cb) throw new Error(`${event} listener was never registered`);
  const named =
    payload && typeof payload === "object" && !("project_id" in payload)
      ? { project_id: PROJECT_ID, ...payload }
      : payload;
  await act(async () => {
    cb({ payload: named });
  });
}

describe("AgentPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    // Task 7 adds an agent.history call on mount. Every existing test in this
    // file renders a panel that neither sets up nor cares about restored
    // history, so it gets an empty conversation by default; tests that do
    // care override this before rendering.
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
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
    // Task 7 adds an agent.history call on mount. Every existing test in this
    // file renders a panel that neither sets up nor cares about restored
    // history, so it gets an empty conversation by default; tests that do
    // care override this before rendering.
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
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
    // Task 7 adds an agent.history call on mount. Every existing test in this
    // file renders a panel that neither sets up nor cares about restored
    // history, so it gets an empty conversation by default; tests that do
    // care override this before rendering.
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
  });

  function toolLines(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>('[data-testid="tool-line"]'));
  }

  // What the engine actually puts in agent.tool's `summary`. Not invented:
  // loop.go's runTool sets it to summarize(call.Arguments) on "started" and
  // summarize(result.Text) on the resolving event, and result.Text is the
  // go-sdk's JSON serialisation of the tool's typed output — every Linetta
  // tool returns a nil *CallToolResult, so the SDK marshals the struct. None
  // of this may reach a writer's screen.
  const ENGINE_ARGS_SUMMARY =
    '{"node_id":"01JQ8Z","text":"비가 내렸다. 그는 우산을 펴지 않았다. 골목 끝에서 누군가 그를 불렀다.","expected_content_version":3}';
  const ENGINE_RESULT_SUMMARY =
    '{"project_id":"01JQ8Y0","node_id":"01JQ8Z1","scene_label":"4-2","brief":"주인공이 기록보관실에서 편지를 발견한다. 그 편지는';

  it("merges the started and done events into one line", async () => {
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_search_manuscript", state: "started" });
    expect(toolLines()).toHaveLength(1);

    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_search_manuscript",
      state: "done",
      summary: ENGINE_RESULT_SUMMARY,
    });

    // Still one line — the second event resolved the first, it did not add
    // a second row.
    const lines = toolLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].textContent).toBe("agentPanel.tool.read · agentPanel.toolName.linetta_search_manuscript");
  });

  it("names the tool in the writer's language and never shows the engine's summary", async () => {
    // The design spec's own example for this tool is "읽음 · 스토리 컨텍스트".
    // Rendering `summary` gave the writer a project id, a node id and the
    // opening prose of the brief instead, wrapped over several lines of a
    // narrow rail.
    await renderReady();

    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_get_story_context",
      state: "started",
      summary: '{"project_id":"01JQ8Y0","node_id":"01JQ8Z1"}',
    });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_get_story_context",
      state: "done",
      summary: ENGINE_RESULT_SUMMARY,
      node_ids: ["01JQ8Z1"],
    });

    const text = toolLines()[0].textContent ?? "";
    expect(text).toBe("agentPanel.tool.read · agentPanel.toolName.linetta_get_story_context");
    // Spelled out, because "it happens to equal the string above" would still
    // hold if the label key itself started carrying JSON.
    for (const leak of ["{", "project_id", "node_id", "scene_label", "brief", "01JQ8Z1"]) {
      expect(text, `engine JSON fragment ${leak} reached the DOM`).not.toContain(leak);
    }
  });

  it("does not show the engine's English failure sentence to a Korean writer", async () => {
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_write_scene", state: "started" });
    // agent/tools.go's toolSession.call writes this sentence itself when the
    // turn is cancelled mid-call. It is engine English, not a translated
    // string, and it must not land beside a Korean verb.
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_write_scene",
      state: "error",
      summary: "the writer stopped this turn",
    });

    const text = toolLines()[0].textContent ?? "";
    expect(text).toBe("agentPanel.tool.writeFailed · agentPanel.toolName.linetta_write_scene");
    expect(text).not.toContain("the writer stopped");
  });

  it("claims neither a read nor a write for a tool name it does not recognise", async () => {
    // An unrecognised name is most likely a tool the engine gained and
    // lib/agentTools.ts did not. Calling it "읽음" would tell the writer their
    // manuscript was untouched by something that may have deleted a chapter;
    // calling it "씀" would send them hunting for an edit that never
    // happened. The line claims only what is known: a tool ran.
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_delete_chapter", state: "started" });
    expect(toolLines()[0].textContent).toBe("agentPanel.tool.using");

    await emit("agent-tool", { run_id: "r1", name: "linetta_delete_chapter", state: "done", summary: "{}" });
    expect(toolLines()[0].textContent).toBe("agentPanel.tool.used");
    expect(toolLines()[0].textContent).not.toContain("agentPanel.tool.read");
  });

  it("shows a running tool call as still in progress, distinct from a resolved one", async () => {
    await renderReady();

    // The "started" event carries the tool's raw call arguments — for a scene
    // write, the whole body. The line is built from the tool name alone, so
    // none of it can appear whatever the state.
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_write_scene",
      state: "started",
      summary: ENGINE_ARGS_SUMMARY,
    });

    const running = toolLines();
    expect(running).toHaveLength(1);
    expect(running[0].textContent).toBe("agentPanel.tool.writing · agentPanel.toolName.linetta_write_scene");
    expect(running[0].textContent).not.toContain("비가 내렸다");
    expect(running[0].className).toContain("tool-running");

    await emit("agent-tool", { run_id: "r1", name: "linetta_write_scene", state: "done", summary: "{}" });

    const resolved = toolLines();
    expect(resolved).toHaveLength(1);
    expect(resolved[0].textContent).toBe("agentPanel.tool.wrote · agentPanel.toolName.linetta_write_scene");
    expect(resolved[0].className).toContain("tool-ok");
  });

  it("never falls back to the started event's raw-arguments text, even if the resolving event carries no summary of its own", async () => {
    await renderReady();

    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_write_scene",
      state: "started",
      summary: ENGINE_ARGS_SUMMARY,
    });
    // The resolving event, unusually, carries no summary of its own.
    await emit("agent-tool", { run_id: "r1", name: "linetta_write_scene", state: "done" });

    const lines = toolLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].textContent).toBe("agentPanel.tool.wrote · agentPanel.toolName.linetta_write_scene");
    expect(lines[0].textContent).not.toContain("비가 내렸다");
  });

  it("shows an error tool call distinctly from a done one", async () => {
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_write_scene", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_write_scene",
      state: "error",
      summary: '{"error":"version conflict"}',
    });

    const lines = toolLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].textContent).toBe("agentPanel.tool.writeFailed · agentPanel.toolName.linetta_write_scene");
    expect(lines[0].className).toContain("tool-error");
    expect(lines[0].className).not.toContain("tool-ok");
  });

  it("drops tool events from a run that is not the current one", async () => {
    // Task 4's stale-run test asserts only on .msg.bot, so removing
    // acceptRun's guard from the agent-tool handler passed every test until
    // this one. It matters more now that tool lines are visible: a late
    // "done" from an abandoned run would append a row under the current
    // run's transcript carrying the OLD run's batch id — a live undo button
    // for a batch the writer is no longer looking at. Task 6's composer is
    // explicitly required to modify currentRunIdRef, so this needs pinning
    // before that lands.
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_search_manuscript", state: "started" });
    await emit("agent-tool", { run_id: "r1", name: "linetta_search_manuscript", state: "done", summary: "{}" });

    await emit("agent-tool", { run_id: "abandoned", name: "linetta_apply_story_ops", state: "started" });
    await emit("agent-tool", {
      run_id: "abandoned",
      name: "linetta_apply_story_ops",
      state: "done",
      summary: "{}",
      batch_id: "batch-from-an-abandoned-run",
    });

    const lines = toolLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].textContent).toBe("agentPanel.tool.read · agentPanel.toolName.linetta_search_manuscript");
    expect(screen.queryByRole("button", { name: "agentPanel.tool.undo" })).toBeNull();
  });

  it("offers undo only on a line that carries a batch id", async () => {
    await renderReady();

    // A read tool never produces a batch id.
    await emit("agent-tool", { run_id: "r1", name: "linetta_search_manuscript", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_search_manuscript",
      state: "done",
      summary: "{}",
    });

    // A write whose result carried a batch id.
    await emit("agent-tool", { run_id: "r1", name: "linetta_apply_story_ops", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_apply_story_ops",
      state: "done",
      summary: "{}",
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

    // And each still says honestly what kind of call it was.
    expect(lines[0].textContent).toBe("agentPanel.tool.read · agentPanel.toolName.linetta_search_manuscript");
    expect(lines[1].textContent).toBe(
      "agentPanel.tool.wrote · agentPanel.toolName.linetta_apply_story_opsagentPanel.tool.undo",
    );
    expect(lines[2].textContent).toBe("agentPanel.tool.writeFailed · agentPanel.toolName.linetta_write_scene");
  });

  /** Emits one apply_story_ops call that resolved with a batch id, so the
   *  line under test has an undo button. */
  async function lineWithUndoButton() {
    await emit("agent-tool", { run_id: "r1", name: "linetta_apply_story_ops", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_apply_story_ops",
      state: "done",
      summary: "{}",
      batch_id: "batch-1",
    });
    return screen.getByRole("button", { name: "agentPanel.tool.undo" });
  }

  it("marks the line undone after agent.undo succeeds", async () => {
    rpc.agentUndo.mockImplementation(() => Promise.resolve({ ok: true }));
    await renderReady();

    const button = await lineWithUndoButton();
    await act(async () => {
      button.click();
    });

    expect(rpc.agentUndo).toHaveBeenCalledWith("batch-1");
    expect(screen.queryByRole("button", { name: "agentPanel.tool.undo" })).toBeNull();
    expect(toolLines()[0].textContent).toBe(
      "agentPanel.tool.wrote · agentPanel.toolName.linetta_apply_story_opsagentPanel.tool.undone",
    );
  });

  it("sends only one agent.undo when the button is clicked twice", async () => {
    // storyops' takeUndoBatch deletes the batch on first use, so a genuine
    // double-fire undoes the batch and then reports agent_undo_unavailable
    // for it — an error message on a line that was just undone successfully.
    let release = () => {};
    rpc.agentUndo.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    );
    await renderReady();

    const button = await lineWithUndoButton();
    // Two acts, not two clicks in one: a real double-click is two separate
    // tasks with a render between them, which is exactly when `disabled` gets
    // its chance to apply. Both clicks inside one act would be batched and
    // could not tell a guarded button from an unguarded one.
    await act(async () => {
      button.click();
    });
    await act(async () => {
      button.click();
    });

    expect(rpc.agentUndo).toHaveBeenCalledTimes(1);
    await act(async () => {
      release();
    });
    expect(toolLines()[0].textContent).toContain("agentPanel.tool.undone");
  });

  it("explains an expired undo window instead of failing silently", async () => {
    // agent_undo_unavailable: the service keeps only the last 8 undo
    // batches, so this is the ordinary result of a restart or a few more
    // turns, not a mistake the writer made (#94 already mapped and
    // translated this reason code).
    rpc.agentUndo.mockImplementation(() => Promise.reject({ data: { reason: "agent_undo_unavailable" } }));
    await renderReady();

    const button = await lineWithUndoButton();
    await act(async () => {
      button.click();
    });

    // The button is gone — undo is not retryable here — and the translated
    // reason renders beside the line, contiguous with the rest of it.
    expect(screen.queryByRole("button", { name: "agentPanel.tool.undo" })).toBeNull();
    expect(toolLines()[0].textContent).toBe(
      "agentPanel.tool.wrote · agentPanel.toolName.linetta_apply_story_ops errors.agentUndoUnavailable",
    );
    // This must not have escalated into a panel-level error: the log (and
    // this line within it) is still the thing on screen.
    expect(screen.getByTestId("agent-log")).toBeTruthy();
    expect(screen.queryByTestId("agent-unconfigured")).toBeNull();
  });

  it("does not write state back after the writer closes the panel mid-undo", async () => {
    // handleUndo resolves outside any effect, so it cannot use the sibling
    // provider effect's `cancelled` flag; it checks a mount ref instead.
    // Nothing observable distinguishes the two in React 18 — a setState on an
    // unmounted component is silently dropped — so this is asserted the way
    // this codebase asserts other unobservable source facts (see
    // rpcAllowlist.test.ts, coreRoleParity.test.ts).
    //
    // Bounded by handleUndo's own braces, not by whatever function happens to
    // follow it: an earlier revision sliced to the next `useEffect`, which
    // silently pinned handleUndo's position in the file, forbade any other
    // mountedRef-guarding function from being written between the two, and
    // depended on that unrelated effect's exact indentation. Handlers had to
    // be moved away from where they read best to keep it green.
    const src = await readSource("components/agent/AgentPanel.tsx");
    const body = functionBody(src, "function handleUndo(");
    // Both resolution paths, not just the happy one.
    expect(body.match(/if \(!mountedRef\.current\) return;/g) ?? []).toHaveLength(2);
    // Set on mount, not only at declaration: a StrictMode remount runs the
    // cleanup and would otherwise leave the panel permanently "unmounted".
    expect(src).toContain("mountedRef.current = true;");
  });
});

describe("AgentPanel composer, stop, starters, and usage (#95 Task 6)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    // Task 7 adds an agent.history call on mount. Every existing test in this
    // file renders a panel that neither sets up nor cares about restored
    // history, so it gets an empty conversation by default; tests that do
    // care override this before rendering.
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
  });

  function botBubbles(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>(".msg.bot .msg-bubble"));
  }

  it("sends on Enter with the project id, the prompt, and the open editor's node id", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();

    await type("다음 문단을 이어서 써줘");
    await pressEnter();

    expect(rpc.agentRun).toHaveBeenCalledWith(PROJECT_ID, "다음 문단을 이어서 써줘", NODE_ID);
    // The composer clears and the writer's own line lands in the log.
    expect(composer().value).toBe("");
    expect(screen.getByTestId("agent-log").querySelector(".msg.user .msg-bubble")?.textContent).toBe(
      "다음 문단을 이어서 써줘",
    );
  });

  it("does not send on Shift+Enter", async () => {
    // jsdom does not simulate a browser's native newline-on-Enter behaviour
    // for a plain keydown dispatch, so this cannot assert a "\n" landed in
    // the textarea — only that Shift+Enter is not treated as send (the
    // draft survives untouched, and no run starts).
    await renderReady();

    await type("one");
    await pressEnter(true);

    expect(rpc.agentRun).not.toHaveBeenCalled();
    expect(composer().value).toBe("one");
  });

  it("does not send an empty or whitespace-only draft", async () => {
    await renderReady();

    await type("   ");
    await pressEnter();

    expect(rpc.agentRun).not.toHaveBeenCalled();
  });

  it("swaps the send control for stop once a run starts, and back once it finishes", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();

    expect(screen.getByTestId("agent-send")).toBeTruthy();
    expect(screen.queryByTestId("agent-stop")).toBeNull();

    await type("써줘");
    await pressEnter();

    expect(screen.queryByTestId("agent-send")).toBeNull();
    expect(screen.getByTestId("agent-stop")).toBeTruthy();

    await emit("agent-done", { run_id: "r1", usage: { input: 1, output: 1 } });

    expect(screen.getByTestId("agent-send")).toBeTruthy();
    expect(screen.queryByTestId("agent-stop")).toBeNull();
  });

  it("cancels the running turn through agent.cancel and keeps the partial reply on screen", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    rpc.agentCancel.mockImplementation(() => Promise.resolve({ ok: true }));
    await renderReady();

    await type("써줘");
    await pressEnter();
    await emit("agent-delta", { run_id: "r1", text: "여기까지 쓰다가" });

    await act(async () => {
      screen.getByTestId("agent-stop").click();
    });
    expect(rpc.agentCancel).toHaveBeenCalledWith("r1");

    // #93's contract: cancel reaches the in-memory turn, but the partial
    // reply already accumulated stays in the transcript — the composer
    // clearing it would throw away work the writer may want.
    await emit("agent-cancelled", { run_id: "r1" });

    expect(botBubbles()[0]?.textContent).toBe("여기까지 쓰다가");
    expect(screen.getByTestId("agent-send")).toBeTruthy();
  });

  it("renders the translated reason for a synchronous refusal and does not leave the composer stuck on stop", async () => {
    rpc.agentRun.mockImplementation(() => Promise.reject({ data: { reason: "provider_consent_required" } }));
    await renderReady();

    await type("이어서 써줘");
    await pressEnter();

    expect(screen.getByTestId("agent-send-error").textContent).toBe("errors.providerConsentRequired");
    // Never the raw engine message, and never stuck showing stop for a turn
    // that was refused before it ever started.
    expect(screen.queryByTestId("agent-stop")).toBeNull();
    expect(screen.getByTestId("agent-send")).toBeTruthy();
    // The draft is handed back rather than thrown away.
    expect(composer().value).toBe("이어서 써줘");
    // And no phantom turn was added to the log.
    expect(screen.queryByTestId("agent-log")?.querySelector(".msg.user")).toBeNull();
  });

  it("fills the composer from each starting chip without sending", async () => {
    await renderReady();

    const draftChip = screen.getByTestId("agent-starter-draftScene");
    await act(async () => {
      draftChip.click();
    });

    expect(rpc.agentRun).not.toHaveBeenCalled();
    expect(composer().value).toBe("agentPanel.starters.draftScene.prompt");

    await act(async () => {
      screen.getByTestId("agent-starter-continuity").click();
    });
    expect(composer().value).toBe("agentPanel.starters.continuity.prompt");

    await act(async () => {
      screen.getByTestId("agent-starter-nextScene").click();
    });
    expect(composer().value).toBe("agentPanel.starters.nextScene.prompt");
  });

  it("hides the starting chips once the conversation has a turn in it", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();

    expect(screen.getByTestId("agent-starters")).toBeTruthy();

    await type("써줘");
    await pressEnter();

    expect(screen.queryByTestId("agent-starters")).toBeNull();
  });

  it("shows the turn's token usage after agent.done, with no cost figure", async () => {
    await renderReady();

    await emit("agent-delta", { run_id: "r1", text: "답변" });
    await emit("agent-done", { run_id: "r1", usage: { input: 120, output: 340 } });

    // getAll, not get: the usage line is per finished turn, the same way
    // `tool-line` is per tool call, so a conversation with two turns has two
    // of these. Asserting with getByTestId here would pass today and throw
    // "Found multiple elements" in the next test that sends twice.
    const usage = screen.getAllByTestId("agent-usage");
    expect(usage).toHaveLength(1);
    expect(usage[0].textContent).toBe("agentPanel.usage:input=120,output=340");
    expect(usage[0].textContent).not.toContain("$");
  });

  it("renders the second turn's reply after the first is done, in the same mount", async () => {
    // The scenario the acceptRun comment calls out by name: without setting
    // currentRunIdRef.current from agent.run's own response, the second
    // turn's events arrive before anything else names r2 as current, get
    // read as stragglers from an abandoned run, and are dropped — the
    // second reply never appears.
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();

    await type("첫 번째 요청");
    await pressEnter();
    await emit("agent-delta", { run_id: "r1", text: "첫 번째 답" });
    await emit("agent-done", { run_id: "r1", usage: { input: 1, output: 1 } });

    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r2" }));
    await type("두 번째 요청");
    await pressEnter();
    await emit("agent-delta", { run_id: "r2", text: "두 번째 답" });
    await emit("agent-done", { run_id: "r2", usage: { input: 2, output: 2 } });

    const bubbles = botBubbles();
    expect(bubbles).toHaveLength(2);
    expect(bubbles[0].textContent).toBe("첫 번째 답");
    expect(bubbles[1].textContent).toBe("두 번째 답");
  });
});

/** A promise whose settlement this test controls, standing in for the
 *  agent.run round trip. Everything between `agentRun` being called and this
 *  resolving is the "send window": the engine has already minted the run id
 *  and started emitting for it, but the renderer does not know it yet. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("AgentPanel send window and IME composition (#95 Task 6 review)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    // Task 7 adds an agent.history call on mount. Every existing test in this
    // file renders a panel that neither sets up nor cares about restored
    // history, so it gets an empty conversation by default; tests that do
    // care override this before rendering.
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
  });

  function botBubbles(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>(".msg.bot .msg-bubble"));
  }

  /** Runs one complete turn as r1, so the panel's run ref is pointed at a
   *  finished run — the state every send-window hazard needs. */
  async function firstTurn(text = "완성된 첫 답") {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();
    await type("첫 번째 요청");
    await pressEnter();
    await emit("agent-delta", { run_id: "r1", text });
    await emit("agent-done", { run_id: "r1", usage: { input: 1, output: 1 } });
  }

  it("does not send the Enter that confirms an IME conversion candidate", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();
    await type("つづきをかいて");
    await act(async () => {
      fireEvent.keyDown(composer(), { key: "Enter", isComposing: true });
    });
    expect(rpc.agentRun).not.toHaveBeenCalled();
    expect(composer().value).toBe("つづきをかいて");
  });

  it("does not send the Enter that commits a composition reported only as keyCode 229", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();
    await type("이어서 써줘");
    await act(async () => {
      fireEvent.keyDown(composer(), { key: "Enter", keyCode: 229 });
    });
    expect(rpc.agentRun).not.toHaveBeenCalled();
    expect(composer().value).toBe("이어서 써줘");
  });

  it("keeps the chunk that arrived before agent.run's response", async () => {
    await firstTurn();
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);

    await type("두 번째 요청");
    await pressEnter();
    await emit("agent-delta", { run_id: "r2", text: "두 번째 " });
    await act(async () => {
      pending.resolve({ run_id: "r2" });
    });
    await emit("agent-delta", { run_id: "r2", text: "조각." });
    await emit("agent-done", { run_id: "r2", usage: { input: 2, output: 2 } });

    expect(botBubbles().map((b) => b.textContent)).toEqual(["완성된 첫 답", "두 번째 조각."]);
  });

  it("does not append a straggler from the finished turn to its bubble", async () => {
    await firstTurn();
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);

    await type("두 번째 요청");
    await pressEnter();
    await emit("agent-delta", { run_id: "r1", text: "  <<유령>>" });
    await act(async () => {
      pending.resolve({ run_id: "r2" });
    });
    await emit("agent-delta", { run_id: "r2", text: "두 번째 답" });
    await emit("agent-done", { run_id: "r2", usage: { input: 2, output: 2 } });

    expect(botBubbles().map((b) => b.textContent)).toEqual(["완성된 첫 답", "두 번째 답"]);
  });

  it("returns the composer to send when the turn fails before agent.run's response", async () => {
    await firstTurn();
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);

    await type("두 번째 요청");
    await pressEnter();
    await emit("agent-error", { run_id: "r2", reason: "provider_unreachable", message: "dial tcp" });
    await act(async () => {
      pending.resolve({ run_id: "r2" });
    });

    expect(screen.queryByTestId("agent-stop")).toBeNull();
    expect(screen.getByTestId("agent-send")).toBeTruthy();
  });

  it("shows a usage line per finished turn", async () => {
    await firstTurn();
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r2" }));
    await type("두 번째 요청");
    await pressEnter();
    await emit("agent-delta", { run_id: "r2", text: "두 번째 답" });
    await emit("agent-done", { run_id: "r2", usage: { input: 20, output: 40 } });

    expect(screen.getAllByTestId("agent-usage").map((n) => n.textContent)).toEqual([
      "agentPanel.usage:input=1,output=1",
      "agentPanel.usage:input=20,output=40",
    ]);
  });

  it("gives the composer an accessible name, not just a placeholder", async () => {
    await renderReady();
    expect(screen.getByRole("textbox", { name: "agentPanel.composer.label" })).toBe(composer());
  });

  it("surfaces a cancel that actually failed while the turn is still running", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    rpc.agentCancel.mockImplementation(() => Promise.reject(new Error("engine did not respond")));
    await renderReady();
    await type("써줘");
    await pressEnter();

    await act(async () => {
      screen.getByTestId("agent-stop").click();
    });

    expect(screen.getByTestId("agent-send-error").textContent).toContain("engine did not respond");
    // The turn never ended, so the control is still stop.
    expect(screen.getByTestId("agent-stop")).toBeTruthy();
  });
});

function historyRow(overrides: Partial<AgentHistoryRow> = {}): AgentHistoryRow {
  return {
    id: "h1",
    project_id: PROJECT_ID,
    role: "user",
    status: "done",
    content: "",
    created_at: 1,
    ...overrides,
  };
}

describe("AgentPanel history restore (#95 Task 7)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
  });

  function toolLines(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>('[data-testid="tool-line"]'));
  }

  function notices(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>('[data-testid="agent-notice"]'));
  }

  it("restores a saved conversation from agent.history when the panel opens", async () => {
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", role: "user", content: "이어서 써줘" }),
        historyRow({
          id: "h2",
          role: "tool",
          content: JSON.stringify({
            name: "linetta_search_manuscript",
            summary: '{"raw":"engine json the writer must never see"}',
            ok: true,
          }),
        }),
        historyRow({ id: "h3", role: "assistant", content: "여기까지 썼습니다." }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("이어서 써줘")).toBeTruthy();
    const log = screen.getByTestId("agent-log");
    expect(log.querySelector(".msg.user .msg-bubble")?.textContent).toBe("이어서 써줘");
    expect(log.querySelector(".msg.bot .msg-bubble")?.textContent).toBe("여기까지 썼습니다.");

    const lines = toolLines();
    expect(lines).toHaveLength(1);
    // Same rendering as a live tool line: verb + translated label, never the
    // persisted `summary` field, whatever it was written as.
    expect(lines[0].textContent).toBe("agentPanel.tool.read · agentPanel.toolName.linetta_search_manuscript");
    expect(lines[0].textContent).not.toContain("raw");
    expect(lines[0].textContent).not.toContain("engine json");
  });

  it("shows a restored tool call as failed when its ok flag is false", async () => {
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", role: "user", content: "확인용" }),
        historyRow({
          id: "h2",
          role: "tool",
          content: JSON.stringify({ name: "linetta_write_scene", ok: false }),
        }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("확인용")).toBeTruthy();
    const lines = toolLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].textContent).toBe("agentPanel.tool.writeFailed · agentPanel.toolName.linetta_write_scene");
  });

  it("carries a restored tool call's batch id, so undo still works after a restart", async () => {
    rpc.agentUndo.mockImplementation(() => Promise.resolve({ ok: true }));
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({
          id: "h1",
          role: "tool",
          content: JSON.stringify({ name: "linetta_apply_story_ops", ok: true, batch_id: "batch-restored" }),
        }),
      ]),
    );
    await renderReady();

    const button = await screen.findByRole("button", { name: "agentPanel.tool.undo" });
    await act(async () => {
      button.click();
    });

    expect(rpc.agentUndo).toHaveBeenCalledWith("batch-restored");
  });

  it("skips a tool row that fails to parse, without blanking the rest of the restored conversation", async () => {
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", role: "user", content: "첫 요청" }),
        historyRow({ id: "h2", role: "tool", content: "{not valid json" }),
        historyRow({ id: "h3", role: "assistant", content: "그래도 답은 옵니다" }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("첫 요청")).toBeTruthy();
    const log = screen.getByTestId("agent-log");
    expect(log.querySelector(".msg.bot .msg-bubble")?.textContent).toBe("그래도 답은 옵니다");
    expect(screen.queryAllByTestId("tool-line")).toHaveLength(0);
  });

  it("skips a tool row whose JSON is well-formed but missing the fields a toolEvent needs", async () => {
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", role: "user", content: "확인용" }),
        historyRow({ id: "h2", role: "tool", content: JSON.stringify({ summary: "no name or ok field" }) }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("확인용")).toBeTruthy();
    expect(screen.queryAllByTestId("tool-line")).toHaveLength(0);
  });

  it("shows a notice that a turn may still be running when the restored conversation's last row is a user turn", async () => {
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([historyRow({ id: "h1", role: "user", content: "아직 안 끝났을 수도" })]),
    );
    await renderReady();

    expect(await screen.findByText("아직 안 끝났을 수도")).toBeTruthy();
    const found = notices();
    expect(found).toHaveLength(1);
    expect(found[0].textContent).toBe("agentPanel.restore.mayBeRunning");
  });

  it("does not show the still-running notice once the conversation already has a reply", async () => {
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", role: "user", content: "요청" }),
        historyRow({ id: "h2", role: "assistant", content: "완료된 답" }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("완료된 답")).toBeTruthy();
    expect(notices()).toHaveLength(0);
  });

  it("does not show the still-running notice when there is no history at all", async () => {
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
    await renderReady();
    // Give the (empty) restore effect a turn to resolve.
    await act(async () => {});

    expect(notices()).toHaveLength(0);
  });
});

describe("AgentPanel wire errors and cancellation (#95 Task 7)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
  });

  function notices(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>('[data-testid="agent-notice"]'));
  }

  it("renders a translated notice for agent.error and never shows the raw engine message", async () => {
    await renderReady();

    await emit("agent-error", {
      run_id: "r1",
      reason: "provider_unreachable",
      // agent_internal_error carries a raw Go panic here; nothing tied to
      // `message` may ever reach the DOM regardless of the reason.
      message: "dial tcp 1.2.3.4:443: connect: connection refused",
    });

    const found = notices();
    expect(found).toHaveLength(1);
    expect(found[0].textContent).toBe("errors.providerUnreachable");
    expect(found[0].textContent).not.toContain("dial tcp");
  });

  it("gives provider_auth_failed a link to Settings, the same shape as the unconfigured notice", async () => {
    await renderReady();

    await emit("agent-error", { run_id: "r1", reason: "provider_auth_failed", message: "401 unauthorized" });

    const notice = notices()[0];
    expect(notice.textContent).toBe("errors.providerAuthFailed agentPanel.openSettings");
    const link = notice.querySelector("a");
    expect(link?.getAttribute("href")).toBe("/settings");
    expect(notice.textContent).not.toContain("401");
  });

  it("keeps a panic value out of the log for agent_internal_error", async () => {
    // Retargeted from agent_busy in the Task 7 review: agent_busy is only ever
    // a synchronous rejection of agent.run (loop.go's Run), never an
    // agent.error notification, and the sendError banner already covers that
    // path. agent_internal_error is a reason the loop really does notify —
    // and the one whose `message` carries a raw Go panic value.
    await renderReady();

    await emit("agent-error", {
      run_id: "r1",
      reason: "agent_internal_error",
      message: "internal error: runtime error: index out of range [3] with length 3",
    });

    expect(notices()[0].textContent).toBe("errors.agentInternalError");
    expect(notices()[0].textContent).not.toContain("index out of range");
  });

  it("degrades to an honest sentence for a reason code the panel does not know", async () => {
    // Latent today — every reason agent.error can carry is mapped — but the
    // fallback must not be `String({data:{reason}})`, which puts the literal
    // text "[object Object]" in front of the writer.
    await renderReady();

    await emit("agent-error", { run_id: "r1", reason: "not_mapped_yet", message: "raw engine sentence" });

    const notice = notices()[0];
    expect(notice.textContent).toBe("errors.unexpectedReason:reason=not_mapped_yet");
    expect(notice.textContent).not.toContain("[object Object]");
    expect(notice.textContent).not.toContain("raw engine sentence");
  });

  it("keeps the partial reply on screen and says the work is kept for agent_iteration_limit", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();
    await type("길게 써줘");
    await pressEnter();
    await emit("agent-delta", { run_id: "r1", text: "여기까지 진행했습니다" });

    await emit("agent-error", {
      run_id: "r1",
      reason: "agent_iteration_limit",
      message: "stopped after 24 tool calls in one turn",
    });

    const log = screen.getByTestId("agent-log");
    expect(log.querySelector(".msg.bot .msg-bubble")?.textContent).toBe("여기까지 진행했습니다");
    expect(notices()[0].textContent).toBe("errors.agentIterationLimit");
    // The composer is handed back — the writer can send a follow-up.
    expect(screen.getByTestId("agent-send")).toBeTruthy();
  });

  it("ignores an agent.error from a run that is not the current one", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();
    await type("써줘");
    await pressEnter();

    await emit("agent-error", { run_id: "stale", reason: "provider_unreachable", message: "x" });

    expect(notices()).toHaveLength(0);
    // The real turn is untouched.
    expect(screen.getByTestId("agent-stop")).toBeTruthy();
  });

  it("renders a notice when the turn is cancelled, and keeps the partial reply on screen", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    rpc.agentCancel.mockImplementation(() => Promise.resolve({ ok: true }));
    await renderReady();
    await type("써줘");
    await pressEnter();
    await emit("agent-delta", { run_id: "r1", text: "여기까지 쓰다가" });

    await act(async () => {
      screen.getByTestId("agent-stop").click();
    });
    await emit("agent-cancelled", { run_id: "r1" });

    const log = screen.getByTestId("agent-log");
    expect(log.querySelector(".msg.bot .msg-bubble")?.textContent).toBe("여기까지 쓰다가");
    expect(notices()[0].textContent).toBe("agentPanel.cancelled");
  });
});

describe("AgentPanel review carry-ins (#95 Task 7)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
  });

  it("does not surface a stale cancel failure once the turn already ended on its own", async () => {
    // Task 6's review carried this in: handleStop's async rejection handler
    // must compare against turnRunIdRef.current (read fresh, at settle time),
    // not the `turnRunId` state closed over when handleStop was called — that
    // closure equals the captured runId by construction, so a mistaken swap
    // makes the guard a no-op and every stale cancel failure would surface,
    // even a genuinely stale one racing a turn that already finished.
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    let releaseCancel: (() => void) | null = null;
    rpc.agentCancel.mockImplementation(
      () =>
        new Promise<void>((_resolve, reject) => {
          releaseCancel = () => reject(new Error("too late, the turn already finished"));
        }),
    );
    await renderReady();
    await type("써줘");
    await pressEnter();

    await act(async () => {
      screen.getByTestId("agent-stop").click();
    });
    // The turn finishes on its own, racing the still-in-flight cancel call.
    await emit("agent-done", { run_id: "r1", usage: { input: 1, output: 1 } });

    await act(async () => {
      releaseCancel?.();
    });

    expect(screen.queryByTestId("agent-send-error")).toBeNull();
  });

  it("keeps an event that arrived during a later-rejected send, instead of losing it forever", async () => {
    // Task 6's review carried this in too: flushSendWindow() (not
    // discardSendWindow(), and not omitted) must run in handleSend's catch
    // branch. Both wrong alternatives leave the held event unapplied here —
    // discarding throws it away, and omitting it leaves pendingSendRef stuck
    // true, so nothing after it is ever processed either.
    //
    // A tool event, not a delta: a replayed delta flips `running` back to
    // true and hands the bubble to useSmoothStream's animated reveal, which
    // needs driven rAF frames to show anything in a test — irrelevant noise
    // for what this test actually checks. A tool line renders straight from
    // state with no animation in between.
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "r1" }));
    await renderReady();
    await type("첫 번째 요청");
    await pressEnter();
    await emit("agent-done", { run_id: "r1", usage: { input: 1, output: 1 } });

    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);
    await type("두 번째 요청");
    await pressEnter();

    // A tool event for the still-current run r1 arrives while the send
    // window for the (about to be refused) second send is open.
    await emit("agent-tool", { run_id: "r1", name: "linetta_search_manuscript", state: "started" });

    await act(async () => {
      pending.reject({ data: { reason: "provider_consent_required" } });
    });

    // currentRunIdRef was never reassigned (the send failed before a run id
    // existed), so flushSendWindow replays the held event exactly as it
    // would have been judged without the window — onto r1, the still-current
    // run. Losing it (discardSendWindow, or no flush at all) is the bug this
    // test exists to catch.
    expect(screen.getByTestId("agent-log").querySelectorAll('[data-testid="tool-line"]')).toHaveLength(1);
  });

  it("keeps COMPOSER_MAX_HEIGHT in AgentPanel.tsx equal to .cmp-input textarea's max-height in App.css", async () => {
    const tsx = await readSource("components/agent/AgentPanel.tsx");
    const css = await readSource("App.css");

    const tsMatch = tsx.match(/const COMPOSER_MAX_HEIGHT = (\d+);/);
    const cssMatch = css.match(/\.cmp-input textarea \{[^}]*max-height:\s*(\d+)px/);
    if (!tsMatch) throw new Error("COMPOSER_MAX_HEIGHT constant not found in AgentPanel.tsx");
    if (!cssMatch) throw new Error(".cmp-input textarea's max-height not found in App.css");

    expect(cssMatch[1]).toBe(tsMatch[1]);
  });
});

const OTHER_PROJECT_ID = "project-2";

describe("AgentPanel project switch (#95 Task 7 review)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
  });

  function panelFor(projectId: string) {
    return (
      <MemoryRouter>
        <AgentPanel onClose={vi.fn()} projectId={projectId} nodeId={NODE_ID} />
      </MemoryRouter>
    );
  }

  /** Renders a ready panel that can be re-rendered at another project without
   *  unmounting — the actual shape of the bug. Neither the `/workspace/:id`
   *  route nor `<AgentPanel>` is keyed on the project id, so global search's
   *  cross-project jump changes this prop under a panel that stays mounted. */
  async function renderReadyAt(projectId: string) {
    rpc.providersList.mockImplementation(() =>
      Promise.resolve([row({ configured: true, consented: true })]),
    );
    const view = render(panelFor(projectId));
    await screen.findByTestId("agent-log");
    return view;
  }

  async function jumpTo(view: ReturnType<typeof render>, projectId: string) {
    await act(async () => {
      view.rerender(panelFor(projectId));
    });
  }

  function notices(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>('[data-testid="agent-notice"]'));
  }

  it("does not leave one work's conversation on screen after a jump to another", async () => {
    rpc.agentHistory.mockImplementation((projectId: string) =>
      Promise.resolve(
        projectId === PROJECT_ID
          ? [historyRow({ id: "a1", run_id: "run-a", role: "user", content: "A작품의 사적인 대화" })]
          : [historyRow({ id: "b1", run_id: "run-b", role: "user", content: "B작품의 요청" })],
      ),
    );
    const view = await renderReadyAt(PROJECT_ID);
    expect(await screen.findByText("A작품의 사적인 대화")).toBeTruthy();

    await jumpTo(view, OTHER_PROJECT_ID);

    expect(await screen.findByText("B작품의 요청")).toBeTruthy();
    // The whole point: A's words are not sitting in B's log, interleaved and
    // unlabelled, for the writer or anyone reading over their shoulder.
    expect(screen.queryByText("A작품의 사적인 대화")).toBeNull();
    expect(screen.getByTestId("agent-log").textContent).not.toContain("A작품");
    // Both histories end on an unanswered user row, so both restores add the
    // "may still be running" notice. Two of them under one fixed id is the
    // duplicate React key that let the two conversations interleave.
    expect(notices()).toHaveLength(1);
  });

  it("does not install a turn sent for the previous work into the new work's panel", async () => {
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);
    const view = await renderReadyAt(PROJECT_ID);
    await type("A작품에만 보낸 요청");
    await pressEnter();

    // The writer jumps to another work before agent.run's response lands. The
    // turn is real and still running in the engine — nothing here can stop it.
    await jumpTo(view, OTHER_PROJECT_ID);
    await act(async () => {
      pending.resolve({ run_id: "run-a" });
    });

    const log = screen.getByTestId("agent-log");
    expect(log.textContent).not.toContain("A작품에만 보낸 요청");
    // And the composer belongs to the new work: it must not be left showing
    // stop for a turn the writer can no longer see.
    expect(screen.getByTestId("agent-send")).toBeTruthy();
    expect(screen.queryByTestId("agent-stop")).toBeNull();
  });

  it("refuses events from a run the writer left behind, instead of adopting them", async () => {
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "run-a" }));
    const view = await renderReadyAt(PROJECT_ID);
    await type("A작품 요청");
    await pressEnter();

    await jumpTo(view, OTHER_PROJECT_ID);
    // A tool event, not a delta, so it renders straight from state with no
    // useSmoothStream animation in between — same reasoning as the send-window
    // carry-in test above.
    await emit("agent-tool", { run_id: "run-a", name: "linetta_write_scene", state: "started" });

    // currentRunIdRef was cleared by the switch, so without naming the
    // abandoned run acceptRun's first-wins null check would adopt this event
    // as the new work's own turn.
    expect(screen.queryAllByTestId("tool-line")).toHaveLength(0);
  });
});

describe("AgentPanel restored turn status (#95 Task 7 review)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
  });

  function notices(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>('[data-testid="agent-notice"]'));
  }

  it("says a restored turn failed, instead of restoring it as a finished reply", async () => {
    // The engine stamps every row of a provider-failed turn "failed"
    // (transcript.go's markRun, from loop.go's endWithError). Live, the
    // writer saw agent.error under this partial text; restored, `status` is
    // the only trace of it left.
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", run_id: "r1", role: "user", content: "이어서 써줘", status: "failed" }),
        historyRow({ id: "h2", run_id: "r1", role: "assistant", content: "여기까지 쓰다가", status: "failed" }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("여기까지 쓰다가")).toBeTruthy();
    const found = notices();
    expect(found).toHaveLength(1);
    expect(found[0].textContent).toBe("agentPanel.restore.failed");
  });

  it("leaves an iteration-limit turn unlabelled, and hedges it as maybe-running — which it always will", async () => {
    // THE SHAPE HERE IS THE ONE THE ENGINE ACTUALLY WRITES, and the version of
    // this test that shipped in review round 1 had it wrong: it ended the run
    // on an assistant row and asserted no notice at all, which made the wall
    // look covered. loop.go cannot produce that ordering.
    //
    //   - appendAssistant runs BEFORE the tool loop, so the partial reply's row
    //     is written first (loop.go:241).
    //   - endAtWall returns from inside that loop, right after a runTool
    //     (loop.go:285), or immediately after it (loop.go:293).
    //   - endWithReason, which endAtWall delegates to, appends no row at all —
    //     it stamps the existing ones and notifies.
    //   - the in-loop cap check (loop.go:263) can never be the one that fires:
    //     toolCalls is unchanged since the post-loop check of the previous
    //     iteration, which would already have returned.
    //
    // So a wall-ended run's LAST row is always a `tool` row stamped "done".
    //
    // Two things follow, and this test pins both:
    //
    //   1. endNoticeVariant leaves it unlabelled, which is right and
    //      deliberate: those tool calls really ran and that partial reply is
    //      real work. A failure notice under it would say something untrue
    //      about the writer's manuscript.
    //   2. turnMayBeRunning returns true, because the last row is not an
    //      assistant row — so the panel hedges "이 요청이 아직 진행 중일 수
    //      있습니다" under a turn the engine definitively stopped. Not
    //      sometimes: 100% of wall-ended turns, every time. Round 2's report
    //      recorded this as a possibility (N2); it is a certainty.
    //
    // The behaviour is NOT fixed here, and the notice is asserted rather than
    // wished away. The transcript stamps a wall "done" — byte-identical to an
    // abandoned turn that ended on a resolved tool call, which is the case the
    // hedge exists for — so the panel genuinely cannot tell them apart. Closing
    // it needs the engine to say which one it was: a run-status RPC, or a
    // distinct status for the wall. The plan adds neither.
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", run_id: "r1", role: "user", content: "많이 해줘", status: "done" }),
        historyRow({ id: "h2", run_id: "r1", role: "assistant", content: "24개까지 했습니다", status: "done" }),
        historyRow({
          id: "h3",
          run_id: "r1",
          role: "tool",
          content: JSON.stringify({ name: "linetta_write_scene", ok: true }),
          status: "done",
        }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("24개까지 했습니다")).toBeTruthy();
    const found = notices();
    // No failure notice: the wall's rows stay "done" on purpose.
    expect(found.map((n) => n.textContent)).not.toContain("agentPanel.restore.failed");
    // And the gap, stated: one hedge, about a turn that is over.
    expect(found).toHaveLength(1);
    expect(found[0].textContent).toBe("agentPanel.restore.mayBeRunning");
  });

  it("says a restored turn was cancelled", async () => {
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", run_id: "r1", role: "user", content: "써줘", status: "cancelled" }),
        historyRow({ id: "h2", run_id: "r1", role: "assistant", content: "쓰다 말았습니다", status: "cancelled" }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("쓰다 말았습니다")).toBeTruthy();
    expect(notices()[0].textContent).toBe("agentPanel.cancelled");
  });

  it("marks only the run that failed when an earlier turn succeeded before it", async () => {
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", run_id: "r1", role: "user", content: "첫 요청", status: "done" }),
        historyRow({ id: "h2", run_id: "r1", role: "assistant", content: "첫 답변", status: "done" }),
        historyRow({ id: "h3", run_id: "r2", role: "user", content: "두 번째 요청", status: "failed" }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("두 번째 요청")).toBeTruthy();
    const found = notices();
    // One notice, for r2 only — and no "may still be running" hedge, since
    // the failed status already says how that turn ended.
    expect(found).toHaveLength(1);
    expect(found[0].textContent).toBe("agentPanel.restore.failed");
  });

  it("shows the still-running notice when the conversation ends on a resolved tool call", async () => {
    // The abandoned turn the "last row is a user row" test missed: the engine
    // died after a tool call resolved but before the model produced final
    // text, so no markRun ever ran and the last row is a tool row.
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", run_id: "r1", role: "user", content: "고쳐줘", status: "done" }),
        historyRow({
          id: "h2",
          run_id: "r1",
          role: "tool",
          content: JSON.stringify({ name: "linetta_write_scene", ok: true }),
          status: "done",
        }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("고쳐줘")).toBeTruthy();
    const found = notices();
    expect(found).toHaveLength(1);
    expect(found[0].textContent).toBe("agentPanel.restore.mayBeRunning");
  });

  it("does not hedge that a cancelled turn may still be running", async () => {
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", run_id: "r1", role: "user", content: "그만", status: "cancelled" }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("그만")).toBeTruthy();
    const found = notices();
    expect(found).toHaveLength(1);
    expect(found[0].textContent).toBe("agentPanel.cancelled");
  });

  it("skips a tool row whose content is the JSON literal null", async () => {
    // JSON.parse("null") succeeds and yields null — the one malformed shape
    // that gets past the try/catch, and the one the brief named.
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", run_id: "r1", role: "user", content: "확인용" }),
        historyRow({ id: "h2", run_id: "r1", role: "tool", content: "null" }),
        historyRow({ id: "h3", run_id: "r1", role: "assistant", content: "그래도 답은 옵니다" }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("확인용")).toBeTruthy();
    expect(screen.getByTestId("agent-log").querySelector(".msg.bot .msg-bubble")?.textContent).toBe(
      "그래도 답은 옵니다",
    );
    expect(screen.queryAllByTestId("tool-line")).toHaveLength(0);
  });
});

describe("AgentPanel abandoned send fence (#95 Task 7 review round 2)", () => {
  // Stand-in animation frames. jsdom's own requestAnimationFrame never runs
  // inside `act`, so a delta that IS wrongly adopted renders an empty bubble
  // and hides the prose it leaked — which is exactly why round 1's tests used
  // tool events and missed this. Driving the frames by hand makes
  // useSmoothStream finish its reveal synchronously, so the assertion can
  // read the sentence that should never have been on screen.
  const frames: FrameRequestCallback[] = [];

  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
    frames.length = 0;
    vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => frames.push(cb));
    vi.stubGlobal("cancelAnimationFrame", () => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  /** Runs queued frames until the reveal stops asking for more (each tick
   *  re-queues, so the count is the bound, not the queue). */
  async function flushFrames(limit = 40) {
    for (let i = 0; i < limit; i++) {
      const queued = frames.splice(0, frames.length);
      if (queued.length === 0) return;
      await act(async () => {
        for (const cb of queued) cb(i);
      });
    }
  }

  function panelFor(projectId: string) {
    return (
      <MemoryRouter>
        <AgentPanel onClose={vi.fn()} projectId={projectId} nodeId={NODE_ID} />
      </MemoryRouter>
    );
  }

  async function renderReadyAt(projectId: string) {
    rpc.providersList.mockImplementation(() =>
      Promise.resolve([row({ configured: true, consented: true })]),
    );
    const view = render(panelFor(projectId));
    await screen.findByTestId("agent-log");
    return view;
  }

  async function jumpTo(view: ReturnType<typeof render>, projectId: string) {
    await act(async () => {
      view.rerender(panelFor(projectId));
    });
  }

  it("refuses a delta for a send it left behind, instead of streaming that work's prose into another's log", async () => {
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);
    const view = await renderReadyAt(PROJECT_ID);
    await type("A작품 요청");
    await pressEnter();

    // The writer jumps away while agent.run is still in flight. The run id is
    // unknown here — that is the whole gap — and the switch has already closed
    // the window that would otherwise have held this event until it was known.
    await jumpTo(view, OTHER_PROJECT_ID);
    await emit("agent-delta", { run_id: "run-a", text: "A작품에만 있던 은밀한 문장" });
    await flushFrames();

    const log = screen.getByTestId("agent-log");
    expect(log.querySelectorAll(".msg.bot")).toHaveLength(0);
    expect(log.textContent).not.toContain("A작품에만 있던 은밀한 문장");

    // And still refused once the send finally lands and names the run.
    await act(async () => {
      pending.resolve({ run_id: "run-a" });
    });
    await emit("agent-delta", { run_id: "run-a", text: "이어지는 문장" });
    await flushFrames();
    expect(screen.getByTestId("agent-log").textContent).not.toContain("이어지는 문장");
  });

  it("holds a second send until the abandoned one settles, instead of racing it", async () => {
    const first = deferred<{ run_id: string }>();
    const second = deferred<{ run_id: string }>();
    rpc.agentRun
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);
    const view = await renderReadyAt(PROJECT_ID);
    await type("A작품 요청");
    await pressEnter();
    expect(rpc.agentRun).toHaveBeenCalledTimes(1);

    await jumpTo(view, OTHER_PROJECT_ID);
    await type("B작품 요청");
    await pressEnter();

    // Sending now would leave two round trips outstanding whose responses can
    // land in either order, each overwriting the other's run id.
    expect(rpc.agentRun).toHaveBeenCalledTimes(1);
    expect(composer().value).toBe("B작품 요청");
    expect((screen.getByTestId("agent-send") as HTMLButtonElement).disabled).toBe(true);

    await act(async () => {
      first.resolve({ run_id: "run-a" });
    });

    // The fence lifts the moment the abandoned send settles.
    expect((screen.getByTestId("agent-send") as HTMLButtonElement).disabled).toBe(false);
    await pressEnter();
    expect(rpc.agentRun).toHaveBeenCalledTimes(2);
    await act(async () => {
      second.resolve({ run_id: "run-b" });
    });
    expect(screen.getByTestId("agent-log").textContent).toContain("B작품 요청");
  });

  it("does not install a send from a work the writer left and came back to", async () => {
    const first = deferred<{ run_id: string }>();
    const second = deferred<{ run_id: string }>();
    rpc.agentRun
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);
    const view = await renderReadyAt(PROJECT_ID);
    await type("첫 번째 요청");
    await pressEnter();

    // Away and back: the project id matches again by the time the stale
    // response lands, so comparing project ids alone cannot tell the two apart.
    await jumpTo(view, OTHER_PROJECT_ID);
    await jumpTo(view, PROJECT_ID);
    await type("두 번째 요청");
    await pressEnter();

    await act(async () => {
      first.resolve({ run_id: "run-stale" });
    });
    const log = () => screen.getByTestId("agent-log");
    expect(log().textContent).not.toContain("첫 번째 요청");
    expect(screen.queryByTestId("agent-stop")).toBeNull();

    // The second send is the one this panel is actually waiting on. (Fenced,
    // it was held above and this press is what sends it; unfenced it went out
    // already and the composer is empty, making this a no-op — either way
    // exactly one turn is outstanding from here.)
    await pressEnter();
    await act(async () => {
      second.resolve({ run_id: "run-new" });
    });

    expect(log().textContent).toContain("두 번째 요청");
    expect(log().textContent).not.toContain("첫 번째 요청");
    // The new run's events render, and stop targets the new run.
    await emit("agent-tool", { run_id: "run-new", name: "linetta_write_scene", state: "started" });
    expect(screen.queryAllByTestId("tool-line")).toHaveLength(1);
    await act(async () => {
      fireEvent.click(screen.getByTestId("agent-stop"));
    });
    expect(rpc.agentCancel).toHaveBeenCalledWith("run-new");
  });
});

describe("AgentPanel restore notice boundaries (#95 Task 7 review round 2)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
  });

  function notices(): HTMLElement[] {
    return Array.from(screen.getByTestId("agent-log").querySelectorAll<HTMLElement>('[data-testid="agent-notice"]'));
  }

  it("still says how a run ended when its last row is a tool row the panel cannot parse", async () => {
    // Skipping the unparseable LINE must not also skip the notice saying the
    // run failed — the row is unreadable, its status is not.
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", run_id: "r1", role: "user", content: "실패한 요청", status: "failed" }),
        historyRow({ id: "h2", run_id: "r1", role: "tool", content: "{잘린 JSON", status: "failed" }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("실패한 요청")).toBeTruthy();
    expect(screen.queryAllByTestId("tool-line")).toHaveLength(0);
    expect(notices()).toHaveLength(1);
    expect(notices()[0].textContent).toBe("agentPanel.restore.failed");
  });

  it("gives each run's end notice its own key, so two of them cannot collide", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    rpc.agentHistory.mockImplementation(() =>
      Promise.resolve([
        historyRow({ id: "h1", run_id: "r1", role: "user", content: "첫 요청", status: "failed" }),
        historyRow({ id: "h2", run_id: "r2", role: "user", content: "둘째 요청", status: "cancelled" }),
      ]),
    );
    await renderReady();

    expect(await screen.findByText("첫 요청")).toBeTruthy();
    expect(notices().map((n) => n.textContent)).toEqual([
      "agentPanel.restore.failed",
      "agentPanel.cancelled",
    ]);
    // A fixed id here is what turned the missing project reset into two
    // interleaved conversations rather than a visibly stale one.
    const logged = consoleError.mock.calls.map((args) => args.join(" ")).join("\n");
    expect(logged).not.toContain("same key");
    consoleError.mockRestore();
  });
});

describe("AgentPanel project ownership on the wire (#95 Task 7 review round 3)", () => {
  // Same hand-driven frames as the round-2 block: jsdom's requestAnimationFrame
  // never runs inside `act`, so a delta that IS wrongly adopted renders an
  // empty bubble and hides the prose it leaked.
  const frames: FrameRequestCallback[] = [];

  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
    rpc.providersList.mockImplementation(() =>
      Promise.resolve([row({ configured: true, consented: true })]),
    );
    frames.length = 0;
    vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => frames.push(cb));
    vi.stubGlobal("cancelAnimationFrame", () => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  async function flushFrames(limit = 40) {
    for (let i = 0; i < limit; i++) {
      const queued = frames.splice(0, frames.length);
      if (queued.length === 0) return;
      await act(async () => {
        for (const cb of queued) cb(i);
      });
    }
  }

  function panelFor(projectId: string, strict = false) {
    const panel = (
      <MemoryRouter>
        <AgentPanel onClose={vi.fn()} projectId={projectId} nodeId={NODE_ID} />
      </MemoryRouter>
    );
    // StrictMode is on in main.tsx, and one of the tests below only fails
    // under it — see "keeps the send button's disabled mirror".
    return strict ? <StrictMode>{panel}</StrictMode> : panel;
  }

  async function renderReadyAt(projectId: string, strict = false) {
    const view = render(panelFor(projectId, strict));
    await screen.findByTestId("agent-log");
    return view;
  }

  async function jumpTo(view: ReturnType<typeof render>, projectId: string, strict = false) {
    await act(async () => {
      view.rerender(panelFor(projectId, strict));
    });
  }

  function log() {
    return screen.getByTestId("agent-log");
  }

  it("refuses another work's delta on a panel that was only just mounted", async () => {
    // The mount boundary. A turn is running for A; the writer closes the agent
    // panel (Workspace unmounts it), jumps to B, and opens it again. The fresh
    // mount has an empty abandonedRunsRef, a null currentRunIdRef and no
    // fence, so every client-side guard this panel has grown over three review
    // rounds starts out knowing nothing — and A's next delta is the first
    // event it sees.
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "run-a" }));
    const first = await renderReadyAt(PROJECT_ID);
    await type("A작품 요청");
    await pressEnter();
    await act(async () => {
      first.unmount();
    });

    await renderReadyAt(OTHER_PROJECT_ID);
    await emit("agent-delta", {
      run_id: "run-a",
      project_id: PROJECT_ID,
      text: "A작품에만 있던 은밀한 문장",
    });
    await flushFrames();

    expect(log().querySelectorAll(".msg.bot")).toHaveLength(0);
    expect(log().textContent).not.toContain("A작품에만 있던 은밀한 문장");
  });

  it("refuses another work's tool and terminal events on a freshly mounted panel", async () => {
    // The same boundary for the events that are not prose: a tool line in the
    // wrong log names work the writer did not ask for, and a terminal event
    // hands the composer a turn that is not theirs.
    await renderReadyAt(OTHER_PROJECT_ID);
    await emit("agent-tool", {
      run_id: "run-a",
      project_id: PROJECT_ID,
      name: "linetta_write_scene",
      state: "started",
    });
    await emit("agent-error", {
      run_id: "run-a",
      project_id: PROJECT_ID,
      reason: "agent_internal_error",
      message: "",
    });
    await emit("agent-cancelled", { run_id: "run-a", project_id: PROJECT_ID });

    expect(screen.queryAllByTestId("tool-line")).toHaveLength(0);
    expect(screen.queryAllByTestId("agent-notice")).toHaveLength(0);
  });

  it("refuses another work's delta after its send was refused, not just after it was named", async () => {
    // The catch branch lowers the fence with nothing named: on a rejection no
    // run id was ever minted, so clearOrphanedSend() runs and abandonedRunsRef
    // gains nothing. From that instant the null-adopt path is open again, and
    // an event for the abandoned work's run has no client-side guard left to
    // meet.
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);
    const view = await renderReadyAt(PROJECT_ID);
    await type("A작품 요청");
    await pressEnter();

    await jumpTo(view, OTHER_PROJECT_ID);
    await act(async () => {
      pending.reject(new Error("provider went away"));
    });

    await emit("agent-delta", {
      run_id: "run-a",
      project_id: PROJECT_ID,
      text: "A작품에만 있던 다른 문장",
    });
    await flushFrames();

    expect(log().querySelectorAll(".msg.bot")).toHaveLength(0);
    expect(log().textContent).not.toContain("A작품에만 있던 다른 문장");
    // And the refusal itself is still not reported in the work the writer is
    // now looking at (round 1's rule, unchanged).
    expect(log().textContent).not.toContain("A작품 요청");
  });

  it("keeps the send button's disabled mirror in step with the fence under StrictMode", async () => {
    // React double-invokes the render body and discards the first pass's
    // queued state updates — but not its ref mutations, and the reset's own
    // discardSendWindow() clears pendingSendRef in that same body. A raise
    // conditioned on pendingSendRef alone therefore lands only in the pass
    // that is thrown away: the ref fence still refuses (nothing leaks) but the
    // button says the writer may send, and pressing it does nothing.
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);
    const view = await renderReadyAt(PROJECT_ID, true);
    await type("A작품 요청");
    await pressEnter();
    expect(rpc.agentRun).toHaveBeenCalledTimes(1);

    await jumpTo(view, OTHER_PROJECT_ID, true);
    await type("B작품 요청");

    expect((screen.getByTestId("agent-send") as HTMLButtonElement).disabled).toBe(true);
    // The affordance and the behaviour have to agree: a live-looking button
    // that swallows the press is the failure this mirror exists to prevent.
    await pressEnter();
    expect(rpc.agentRun).toHaveBeenCalledTimes(1);
    expect(composer().value).toBe("B작품 요청");

    // And it comes back the moment the abandoned send settles.
    await act(async () => {
      pending.resolve({ run_id: "run-a" });
    });
    expect((screen.getByTestId("agent-send") as HTMLButtonElement).disabled).toBe(false);
  });

  it("still refuses a named abandoned run once the writer jumps back to its work", async () => {
    // A → B → A. The project check cannot help here: the event names the work
    // now on screen, and it is telling the truth. abandonedRunsRef is the only
    // thing that knows this run's bubble is gone — restored from history
    // instead — so this is what keeps it pinned now that the wire says whose
    // an event is.
    rpc.agentRun.mockImplementation(() => Promise.resolve({ run_id: "run-a" }));
    const view = await renderReadyAt(PROJECT_ID);
    await type("A작품 요청");
    await pressEnter();

    await jumpTo(view, OTHER_PROJECT_ID);
    await jumpTo(view, PROJECT_ID);
    await emit("agent-tool", {
      run_id: "run-a",
      project_id: PROJECT_ID,
      name: "linetta_write_scene",
      state: "started",
    });

    expect(screen.queryAllByTestId("tool-line")).toHaveLength(0);
  });

  it("still fences an unnamed abandoned send once the writer jumps back to its work", async () => {
    // The same A → B → A shape while the run id is still in the post. The
    // event names the work on screen, currentRunIdRef is null, and nothing is
    // in abandonedRunsRef yet — orphanedSendRef is the only refusal left, and
    // it has to survive the second switch to give it.
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);
    const view = await renderReadyAt(PROJECT_ID);
    await type("A작품 요청");
    await pressEnter();

    await jumpTo(view, OTHER_PROJECT_ID);
    await jumpTo(view, PROJECT_ID);
    await emit("agent-delta", {
      run_id: "run-a",
      project_id: PROJECT_ID,
      text: "이름이 아직 붙지 않은 문장",
    });
    await flushFrames();

    expect(log().querySelectorAll(".msg.bot")).toHaveLength(0);
    expect(log().textContent).not.toContain("이름이 아직 붙지 않은 문장");
  });
});

describe("AgentPanel ownership ordering (#95 Task 7 final review)", () => {
  // Two orderings the 94 tests above did not pin. Both mutations leave the
  // whole suite green, and both reopen a leak the branch spent six rounds
  // closing — which is the point: the ordering IS the fix, and an invariant
  // that only holds by accident holds until the next refactor.
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.agentHistory.mockImplementation(() => Promise.resolve([]));
    rpc.providersList.mockImplementation(() =>
      Promise.resolve([row({ configured: true, consented: true })]),
    );
  });

  function panelFor(projectId: string) {
    return (
      <MemoryRouter>
        <AgentPanel onClose={vi.fn()} projectId={projectId} nodeId={NODE_ID} />
      </MemoryRouter>
    );
  }

  async function renderReadyAt(projectId: string) {
    const view = render(panelFor(projectId));
    await screen.findByTestId("agent-log");
    return view;
  }

  async function jumpTo(view: ReturnType<typeof render>, projectId: string) {
    await act(async () => {
      view.rerender(panelFor(projectId));
    });
  }

  it("refuses another work's event during a send instead of holding it for the flush", async () => {
    // The project check must run AHEAD of the send window, not behind it.
    // Swapping those two lines in whileNotSending leaves every other test in
    // this file green, because the window is normally empty — and it reopens
    // the cross-work leak through a door no ref can reach:
    //
    // The panel is freshly mounted in A (the writer closed it during a turn in
    // B, so nothing here remembers B). The writer sends in A, which opens the
    // send window. B's turn — still running, still emitting — lands a tool
    // event. Buffered, if the window is consulted first, and buffered as
    // `() => apply(payload)`, which calls the handler directly and never
    // re-asks whose event it is. A's send is then refused, flushSendWindow()
    // replays it, and acceptRun adopts B's run because currentRunIdRef is null.
    // B's tool line, in A's conversation.
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);
    await renderReadyAt(PROJECT_ID);
    await type("A작품 요청");
    await pressEnter();

    await emit("agent-tool", {
      run_id: "run-b",
      project_id: OTHER_PROJECT_ID,
      name: "linetta_undo_last_change",
      state: "started",
    });
    await act(async () => {
      pending.reject({ data: { reason: "provider_not_configured" } });
    });

    expect(screen.queryAllByTestId("tool-line")).toHaveLength(0);
    expect(screen.getByTestId("agent-log").textContent).not.toContain(
      "agentPanel.toolName.linetta_undo_last_change",
    );
  });

  it("lowers the fence when the send it left behind is refused, not only when it succeeds", async () => {
    // clearOrphanedSend() in handleSend's CATCH, specifically. Removing it
    // from only that site leaves all 94 tests green — the resolve site catches
    // the other half — and the send button is then disabled for the life of
    // the mount, with no way back: a cross-project jump does not unmount the
    // panel, so only closing and reopening the whole panel recovers.
    const pending = deferred<{ run_id: string }>();
    rpc.agentRun.mockImplementation(() => pending.promise);
    const view = await renderReadyAt(PROJECT_ID);
    await type("A작품 요청");
    await pressEnter();

    // The jump raises the fence and clears the token, so this send can no
    // longer be this panel's — which is exactly the state the catch has to
    // clean up after. Typing first, so `disabled` is answering the fence and
    // not merely an empty composer.
    await jumpTo(view, OTHER_PROJECT_ID);
    await type("B작품 요청");
    expect((screen.getByTestId("agent-send") as HTMLButtonElement).disabled).toBe(true);

    await act(async () => {
      pending.reject({ data: { reason: "provider_unreachable" } });
    });

    expect((screen.getByTestId("agent-send") as HTMLButtonElement).disabled).toBe(false);
    // And the affordance has to be telling the truth: the press goes out.
    await pressEnter();
    expect(rpc.agentRun).toHaveBeenCalledTimes(2);
    expect(rpc.agentRun).toHaveBeenLastCalledWith(OTHER_PROJECT_ID, "B작품 요청", NODE_ID);
    // The refused send for the work the writer left is still not news here.
    expect(screen.getByTestId("agent-log").textContent).not.toContain("A작품 요청");
  });
});
