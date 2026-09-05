import { act, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ProviderStatus } from "../../lib/types";

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
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

  it("keeps agent.tool events out of the rendered log (the collapsed line is Task 5)", async () => {
    await renderReady();

    await emit("agent-tool", { run_id: "r1", name: "linetta_search_scenes", state: "started" });
    await emit("agent-tool", {
      run_id: "r1",
      name: "linetta_search_scenes",
      state: "done",
      summary: "3 scenes",
    });
    await emit("agent-delta", { run_id: "r1", text: "Found them." });
    await emit("agent-done", { run_id: "r1", usage: { input: 1, output: 1 } });

    const log = screen.getByTestId("agent-log");
    expect(log.querySelectorAll(".msg")).toHaveLength(1);
    expect(log.textContent).not.toContain("3 scenes");
  });
});
