import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

/** The Tauri event bus, reduced to the one thing this pane needs: a place to
 *  put `memory-changed` and a way to fire it. */
const ev = vi.hoisted(() => ({
  listeners: new Map<string, (e: { payload: unknown }) => void>(),
}));

vi.mock("@tauri-apps/api/event", () => ({
  listen: (event: string, cb: (e: { payload: unknown }) => void) => {
    ev.listeners.set(event, cb);
    return Promise.resolve(() => ev.listeners.delete(event));
  },
}));

const rpc = vi.hoisted(() => ({
  memoryGet: vi.fn(),
  memorySet: vi.fn(),
  projectsList: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  memory: { get: rpc.memoryGet, set: rpc.memorySet },
  projects: { list: rpc.projectsList },
}));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { MemorySection } from "./MemorySection";

// The engine's budgets, in runes (agentmemory/agentmemory.go:25-27). The pane
// never invents them — it draws the capacity line from what memory.get sends —
// but the fixtures have to carry the real numbers for the assertions to mean
// anything.
const PROFILE_BUDGET = 1400;
const NOTES_BUDGET = 2200;

type Scope = "writer_profile" | "work_notes";

function doc(scope: Scope, body: string) {
  return {
    scope,
    body,
    chars_used: [...body].length,
    chars_budget: scope === "writer_profile" ? PROFILE_BUDGET : NOTES_BUDGET,
    updated_at: 1780200000000,
  };
}

function memoryState(profile: string, notes: string) {
  return { writer_profile: doc("writer_profile", profile), work_notes: doc("work_notes", notes) };
}

/** Mount and wait for the first memory.get to have landed.
 *
 *  The count lines only exist once a document is in hand — the pane refuses to
 *  invent a budget — so their appearance is the honest "loaded" barrier. Typing
 *  before it would race the load, and the race would be the test's, not the
 *  pane's. */
async function mounted() {
  render(<MemorySection />);
  await screen.findByTestId("memory-writer-profile-count");
  return {
    profile: screen.getByTestId("memory-writer-profile") as HTMLTextAreaElement,
    notes: screen.getByTestId("memory-work-notes") as HTMLTextAreaElement,
  };
}

const profileValue = () => (screen.getByTestId("memory-writer-profile") as HTMLTextAreaElement).value;
const notesValue = () => (screen.getByTestId("memory-work-notes") as HTMLTextAreaElement).value;

async function fireChanged(payload: Record<string, unknown>) {
  const fire = ev.listeners.get("memory-changed");
  expect(fire, "the pane never subscribed to memory-changed").toBeTruthy();
  await act(async () => {
    fire?.({ payload });
  });
}

