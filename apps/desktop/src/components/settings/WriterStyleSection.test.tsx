import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  get: vi.fn(), set: vi.fn(), setNotes: vi.fn(), getDraft: vi.fn(), approve: vi.fn(), discard: vi.fn(),
}));
const events = vi.hoisted(() => ({ handlers: new Map<string, (payload: unknown) => void>() }));

vi.mock("../../lib/rpc", () => ({
  style: {
    getWriterRules: rpc.get, setWriterRules: rpc.set, setWriterNotes: rpc.setNotes,
    getWriterDraft: rpc.getDraft, approveWriterDraft: rpc.approve, discardWriterDraft: rpc.discard,
  },
}));
vi.mock("../../hooks/useEngineEvent", () => ({
  useEngineEvent: (name: string, handler: (payload: unknown) => void) => {
    events.handlers.set(name, handler);
  },
}));
vi.mock("../../lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string | number>) =>
      vars ? `${key}:${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(",")}` : key,
  }),
}));
vi.mock("../../lib/rpcMessage", () => ({
  rpcErrorMessage: (error: { data?: { reason?: string } }) => `error:${error.data?.reason ?? "unknown"}`,
}));

import { WriterStyleSection } from "./WriterStyleSection";

const noDraft = { pending: false, draft: { project_id: "", body: "", author: "", created_at: 0 }, limit: 1000 };
const pending = { pending: true, draft: { project_id: "", body: "Present tense throughout.", author: "agent", created_at: 1 }, limit: 1000 };

describe("WriterStyleSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    events.handlers.clear();
    rpc.get.mockResolvedValue({ avoid_phrases: ["that was"], max_sentence_chars: 80, updated_at: 1, notes: "Short sentences.", notes_limit: 1000 });
    rpc.getDraft.mockResolvedValue(noDraft);
  });

  it("shows the writer's rules and saves what was typed", async () => {
    rpc.set.mockResolvedValue({ avoid_phrases: ["that was", "suddenly"], max_sentence_chars: 0, updated_at: 2 });
    render(<WriterStyleSection />);
    const box = await screen.findByLabelText("style.avoidPhrases");
    expect(box).toHaveValue("that was");
    expect(screen.getByLabelText("style.maxSentence")).toHaveValue(80);

    fireEvent.change(box, { target: { value: "that was\n  suddenly \n\n" } });
    fireEvent.change(screen.getByLabelText("style.maxSentence"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "style.save" }));

    await waitFor(() => expect(rpc.set).toHaveBeenCalledWith({ avoid_phrases: ["that was", "suddenly"], max_sentence_chars: 0 }));
    expect(await screen.findByRole("status")).toHaveTextContent("settings.writerStyle.saved");
  });

  // #182: the notes, with a count against their limit.
  it("edits the writer's style notes and says how long they are", async () => {
    rpc.setNotes.mockResolvedValue({ notes: "Present tense." });
    render(<WriterStyleSection />);
    const notes = await screen.findByLabelText(/settings.writerStyle.notes/);
    expect(notes).toHaveValue("Short sentences.");
    fireEvent.change(notes, { target: { value: "Present tense." } });
    expect(screen.getByTestId("writer-style-notes-count")).toHaveTextContent("count=14,limit=1000");
    fireEvent.click(screen.getByRole("button", { name: "style.saveNotes" }));
    await waitFor(() => expect(rpc.setNotes).toHaveBeenCalledWith("Present tense."));
  });

  it("says why a save was refused", async () => {
    rpc.setNotes.mockRejectedValue({ data: { reason: "style_writer_notes_too_long" } });
    render(<WriterStyleSection />);
    fireEvent.click(await screen.findByRole("button", { name: "style.saveNotes" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("error:style_writer_notes_too_long");
  });

  // #183: an agent's proposal waits here for the writer.
  it("approves the writer's edit of a proposal, appended to the notes", async () => {
    rpc.getDraft.mockResolvedValue(pending);
    rpc.approve.mockResolvedValue({ notes: "Short sentences.\n\nPresent tense, always." });
    render(<WriterStyleSection />);
    const draft = await screen.findByLabelText("style.draftBody");
    fireEvent.change(draft, { target: { value: "Present tense, always." } });
    fireEvent.click(screen.getByRole("button", { name: "style.draftAppend" }));
    await waitFor(() => expect(rpc.approve).toHaveBeenCalledWith("Present tense, always.", "append"));
    await waitFor(() => expect(screen.queryByTestId("writer-style-draft")).toBeNull());
    expect(screen.getByLabelText(/settings.writerStyle.notes/)).toHaveValue("Short sentences.\n\nPresent tense, always.");
  });

  it("picks up a writer-wide proposal while open, and ignores a work's", async () => {
    render(<WriterStyleSection />);
    await screen.findByLabelText(/settings.writerStyle.notes/);
    expect(rpc.getDraft).toHaveBeenCalledTimes(1);
    act(() => events.handlers.get("mcp-changed")?.({ tool: "linetta_propose_style_profile", project_id: "p1" }));
    expect(rpc.getDraft).toHaveBeenCalledTimes(1);
    rpc.getDraft.mockResolvedValue(pending);
    act(() => events.handlers.get("mcp-changed")?.({ tool: "linetta_propose_style_profile", project_id: "" }));
    expect(await screen.findByTestId("writer-style-draft")).toBeInTheDocument();
  });

  it("discards a proposal", async () => {
    rpc.getDraft.mockResolvedValue(pending);
    rpc.discard.mockResolvedValue({ ok: true });
    render(<WriterStyleSection />);
    fireEvent.click(await screen.findByRole("button", { name: "style.draftDiscard" }));
    await waitFor(() => expect(rpc.discard).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByTestId("writer-style-draft")).toBeNull());
  });
});
