import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentCitation, AgentHistoryRow, PassageTarget } from "../../lib/types";

// Question mode (#385 Phase 3): the panel's writing/asking switch, the source
// cards under an answer and the [n] chips in it, live and restored.

const rpc = vi.hoisted(() => ({
  providersList: vi.fn(),
  agentRun: vi.fn(),
  agentHistory: vi.fn(),
  agentPrompts: vi.fn(),
  settingsGet: vi.fn(),
  proposalsList: vi.fn(),
  trialStatus: vi.fn(),
}));

const ev = vi.hoisted(() => ({ listeners: new Map<string, (e: { payload: unknown }) => void>() }));

vi.mock("@tauri-apps/api/event", () => ({
  listen: (event: string, cb: (e: { payload: unknown }) => void) => {
    ev.listeners.set(event, cb);
    return Promise.resolve(() => ev.listeners.delete(event));
  },
}));

vi.mock("../../lib/rpc", () => ({
  providers: { list: rpc.providersList },
  agent: { run: rpc.agentRun, history: rpc.agentHistory, prompts: rpc.agentPrompts, undo: vi.fn(), cancel: vi.fn(), startFresh: vi.fn() },
  settings: { get: rpc.settingsGet },
  proposals: { list: rpc.proposalsList, accept: vi.fn() },
  trial: { status: rpc.trialStatus },
}));

vi.mock("../illustration/IllustrationRequestCard", () => ({ IllustrationRequestCard: () => null }));

vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(",")}` : key,
  }),
  // Stored labels pass through, marked, so a test can see they went through it.
  displayNodeLabel: (_language: string, label: string) => `<${label}>`,
}));

import { AgentPanel } from "./AgentPanel";

const PROJECT_ID = "project-1";
const NODE_ID = "node-1";

const PROMPTS = [
  { name: "draft_current_scene", title: "초고", text: "draft text", slots: ["start"] },
  { name: "first_appearances", title: "처음 등장", text: "first appearance text", slots: ["ask"] },
];

const SOURCE: AgentCitation = {
  n: 1,
  node_id: "scene-12",
  breadcrumb: "12화 / 씬 3",
  scene_title: "비 오는 음악실",
  paragraph_from: 4,
  paragraph_to: 5,
  quote: "검을 건네받은 손이 떨렸다",
};
const SAME_SCENE: AgentCitation = { ...SOURCE, n: 2, paragraph_from: 9, paragraph_to: 9, quote: "칼날이 빛났다" };

function historyRow(overrides: Partial<AgentHistoryRow>): AgentHistoryRow {
  return { id: "h", project_id: PROJECT_ID, run_id: "run-1", role: "user", status: "done", content: "", created_at: 1, ...overrides };
}

function panel(onRevealPassage?: (t: PassageTarget) => void, selectionRequest?: { id: number; text: string; nodeId: string }) {
  return (
    <MemoryRouter>
      <AgentPanel
        onClose={vi.fn()}
        projectId={PROJECT_ID}
        nodeId={NODE_ID}
        onRevealPassage={onRevealPassage}
        selectionRequest={selectionRequest}
      />
    </MemoryRouter>
  );
}

async function renderReady(onRevealPassage?: (t: PassageTarget) => void) {
  const view = render(panel(onRevealPassage));
  await screen.findByTestId("agent-log");
  return view;
}

async function emit(event: string, payload: Record<string, unknown>) {
  const cb = ev.listeners.get(event);
  if (!cb) throw new Error(`${event} listener was never registered`);
  await act(async () => {
    cb({ payload: { project_id: PROJECT_ID, ...payload } });
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  ev.listeners.clear();
  window.localStorage.clear();
  rpc.providersList.mockResolvedValue([{ id: "anthropic", auth: "api_key", active: true, configured: true, consented: true }]);
  rpc.agentHistory.mockResolvedValue([]);
  rpc.agentPrompts.mockResolvedValue({ prompts: PROMPTS });
  rpc.settingsGet.mockResolvedValue({ review_tools_enabled: true });
  rpc.proposalsList.mockResolvedValue({ proposals: [] });
  rpc.agentRun.mockResolvedValue({ run_id: "run-1" });
});

describe("AgentPanel question mode", () => {
  it("switches to asking: its own placeholder and chips, and the turn goes out as a question", async () => {
    await renderReady();
    expect(screen.getByTestId("agent-mode-write").getAttribute("aria-pressed")).toBe("true");
    await screen.findByTestId("agent-starter-draft_current_scene");

    fireEvent.click(screen.getByTestId("agent-mode-ask"));
    const composer = screen.getByTestId("agent-composer") as HTMLTextAreaElement;
    expect(composer.placeholder).toBe("agentPanel.composer.placeholderAsk");
    expect(screen.queryByTestId("agent-starter-draft_current_scene")).toBeNull();
    fireEvent.click(screen.getByTestId("agent-ask-first_appearances"));
    expect(composer.value).toBe("first appearance text");

    await act(async () => {
      fireEvent.click(screen.getByTestId("agent-send"));
    });
    expect(rpc.agentRun).toHaveBeenCalledWith(PROJECT_ID, "first appearance text", NODE_ID, undefined, undefined, "ask");
  });

  it("remembers the mode per work", async () => {
    const view = await renderReady();
    fireEvent.click(screen.getByTestId("agent-mode-ask"));
    view.unmount();
    await renderReady();
    expect(screen.getByTestId("agent-mode-ask").getAttribute("aria-pressed")).toBe("true");
  });

  it("opens on writing when a passage is sent from the editor", async () => {
    window.localStorage.setItem("linetta.agentIntent." + PROJECT_ID, "ask");
    const view = await renderReady();
    expect(screen.getByTestId("agent-mode-ask").getAttribute("aria-pressed")).toBe("true");
    await act(async () => {
      view.rerender(panel(undefined, { id: 1, text: "소매 끝", nodeId: NODE_ID }));
    });
    expect(screen.getByTestId("agent-mode-write").getAttribute("aria-pressed")).toBe("true");
  });

  it("draws a restored answer's sources as cards, one per scene, and its [n] as chips that open them", async () => {
    rpc.agentHistory.mockResolvedValue([
      historyRow({ id: "h1", role: "user", content: "검을 처음 받은 게 몇 화야?" }),
      historyRow({ id: "h2", role: "tool", content: JSON.stringify({ name: "linetta_cite_sources", ok: true, citations: [SOURCE, SAME_SCENE] }) }),
      historyRow({ id: "h3", role: "assistant", content: "12화에서 받았습니다 [1]. 칼날도 빛났죠 [2]. 다른 회차 [3]은 없습니다." }),
    ]);
    const reveal = vi.fn();
    await renderReady(reveal);

    const cards = await screen.findAllByTestId("agent-source");
    expect(cards).toHaveLength(1);
    expect(cards[0].textContent).toContain("<12화> / <씬 3> · 비 오는 음악실");
    expect(screen.getByTestId("agent-source-1").textContent).toContain("continuity.paragraphRange:from=4,to=5");
    expect(screen.getByTestId("agent-source-2").textContent).toContain("continuity.paragraphSingle:n=9");
    // The check is the cards; it is not drawn as a tool line as well.
    expect(screen.queryByTestId("tool-line")).toBeNull();

    expect(screen.getByTestId("agent-cite-1")).toBeTruthy();
    expect(screen.getByTestId("agent-cite-2")).toBeTruthy();
    // A number the check never returned stays text.
    expect(screen.queryByTestId("agent-cite-3")).toBeNull();
    expect(screen.getByTestId("agent-log").textContent).toContain("[3]");

    fireEvent.click(screen.getByTestId("agent-cite-1"));
    expect(reveal).toHaveBeenLastCalledWith({ nodeId: "scene-12", paragraphFrom: 4, paragraphTo: 5, quote: SOURCE.quote });
    fireEvent.click(screen.getByTestId("agent-source-2"));
    expect(reveal).toHaveBeenLastCalledWith({ nodeId: "scene-12", paragraphFrom: 9, paragraphTo: 9, quote: SAME_SCENE.quote });
  });

  it("leaves brackets alone in a reply whose turn checked no sources", async () => {
    rpc.agentHistory.mockResolvedValue([
      historyRow({ id: "h1", role: "user", content: "목록 줘" }),
      historyRow({ id: "h2", role: "assistant", content: "후보는 [1] 하린, [2] 도현입니다." }),
      historyRow({ id: "h3", role: "user", run_id: "run-2", content: "몇 화야?" }),
      historyRow({ id: "h4", role: "tool", run_id: "run-2", content: JSON.stringify({ name: "linetta_cite_sources", ok: true, citations: [SOURCE] }) }),
      historyRow({ id: "h5", role: "assistant", run_id: "run-2", content: "12화입니다 [1]." }),
    ]);
    await renderReady(vi.fn());
    await screen.findByTestId("agent-sources");
    // One chip: the second turn's. The first turn's [1] is a list, not a source.
    expect(screen.getAllByTestId("agent-cite-1")).toHaveLength(1);
    expect(screen.getByTestId("agent-log").textContent).toContain("[1] 하린");
  });

  it("shows the sources a live check returns", async () => {
    await renderReady(vi.fn());
    fireEvent.click(screen.getByTestId("agent-mode-ask"));
    fireEvent.change(screen.getByTestId("agent-composer"), { target: { value: "몇 화야?" } });
    await act(async () => {
      fireEvent.click(screen.getByTestId("agent-send"));
    });
    await emit("agent-tool", { run_id: "run-1", seq: 1, name: "linetta_cite_sources", state: "started" });
    await emit("agent-tool", { run_id: "run-1", seq: 2, name: "linetta_cite_sources", state: "done", citations: [SOURCE] });
    await waitFor(() => expect(screen.getByTestId("agent-source-1")).toBeTruthy());
  });
});