describe("MemorySection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    rpc.projectsList.mockResolvedValue([{ id: "work-1", title: "첫 작품" }]);
    rpc.memoryGet.mockResolvedValue(memoryState("존댓말을 쓴다", "민준은 3화부터 존댓말"));
    rpc.memorySet.mockImplementation((scope: Scope, _id: string, body: string) =>
      Promise.resolve(doc(scope, body)),
    );
  });

  it("shows both memories with their character counts", async () => {
    const { profile, notes } = await mounted();

    expect(profile.value).toBe("존댓말을 쓴다");
    expect(notes.value).toBe("민준은 3화부터 존댓말");

    // Runes, not bytes — the same unit the engine enforces the budget in.
    expect(screen.getByTestId("memory-writer-profile-count")).toHaveTextContent(
      `settings.memory.remaining:7,${PROFILE_BUDGET}`,
    );
    expect(screen.getByTestId("memory-work-notes-count")).toHaveTextContent(
      `settings.memory.remaining:12,${NOTES_BUDGET}`,
    );
  });

  it("saves on blur, not on every keystroke", async () => {
    rpc.memoryGet.mockResolvedValue(memoryState("", ""));
    const { profile } = await mounted();

    await userEvent.type(profile, "가나다라마바사아자차");

    // Ten keystrokes are ten RPCs if the textarea is bound straight to the
    // engine — and ten memory.changed events bouncing back at the pane.
    expect(rpc.memorySet).not.toHaveBeenCalled();

    await userEvent.tab();

    await waitFor(() => expect(rpc.memorySet).toHaveBeenCalledTimes(1));
    expect(rpc.memorySet).toHaveBeenCalledWith("writer_profile", "", "가나다라마바사아자차");
  });

  it("does not save when the text is unchanged", async () => {
    const { profile } = await mounted();

    await userEvent.click(profile);
    await userEvent.tab();

    // A blur is not an edit. Saving anyway rewrites updated_at and fires a
    // memory.changed at every other window for a document nobody touched.
    await waitFor(() => expect(rpc.memorySet).not.toHaveBeenCalled());
  });

  it("counts down the remaining characters as the writer types", async () => {
    rpc.memoryGet.mockResolvedValue(memoryState("안녕", ""));
    const { profile } = await mounted();

    expect(screen.getByTestId("memory-writer-profile-count")).toHaveTextContent(
      `settings.memory.remaining:2,${PROFILE_BUDGET}`,
    );

    await userEvent.type(profile, "하세요");

    // The count follows the DRAFT, not the last saved document — otherwise it
    // reads as room the writer still has while they are already over budget.
    await waitFor(() =>
      expect(screen.getByTestId("memory-writer-profile-count")).toHaveTextContent(
        `settings.memory.remaining:5,${PROFILE_BUDGET}`,
      ),
    );
  });

  it("keeps the draft when the server refuses the save", async () => {
    rpc.memoryGet.mockResolvedValue(memoryState("안녕", ""));
    rpc.memorySet.mockRejectedValue(new Error("memory is over budget by 12 characters"));
    const { profile } = await mounted();

    await userEvent.type(profile, "하세요");
    await userEvent.tab();

    const error = await screen.findByTestId("memory-error");
    expect(error.textContent).toContain("over budget");
    // Losing what the writer typed because it was one character too long is
    // the worst thing this pane can do. The text stays; the fix is one
    // keystroke away.
    expect(profileValue()).toBe("안녕하세요");
  });

  it("reloads when an agent changes a memory underneath", async () => {
    await mounted();
    const before = rpc.memoryGet.mock.calls.length;

    rpc.memoryGet.mockResolvedValue(memoryState("에이전트가 쓴 프로필", "에이전트가 쓴 노트"));
    await fireChanged({ scope: "writer_profile", source: "agent" });

    await waitFor(() => expect(rpc.memoryGet.mock.calls.length).toBeGreaterThan(before));
    // A clean textarea is the engine's, so it shows what the agent wrote.
    await waitFor(() => expect(profileValue()).toBe("에이전트가 쓴 프로필"));
    expect(screen.queryByTestId("memory-writer-profile-changed")).toBeNull();
  });

  it("does not clobber an unsent draft when an agent writes", async () => {
    rpc.memoryGet.mockResolvedValue(memoryState("안녕", ""));
    const { profile } = await mounted();

    await userEvent.type(profile, "하세요");

    rpc.memoryGet.mockResolvedValue(memoryState("에이전트가 쓴 프로필", ""));
    await fireChanged({ scope: "writer_profile", source: "agent" });

    // The refetch happens, but the textarea belongs to the writer until they
    // save it. Overwriting it here destroys text nothing can recover — so the
    // pane says what happened instead.
    await screen.findByTestId("memory-writer-profile-changed");
    expect(profileValue()).toBe("안녕하세요");
    expect(rpc.memorySet).not.toHaveBeenCalled();
  });

  it("switches work notes when the work picker changes", async () => {
    rpc.projectsList.mockResolvedValue([
      { id: "work-1", title: "첫 작품" },
      { id: "work-2", title: "두 번째 작품" },
    ]);
    rpc.memoryGet.mockImplementation((projectId: string) =>
      Promise.resolve(
        memoryState("존댓말을 쓴다", projectId === "work-2" ? "두 번째 작품 노트" : "첫 작품 노트"),
      ),
    );
    const { notes } = await mounted();

    expect(notes.value).toBe("첫 작품 노트");

    await userEvent.selectOptions(screen.getByTestId("memory-work"), "work-2");

    await waitFor(() => expect(rpc.memoryGet).toHaveBeenCalledWith("work-2"));
    await waitFor(() => expect(notesValue()).toBe("두 번째 작품 노트"));
    // The profile is global; switching works must not disturb it.
    expect(profileValue()).toBe("존댓말을 쓴다");
  });
});
