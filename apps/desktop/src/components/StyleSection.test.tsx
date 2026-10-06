import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { StyleSection } from "./StyleSection";

const mocks = vi.hoisted(() => ({
  listeners: new Map<string, (e: { payload: unknown }) => void>(),
  getProject: vi.fn(),
  updateProject: vi.fn(),
  getRules: vi.fn(),
  setRules: vi.fn(),
  check: vi.fn(),
  getDraft: vi.fn(),
  approveDraft: vi.fn(),
  discardDraft: vi.fn(),
}));
vi.mock("@tauri-apps/api/event", () => ({ listen: (name: string, cb: (e: { payload: unknown }) => void) => {
  mocks.listeners.set(name, cb); return Promise.resolve(() => mocks.listeners.delete(name));
} }));
vi.mock("../lib/rpc", () => ({
  settings: { get: vi.fn().mockResolvedValue({ language: "en" }) },
  projects: { get: mocks.getProject, update: mocks.updateProject },
  style: {
    getRules: mocks.getRules, setRules: mocks.setRules, check: mocks.check,
    getDraft: mocks.getDraft, approveDraft: mocks.approveDraft, discardDraft: mocks.discardDraft,
  },
}));

const rules = { project_id: "p", avoid_phrases: ["it was as if", "very"], max_sentence_chars: 120, updated_at: 1 };
const noDraft = { pending: false, draft: { project_id: "p", body: "", author: "", created_at: 0 }, limit: 2200 };
const pendingDraft = {
  pending: true, limit: 2200,
  draft: { project_id: "p", body: "Short sentences. Past tense.", author: "agent", created_at: 5 },
};

function renderSection(props: Partial<Parameters<typeof StyleSection>[0]> = {}) {
  return render(<I18nProvider><StyleSection projectId="p" nodeId="n1" {...props} /></I18nProvider>);
}

