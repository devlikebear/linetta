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

/** memory.get with its answers held back, so a test decides the order they
 *  land in.
 *
 *  The bug these exist for is an ordering bug, and an ordering bug written
 *  with timers is a race the test itself can lose. Here every reply is a
 *  promise a test resolves by hand: the interleaving is stated, not hoped for. */
const gets: { projectId: string; answer: (state: unknown) => void }[] = [];

function holdGets() {
  gets.length = 0;
  rpc.memoryGet.mockImplementation(
    (projectId: string) =>
      new Promise((resolve) => {
        gets.push({ projectId, answer: resolve });
      }),
  );
}

async function answerGet(index: number, state: unknown) {
  await act(async () => {
    gets[index].answer(state);
    await Promise.resolve();
  });
}

describe("MemorySection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    gets.length = 0;
    rpc.projectsList.mockResolvedValue([{ id: "work-1", title: "첫 작품" }]);
    // The emoji is load-bearing, not decoration: 📖 and 📕 are astral, so
    // `s.length` counts them twice and `[...s].length` — the engine's
    // utf8.RuneCountInString — counts them once. Every count assertion below
    // therefore fails outright if the pane is ever "simplified" to `.length`,
    // instead of agreeing with it by accident the way all-Hangul prose does.
    rpc.memoryGet.mockResolvedValue(memoryState("존댓말을 쓴다 📖", "민준은 3화부터 존댓말 📕"));
    rpc.memorySet.mockImplementation((scope: Scope, _id: string, body: string) =>
      Promise.resolve(doc(scope, body)),
    );
  });

  it("shows both memories with their character counts", async () => {
    const { profile, notes } = await mounted();

    expect(profile.value).toBe("존댓말을 쓴다 📖");
    expect(notes.value).toBe("민준은 3화부터 존댓말 📕");

    // Runes, not bytes and not UTF-16 units — the same unit the engine
    // enforces the budget in. `.length` would say 10 and 15 here, and a
    // counter reading 1399 while the engine refuses at 1401 is a trap.
    expect([..."존댓말을 쓴다 📖"].length).toBe(9);
    expect("존댓말을 쓴다 📖".length).toBe(10);
    expect(screen.getByTestId("memory-writer-profile-count")).toHaveTextContent(
      `settings.memory.remaining:9,${PROFILE_BUDGET}`,
    );
    expect(screen.getByTestId("memory-work-notes-count")).toHaveTextContent(
      `settings.memory.remaining:14,${NOTES_BUDGET}`,
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
    // 3 runes, 4 UTF-16 units: the live count has to move in runes too, not
    // just the one drawn on load.
    rpc.memoryGet.mockResolvedValue(memoryState("안녕📖", ""));
    const { profile } = await mounted();

    expect(screen.getByTestId("memory-writer-profile-count")).toHaveTextContent(
      `settings.memory.remaining:3,${PROFILE_BUDGET}`,
    );

    await userEvent.type(profile, "하세요");

    // The count follows the DRAFT, not the last saved document — otherwise it
    // reads as room the writer still has while they are already over budget.
    await waitFor(() =>
      expect(screen.getByTestId("memory-writer-profile-count")).toHaveTextContent(
        `settings.memory.remaining:6,${PROFILE_BUDGET}`,
      ),
    );
  });

  it("keeps the draft when the server refuses the save", async () => {
    rpc.memoryGet.mockResolvedValue(memoryState("안녕", ""));
    rpc.memorySet.mockRejectedValue(new Error("memory is over budget by 12 characters"));
    const { profile } = await mounted();

    await userEvent.type(profile, "하세요");
    await userEvent.tab();

    // Beside the document it was refused for. One shared line at the foot of
    // the section reads as "Settings is broken" when what actually happened is
    // "this one document is twelve characters too long".
    const error = await screen.findByTestId("memory-writer-profile-error");
    expect(error.textContent).toContain("over budget");
    expect(screen.queryByTestId("memory-work-notes-error")).toBeNull();
    expect(screen.queryByTestId("memory-error")).toBeNull();
    // Losing what the writer typed because it was one character too long is
    // the worst thing this pane can do. The text stays; the fix is one
    // keystroke away.
    expect(profileValue()).toBe("안녕하세요");
  });

  it("keeps what the writer typed while the save was in flight", async () => {
    rpc.memoryGet.mockResolvedValue(memoryState("안녕", ""));
    let confirmSave!: (document: unknown) => void;
    rpc.memorySet.mockImplementation(
      () =>
        new Promise((resolve) => {
          confirmSave = resolve;
        }),
    );
    const { profile } = await mounted();

    await userEvent.type(profile, "하세요");
    await userEvent.tab();
    await waitFor(() => expect(rpc.memorySet).toHaveBeenCalledTimes(1));

    // A writer does not stop typing because a save is travelling.
    await userEvent.type(profile, "!");
    await act(async () => {
      confirmSave(doc("writer_profile", "안녕하세요"));
    });

    // The confirmation is only ever allowed to replace the draft it was FOR.
    // Anything typed since is unsent text, and unsent text is the writer's.
    expect(profileValue()).toBe("안녕하세요!");
    // ...and it is genuinely unsaved, not merely still drawn: the next blur
    // has to send it.
    await userEvent.tab();
    await waitFor(() => expect(rpc.memorySet).toHaveBeenCalledTimes(2));
    expect(rpc.memorySet).toHaveBeenLastCalledWith("writer_profile", "", "안녕하세요!");
  });

  it("points the textarea at its help line and its running count", async () => {
    const { profile } = await mounted();
    const help = screen.getByTestId("memory-writer-profile-help");
    const count = screen.getByTestId("memory-writer-profile-count");

    // Sighted writers get the budget for free — it sits under the box. A
    // screen reader is told the label and then nothing at all, in a pane whose
    // entire interaction is staying under a limit, unless the box names the
    // lines that describe it.
    const describedBy = (profile.getAttribute("aria-describedby") ?? "").split(/\s+/);
    expect(describedBy).toContain(help.id);
    expect(describedBy).toContain(count.id);
    // And the count changes while the writer types, so it also has to be
    // announced. <output> is a polite live region natively — the same
    // reasoning as the provider test result (ProviderSection.tsx:820-828).
    expect(count.tagName).toBe("OUTPUT");
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

  it("drops a refetch that answers after the writer has switched works", async () => {
    rpc.projectsList.mockResolvedValue([
      { id: "work-1", title: "첫 작품" },
      { id: "work-2", title: "두 번째 작품" },
    ]);
    holdGets();
    render(<MemorySection />);

    await waitFor(() => expect(gets.length).toBe(1));
    expect(gets[0].projectId).toBe("work-1");
    await answerGet(0, memoryState("프로필", "첫 작품 노트"));
    await screen.findByTestId("memory-work-notes-count");

    // An agent writes while work-1 is on screen. The refetch goes out for
    // work-1 and does not come back yet.
    await fireChanged({ scope: "work_notes", project_id: "work-1", source: "agent" });
    await waitFor(() => expect(gets.length).toBe(2));
    expect(gets[1].projectId).toBe("work-1");

    // The writer moves the picker before it answers, and work-2 answers first.
    await userEvent.selectOptions(screen.getByTestId("memory-work"), "work-2");
    await waitFor(() => expect(gets.length).toBe(3));
    expect(gets[2].projectId).toBe("work-2");
    await answerGet(2, memoryState("프로필", "두 번째 작품 노트"));
    await waitFor(() => expect(notesValue()).toBe("두 번째 작품 노트"));

    // Only now does work-1's stale reply land.
    await answerGet(1, memoryState("프로필", "첫 작품 노트"));

    expect(notesValue()).toBe("두 번째 작품 노트");
    // The real damage is one step further on: with work-1's notes in the box
    // under a picker reading work-2, the next blur saves them ONTO work-2.
    await userEvent.click(screen.getByTestId("memory-work-notes"));
    await userEvent.tab();
    expect(rpc.memorySet).not.toHaveBeenCalled();
  });

  it("drops a work's first load when it answers after the writer has moved on", async () => {
    rpc.projectsList.mockResolvedValue([
      { id: "work-1", title: "첫 작품" },
      { id: "work-2", title: "두 번째 작품" },
    ]);
    holdGets();
    render(<MemorySection />);

    const picker = (await screen.findByTestId("memory-work")) as HTMLSelectElement;
    await waitFor(() => expect(picker.options.length).toBe(2));
    await waitFor(() => expect(gets.length).toBe(1));

    // work-1's load is still travelling when the writer picks work-2.
    await userEvent.selectOptions(picker, "work-2");
    await waitFor(() => expect(gets.length).toBe(2));
    await answerGet(1, memoryState("프로필", "두 번째 작품 노트"));
    await screen.findByTestId("memory-work-notes-count");

    await answerGet(0, memoryState("프로필", "첫 작품 노트"));

    expect(notesValue()).toBe("두 번째 작품 노트");
    await userEvent.click(screen.getByTestId("memory-work-notes"));
    await userEvent.tab();
    expect(rpc.memorySet).not.toHaveBeenCalled();
  });

  it("does not warn about a change to another work's notes", async () => {
    rpc.projectsList.mockResolvedValue([
      { id: "work-1", title: "첫 작품" },
      { id: "work-2", title: "두 번째 작품" },
    ]);
    rpc.memoryGet.mockResolvedValue(memoryState("프로필", "첫 작품 노트"));
    const { notes } = await mounted();

    await userEvent.type(notes, " 추가");
    const before = rpc.memoryGet.mock.calls.length;

    await fireChanged({ scope: "work_notes", project_id: "work-2", source: "agent" });

    // The notice is a claim about the writer's own unsaved text — "saving will
    // overwrite what just changed". Nothing changed under this box, so the
    // claim would simply be false, and the notice has no dismiss. There is
    // nothing to fetch either.
    expect(rpc.memoryGet.mock.calls.length).toBe(before);
    expect(screen.queryByTestId("memory-work-notes-changed")).toBeNull();
    expect(notesValue()).toBe("첫 작품 노트 추가");

    // The same event for the work ON SCREEN is the one the notice exists for.
    await fireChanged({ scope: "work_notes", project_id: "work-1", source: "agent" });
    await screen.findByTestId("memory-work-notes-changed");
    expect(notesValue()).toBe("첫 작품 노트 추가");
  });

  it("tells the writer's own save apart from another window's", async () => {
    rpc.memoryGet.mockResolvedValue(memoryState("안녕", ""));
    const { profile } = await mounted();

    await userEvent.type(profile, "하세요");
    await userEvent.tab();
    await waitFor(() => expect(rpc.memorySet).toHaveBeenCalledTimes(1));

    // Typing again makes the box dirty, so the notice is now possible — and
    // our own save is about to come back round as source "writer".
    await userEvent.type(profile, "!");
    await fireChanged({ scope: "writer_profile", source: "writer" });
    expect(screen.queryByTestId("memory-writer-profile-changed")).toBeNull();

    // A second writer-sourced event has no save of ours left to account for
    // it, so it is a person in another window editing the same document —
    // which types.ts:1003-1005 defines as a genuine elsewhere-edit.
    await fireChanged({ scope: "writer_profile", source: "writer" });
    await screen.findByTestId("memory-writer-profile-changed");
  });
});
