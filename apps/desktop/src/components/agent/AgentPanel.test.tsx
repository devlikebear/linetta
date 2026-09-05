import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readSource } from "../../test/readSource";
import type { ProviderStatus } from "../../lib/types";

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
  agentUndo: vi.fn(),
  agentRun: vi.fn(),
  agentCancel: vi.fn(),
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
  agent: { undo: rpc.agentUndo, run: rpc.agentRun, cancel: rpc.agentCancel },
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