describe("StyleSection", () => {
  beforeEach(() => {
    mocks.listeners.clear();
    mocks.getProject.mockReset().mockResolvedValue({ id: "p", style_notes: "Dry and plain." });
    mocks.updateProject.mockReset();
    mocks.getRules.mockReset().mockResolvedValue(rules);
    mocks.setRules.mockReset();
    mocks.check.mockReset();
    mocks.getDraft.mockReset().mockResolvedValue(noDraft);
    mocks.approveDraft.mockReset();
    mocks.discardDraft.mockReset();
  });

  it("shows the saved rules, one phrase per line", async () => {
    renderSection();
    const box = await screen.findByLabelText(/Phrases to avoid/);
    expect(box).toHaveValue("it was as if\nvery");
    expect(screen.getByLabelText(/Sentence length limit/)).toHaveValue(120);
  });

  it("saves what the writer typed as a phrase list and a number", async () => {
    mocks.setRules.mockResolvedValue({ ...rules, avoid_phrases: ["suddenly"], max_sentence_chars: 0 });
    renderSection();
    const box = await screen.findByLabelText(/Phrases to avoid/);
    fireEvent.change(box, { target: { value: "  suddenly  \n\n" } });
    fireEvent.change(screen.getByLabelText(/Sentence length limit/), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Save rules" }));

    await waitFor(() => expect(mocks.setRules).toHaveBeenCalledWith({
      project_id: "p", avoid_phrases: ["suddenly"], max_sentence_chars: 0,
    }));
    await waitFor(() => expect(box).toHaveValue("suddenly"));
  });

  it("checks the open scene after flushing the editor, and opens a scene from a result", async () => {
    const order: string[] = [];
    const onBeforeCheck = vi.fn(async () => { order.push("flush"); });
    const onOpenNode = vi.fn();
    mocks.check.mockImplementation(async () => {
      order.push("check");
      return {
        rules, scenes_checked: 1, total: 2, truncated: false,
        violations: [
          { node_id: "n1", label: "Scene 1", rule: "avoid_phrase", paragraph: 2, phrase: "very", excerpt: "…it was very quiet…" },
          { node_id: "n1", label: "Scene 1", rule: "sentence_too_long", paragraph: 3, excerpt: "A long one…", length: 140, limit: 120 },
        ],
      };
    });
    renderSection({ onBeforeCheck, onOpenNode });
    await screen.findByLabelText(/Phrases to avoid/);
    fireEvent.click(screen.getByRole("button", { name: "Check this scene" }));

    expect(await screen.findByText("2 place(s) break a rule across 1 scene(s).")).toBeInTheDocument();
    expect(order).toEqual(["flush", "check"]);
    expect(mocks.check).toHaveBeenCalledWith("p", "n1");
    expect(screen.getByText("Phrase to avoid: very")).toBeInTheDocument();
    expect(screen.getByText("…it was very quiet…")).toBeInTheDocument();
    expect(screen.getByText("Long sentence: 140 characters (limit 120)")).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "Scene 1 · paragraph 2" })[0]);
    expect(onOpenNode).toHaveBeenCalledWith("n1");
  });

  it("checks the whole work without a scene id", async () => {
    mocks.check.mockResolvedValue({ rules, scenes_checked: 12, total: 0, truncated: false, violations: [] });
    renderSection();
    await screen.findByLabelText(/Phrases to avoid/);
    fireEvent.click(screen.getByRole("button", { name: "Check the whole work" }));

    expect(await screen.findByText("No rule is broken in 12 scene(s).")).toBeInTheDocument();
    expect(mocks.check).toHaveBeenCalledWith("p", undefined);
  });

  // "No violations" and "nothing was checked" are different results, and the
  // second must never be shown as the first.
  it("says nothing was checked when no rules are saved", async () => {
    const empty = { project_id: "p", avoid_phrases: [], max_sentence_chars: 0, updated_at: 0 };
    mocks.getRules.mockResolvedValue(empty);
    mocks.check.mockResolvedValue({ rules: empty, scenes_checked: 3, total: 0, truncated: false, violations: [] });
    renderSection();
    await screen.findByLabelText(/Phrases to avoid/);
    fireEvent.click(screen.getByRole("button", { name: "Check the whole work" }));

    expect(await screen.findByText(/nothing was checked/)).toBeInTheDocument();
    expect(screen.queryByText(/No rule is broken/)).not.toBeInTheDocument();
  });

  it("translates the engine's refusal and drops a report made under the old rules", async () => {
    mocks.check.mockResolvedValue({ rules, scenes_checked: 1, total: 0, truncated: false, violations: [] });
    renderSection();
    await screen.findByLabelText(/Phrases to avoid/);
    fireEvent.click(screen.getByRole("button", { name: "Check the whole work" }));
    await screen.findByText("No rule is broken in 1 scene(s).");

    mocks.setRules.mockRejectedValueOnce({ code: -32602, message: "a phrase to avoid is at most 80 characters", data: { reason: "style_phrase_too_long" } });
    fireEvent.click(screen.getByRole("button", { name: "Save rules" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("A phrase to avoid must be 80 characters or fewer.");

    mocks.setRules.mockResolvedValueOnce(rules);
    fireEvent.click(screen.getByRole("button", { name: "Save rules" }));
    await waitFor(() => expect(screen.queryByText("No rule is broken in 1 scene(s).")).not.toBeInTheDocument());
  });
  it("lets the writer edit and save the style notes", async () => {
    mocks.updateProject.mockResolvedValue({ id: "p", style_notes: "Dry, plain, short." });
    renderSection();
    const box = await screen.findByLabelText(/Style notes/);
    expect(box).toHaveValue("Dry and plain.");
    fireEvent.change(box, { target: { value: "Dry, plain, short." } });
    fireEvent.click(screen.getByRole("button", { name: "Save notes" }));
    await waitFor(() => expect(mocks.updateProject).toHaveBeenCalledWith({ id: "p", style_notes: "Dry, plain, short." }));
  });

  it("shows no draft block when nothing is waiting", async () => {
    renderSection();
    await screen.findByLabelText(/Style notes/);
    expect(screen.queryByText("Style profile proposed by an agent")).not.toBeInTheDocument();
    expect(screen.queryByText("Draft waiting")).not.toBeInTheDocument();
  });

  // The draft is the agent's reading, not the writer's wish: it is shown apart
  // from the notes, and only the writer's click moves it.
  it("shows a proposed profile apart from the notes and approves the writer's edit of it", async () => {
    mocks.getDraft.mockResolvedValue(pendingDraft);
    mocks.approveDraft.mockResolvedValue({ style_notes: "Dry and plain.\n\nShort sentences." });
    renderSection();

    const draftBox = await screen.findByLabelText("Style profile draft");
    expect(draftBox).toHaveValue("Short sentences. Past tense.");
    expect(screen.getByText("Draft waiting")).toBeInTheDocument();
    expect(screen.getByText("28 / 2200 characters")).toBeInTheDocument();
    // Not yet in the notes.
    expect(screen.getByLabelText(/Style notes/)).toHaveValue("Dry and plain.");
    expect(mocks.approveDraft).not.toHaveBeenCalled();

    fireEvent.change(draftBox, { target: { value: "Short sentences." } });
    fireEvent.click(screen.getByRole("button", { name: "Add after my style notes" }));

    await waitFor(() => expect(mocks.approveDraft).toHaveBeenCalledWith("p", "Short sentences.", "append"));
    await waitFor(() => expect(screen.getByLabelText(/Style notes/)).toHaveValue("Dry and plain.\n\nShort sentences."));
    expect(screen.queryByLabelText("Style profile draft")).not.toBeInTheDocument();
  });

  it("can replace the notes with the draft, or discard it", async () => {
    mocks.getDraft.mockResolvedValue(pendingDraft);
    mocks.approveDraft.mockResolvedValue({ style_notes: "Short sentences. Past tense." });
    const first = renderSection();
    await screen.findByLabelText("Style profile draft");
    fireEvent.click(screen.getByRole("button", { name: "Replace my style notes with this" }));
    await waitFor(() => expect(mocks.approveDraft).toHaveBeenCalledWith("p", "Short sentences. Past tense.", "replace"));
    first.unmount();

    mocks.discardDraft.mockResolvedValue({ ok: true });
    renderSection();
    await screen.findByLabelText("Style profile draft");
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(mocks.discardDraft).toHaveBeenCalledWith("p"));
    await waitFor(() => expect(screen.queryByLabelText("Style profile draft")).not.toBeInTheDocument());
    expect(screen.getByLabelText(/Style notes/)).toHaveValue("Dry and plain.");
  });

  it("picks up a draft an agent proposes while the panel is open, for this work only", async () => {
    renderSection();
    await screen.findByLabelText(/Style notes/);
    expect(mocks.getDraft).toHaveBeenCalledTimes(1);

    act(() => mocks.listeners.get("mcp-changed")?.({ payload: { project_id: "other", tool: "linetta_propose_style_profile" } }));
    act(() => mocks.listeners.get("mcp-changed")?.({ payload: { project_id: "p", tool: "linetta_write_scene" } }));
    expect(mocks.getDraft).toHaveBeenCalledTimes(1);

    mocks.getDraft.mockResolvedValue(pendingDraft);
    act(() => mocks.listeners.get("mcp-changed")?.({ payload: { project_id: "p", tool: "linetta_propose_style_profile" } }));
    expect(await screen.findByLabelText("Style profile draft")).toHaveValue("Short sentences. Past tense.");
  });

  it("explains a refused approval and keeps the draft on screen", async () => {
    mocks.getDraft.mockResolvedValue(pendingDraft);
    mocks.approveDraft.mockRejectedValue({ code: -32602, message: "style notes would be over 2200 characters", data: { reason: "style_notes_too_long" } });
    renderSection();
    await screen.findByLabelText("Style profile draft");
    fireEvent.click(screen.getByRole("button", { name: "Add after my style notes" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("over 2200 characters");
    expect(screen.getByLabelText("Style profile draft")).toHaveValue("Short sentences. Past tense.");
  });
});
