import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

/** The Tauri event bus, reduced to the one thing this pane needs: a place to
 *  put `skills-changed` and a way to fire it. */
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
  list: vi.fn(),
  read: vi.fn(),
  write: vi.fn(),
  del: vi.fn(),
  history: vi.fn(),
  restore: vi.fn(),
  projectsList: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  skills: {
    list: rpc.list,
    read: rpc.read,
    write: rpc.write,
    delete: rpc.del,
    history: rpc.history,
    restore: rpc.restore,
  },
  projects: { list: rpc.projectsList },
}));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { SkillsSection, diagnosticTarget } from "./SkillsSection";

// agentskills.MaxBodyRunes. The pane never invents it — it draws the capacity
// line from what the engine sends — but the fixtures have to carry the real
// number for the assertions to mean anything.
const BODY_BUDGET = 8000;

type Over = Record<string, unknown>;

function summary(over: Over = {}) {
  return {
    name: "dialogue-beats",
    scope: "writer",
    description: "대사 사이 호흡을 넣는 법",
    author: "writer",
    enabled: true,
    updated_at: 1780200000000,
    body_runes: 4,
    body_budget: BODY_BUDGET,
    ...over,
  };
}

function full(over: Over = {}) {
  const s = summary(over);
  return { body: "짧게 끊는다 📖", ...s, ...over };
}

function listResult(skills: Over[], diagnostics: Over[] = []) {
  return { skills, diagnostics };
}

/** Mount and wait for the first skills.list to have landed. */
async function mounted() {
  render(<SkillsSection />);
  await screen.findByTestId("skills-list");
}

const bodyBox = () => screen.getByTestId("skill-body") as HTMLTextAreaElement;
const descBox = () => screen.getByTestId("skill-description") as HTMLInputElement;

async function fireChanged(payload: Record<string, unknown>) {
  const fire = ev.listeners.get("skills-changed");
  expect(fire, "the pane never subscribed to skills-changed").toBeTruthy();
  await act(async () => {
    fire?.({ payload });
  });
}

/** skills.list / skills.read with their answers held back, so a test decides
 *  the order they land in.
 *
 *  The bugs these exist for are ordering bugs, and an ordering bug written
 *  with timers is a race the test itself can lose. Here every reply is a
 *  promise a test resolves by hand: the interleaving is stated, not hoped for. */
const lists: { projectId: string; answer: (v: unknown) => void }[] = [];
const reads: { scope: string; name: string; answer: (v: unknown) => void }[] = [];

function holdLists() {
  lists.length = 0;
  rpc.list.mockImplementation(
    (projectId: string) => new Promise((resolve) => lists.push({ projectId, answer: resolve })),
  );
}

function holdReads() {
  reads.length = 0;
  rpc.read.mockImplementation(
    (scope: string, _id: string, name: string) =>
      new Promise((resolve) => reads.push({ scope, name, answer: resolve })),
  );
}

async function settle(answer: () => void) {
  await act(async () => {
    answer();
    await Promise.resolve();
  });
}

/** A diagnostic carries a PATH and nothing else, so the opener has to read the
 *  skill's identity back out of it. `agentskills.Store.Dir` is the layout it
 *  reads against, and Windows is why the separators are normalised — the
 *  engine builds these paths with `filepath.Join`, and this app ships there. */
describe("diagnosticTarget", () => {
  it("reads a writer-scope skill out of its path", () => {
    expect(diagnosticTarget("/home/l/skills/dialogue-beats/SKILL.md", "work-1")).toEqual({
      scope: "writer",
      name: "dialogue-beats",
    });
    // No SKILL.md in the directory: the diagnostic points at the directory
    // itself, and the directory's name is still the skill's name.
    expect(diagnosticTarget("/home/l/skills/dialogue-beats", "work-1")).toEqual({
      scope: "writer",
      name: "dialogue-beats",
    });
  });

  it("reads a work-scope skill out of its path, on either platform", () => {
    expect(diagnosticTarget("/home/l/skills/works/work-1/cliffhangers/SKILL.md", "work-1")).toEqual({
      scope: "work",
      name: "cliffhangers",
    });
    expect(
      diagnosticTarget("C:\\Users\\l\\skills\\works\\work-1\\cliffhangers\\SKILL.md", "work-1"),
    ).toEqual({ scope: "work", name: "cliffhangers" });
  });

  it("refuses to guess at a path it does not recognise", () => {
    // A wrong guess sends skills.read after a name that is not there and the
    // writer gets "not found" for a file they can see on disk. The diagnostic
    // is still SHOWN — it just loses its opener, which is the honest trade.
    expect(diagnosticTarget("/somewhere/else/thing/SKILL.md", "work-1")).toBeNull();
    // Another work's skill, listed because the writer switched works between
    // the list and the click.
    expect(diagnosticTarget("/home/l/skills/works/work-2/x/SKILL.md", "work-1")).toBeNull();
    expect(diagnosticTarget("", "work-1")).toBeNull();
  });
});

describe("SkillsSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    ev.listeners.clear();
    lists.length = 0;
    reads.length = 0;
    rpc.projectsList.mockResolvedValue([{ id: "work-1", title: "첫 작품" }]);
    rpc.list.mockResolvedValue(listResult([summary()]));
    rpc.read.mockResolvedValue(full());
    rpc.write.mockImplementation((input: Over) =>
      Promise.resolve({ ...full(), ...input, project_id: undefined, versioned: true }),
    );
    rpc.del.mockResolvedValue({ versioned: true });
    rpc.history.mockResolvedValue({ versions: [] });
  });

  it("names who wrote each skill", async () => {
    rpc.list.mockResolvedValue(
      listResult([
        summary({ name: "dialogue-beats", author: "writer" }),
        summary({ name: "cliffhangers", author: "agent", description: "회차 끝맺기" }),
      ]),
    );
    await mounted();

    // The badge is the whole substitute for an approval gate: an agent writes a
    // skill without asking, and what the writer gets instead is attribution,
    // a history and a switch. A badge that is not on screen means that trade
    // was never made.
    expect(screen.getByTestId("skill-author-writer-dialogue-beats")).toHaveTextContent(
      "settings.skills.author.writer",
    );
    expect(screen.getByTestId("skill-author-writer-cliffhangers")).toHaveTextContent(
      "settings.skills.author.agent",
    );
  });

  it("shows a skill it could not read instead of hiding it", async () => {
    rpc.list.mockResolvedValue(
      listResult(
        [summary()],
        [{ path: "/home/skills/broken-one/SKILL.md", message: "agentskills: no frontmatter" }],
      ),
    );
    await mounted();

    // A broken SKILL.md is never listed as a skill — it must not reach a
    // prompt — so the diagnostic is the ONLY thing that tells the writer why
    // the skill they wrote is missing. Hiding it makes three tasks of engine
    // work deliver nothing.
    const diag = await screen.findByTestId("skill-diagnostic-0");
    expect(diag).toHaveAttribute("role", "alert");
    expect(diag).toHaveTextContent("settings.skills.broken:/home/skills/broken-one/SKILL.md");
    expect(diag).toHaveTextContent("no frontmatter");
  });

  it("opens a broken skill on its raw text and saves the repair", async () => {
    rpc.list.mockResolvedValue(
      listResult(
        [],
        [{ path: "/home/skills/broken-one/SKILL.md", message: "agentskills: no frontmatter" }],
      ),
    );
    // skills.read does not screen what it opens: that is the repair path. A
    // file whose frontmatter the writer broke comes back VERBATIM, with the
    // reason attached.
    rpc.read.mockResolvedValue({
      ...full({ name: "broken-one", description: "", body: "--\nname: broken-one\n--\n본문" }),
      parse_error: "agentskills: no frontmatter",
    });
    await mounted();

    await userEvent.click(screen.getByTestId("skill-diagnostic-open-0"));

    await waitFor(() => expect(rpc.read).toHaveBeenCalledWith("writer", "work-1", "broken-one"));
    // The reason has to be on screen beside the text: a writer handed a body
    // with no explanation would save it straight back and wonder why the
    // skill still does not appear.
    const why = await screen.findByTestId("skill-parse-error");
    expect(why).toHaveAttribute("role", "alert");
    expect(why).toHaveTextContent("no frontmatter");
    expect(bodyBox().value).toBe("--\nname: broken-one\n--\n본문");

    await userEvent.clear(descBox());
    await userEvent.type(descBox(), "고친 설명");
    await userEvent.tab();

    await waitFor(() => expect(rpc.write).toHaveBeenCalledTimes(1));
    expect(rpc.write).toHaveBeenCalledWith({
      scope: "writer",
      projectId: "work-1",
      name: "broken-one",
      description: "고친 설명",
      body: "--\nname: broken-one\n--\n본문",
    });
  });

  it("saves on blur, not on every keystroke", async () => {
    rpc.read.mockResolvedValue(full({ body: "" }));
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    await screen.findByTestId("skill-body");

    await userEvent.type(bodyBox(), "가나다라마바사아자차");

    // Ten keystrokes are ten RPCs if the textarea is bound straight to the
    // engine — and ten skills-changed events bouncing back at this pane.
    expect(rpc.write).not.toHaveBeenCalled();

    await userEvent.tab();

    await waitFor(() => expect(rpc.write).toHaveBeenCalledTimes(1));
    expect(rpc.write).toHaveBeenCalledWith({
      scope: "writer",
      projectId: "work-1",
      name: "dialogue-beats",
      description: "대사 사이 호흡을 넣는 법",
      body: "가나다라마바사아자차",
    });
  });

  it("does not save when nothing was changed", async () => {
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    await screen.findByTestId("skill-body");

    await userEvent.click(bodyBox());
    await userEvent.tab();

    // A blur is not an edit. Saving anyway restamps updated_at, records a
    // version row for a change nobody made, and fires skills-changed at every
    // other window.
    await waitFor(() => expect(rpc.write).not.toHaveBeenCalled());
  });

  it("counts the body in runes, the way the engine does", async () => {
    rpc.read.mockResolvedValue(full({ body: "안녕📖" }));
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));

    // 📖 is astral: `.length` says 4, the engine's utf8.RuneCountInString says
    // 3. A counter reading 7999 while the engine refuses at 8001 is a trap, so
    // this assertion fails outright if the count is ever "simplified".
    expect([..."안녕📖"].length).toBe(3);
    expect("안녕📖".length).toBe(4);
    const count = await screen.findByTestId("skill-body-count");
    expect(count).toHaveTextContent(`settings.skills.remaining:3,${BODY_BUDGET}`);

    await userEvent.type(bodyBox(), "하세요");

    // The count follows the DRAFT, not the last saved document — otherwise it
    // reads as room the writer still has while they are already over budget.
    await waitFor(() =>
      expect(screen.getByTestId("skill-body-count")).toHaveTextContent(
        `settings.skills.remaining:6,${BODY_BUDGET}`,
      ),
    );
  });

  it("keeps the draft when the engine refuses the save", async () => {
    rpc.read.mockResolvedValue(full({ body: "안녕" }));
    rpc.write.mockRejectedValue(new Error("agentskills: body is 8012 runes, over the 8000-rune limit"));
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    await screen.findByTestId("skill-body");

    await userEvent.type(bodyBox(), "하세요");
    await userEvent.tab();

    const error = await screen.findByTestId("skill-save-error");
    expect(error).toHaveAttribute("role", "alert");
    expect(error.textContent).toContain("over the 8000-rune limit");
    // Losing what the writer typed because it was twelve runes too long is the
    // worst thing this pane can do. The text stays; the fix is a few
    // keystrokes away.
    expect(bodyBox().value).toBe("안녕하세요");
  });

  it("keeps an unsent draft when an agent writes underneath, and says so", async () => {
    rpc.read.mockResolvedValue(full({ body: "안녕" }));
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    await screen.findByTestId("skill-body");

    await userEvent.type(bodyBox(), "하세요");

    rpc.read.mockResolvedValue(full({ body: "에이전트가 쓴 본문" }));
    await fireChanged({ scope: "writer", name: "dialogue-beats", source: "agent" });

    // The refetch happens, but the textarea belongs to the writer until they
    // save it. Overwriting it here destroys text nothing can recover — so the
    // pane says what happened instead.
    const notice = await screen.findByTestId("skill-changed");
    expect(notice).toHaveAttribute("role", "alert");
    expect(notice).toHaveTextContent("settings.skills.changedElsewhere");
    expect(bodyBox().value).toBe("안녕하세요");
    expect(rpc.write).not.toHaveBeenCalled();
  });

  it("takes an agent's rewrite into a clean textarea", async () => {
    rpc.read.mockResolvedValue(full({ body: "안녕" }));
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    await screen.findByTestId("skill-body");

    rpc.read.mockResolvedValue(full({ body: "에이전트가 쓴 본문" }));
    await fireChanged({ scope: "writer", name: "dialogue-beats", source: "agent" });

    await waitFor(() => expect(bodyBox().value).toBe("에이전트가 쓴 본문"));
    expect(screen.queryByTestId("skill-changed")).toBeNull();
  });

  it("ignores a change to a skill in another work", async () => {
    await mounted();
    const before = rpc.list.mock.calls.length;

    await fireChanged({ scope: "work", project_id: "work-9", name: "other", source: "agent" });

    // Nothing on this screen can be showing work-9's skills, so there is
    // nothing to refetch and nothing to warn about.
    expect(rpc.list.mock.calls.length).toBe(before);
  });

  it("drops a detail read that answers after the writer opened another skill", async () => {
    rpc.list.mockResolvedValue(
      listResult([summary({ name: "aaa" }), summary({ name: "bbb" })]),
    );
    holdReads();
    await mounted();

    await userEvent.click(screen.getByTestId("skill-open-writer-aaa"));
    await waitFor(() => expect(reads.length).toBe(1));
    expect(reads[0].name).toBe("aaa");

    // The writer moves on before it answers, and bbb answers first.
    await userEvent.click(screen.getByTestId("skill-open-writer-bbb"));
    await waitFor(() => expect(reads.length).toBe(2));
    await settle(() => reads[1].answer(full({ name: "bbb", body: "bbb의 본문" })));
    await waitFor(() => expect(bodyBox().value).toBe("bbb의 본문"));

    // Only now does aaa's stale reply land.
    await settle(() => reads[0].answer(full({ name: "aaa", body: "aaa의 본문" })));

    expect(bodyBox().value).toBe("bbb의 본문");
    expect(screen.getByTestId("skill-detail-name")).toHaveTextContent("bbb");
    // The real damage is one step on: with aaa's body in the box under a
    // heading reading bbb, the next blur saves it ONTO bbb.
    await userEvent.click(bodyBox());
    await userEvent.tab();
    expect(rpc.write).not.toHaveBeenCalled();
  });

  it("drops a list reply that answers after the writer switched works", async () => {
    rpc.projectsList.mockResolvedValue([
      { id: "work-1", title: "첫 작품" },
      { id: "work-2", title: "두 번째 작품" },
    ]);
    holdLists();
    render(<SkillsSection />);

    await waitFor(() => expect(lists.length).toBe(1));
    expect(lists[0].projectId).toBe("work-1");

    // The writer moves the picker before work-1 answers, and work-2 answers
    // first.
    const picker = (await screen.findByTestId("skills-work")) as HTMLSelectElement;
    await waitFor(() => expect(picker.options.length).toBe(2));
    await userEvent.selectOptions(picker, "work-2");
    await waitFor(() => expect(lists.length).toBe(2));
    await settle(() =>
      lists[1].answer(listResult([summary({ name: "work-two-skill", scope: "work" })])),
    );
    await screen.findByTestId("skill-open-work-work-two-skill");

    // Only now does work-1's stale reply land.
    await settle(() => lists[0].answer(listResult([summary({ name: "work-one-skill", scope: "work" })])));

    expect(screen.queryByTestId("skill-open-work-work-one-skill")).toBeNull();
    expect(screen.getByTestId("skill-open-work-work-two-skill")).toBeInTheDocument();
  });

  it("drops a save's confirmation once the writer has opened another skill", async () => {
    rpc.list.mockResolvedValue(listResult([summary({ name: "aaa" }), summary({ name: "bbb" })]));
    rpc.read.mockImplementation((_s: string, _p: string, name: string) =>
      Promise.resolve(full({ name, body: `${name}의 본문` })),
    );
    let confirmSave!: (v: unknown) => void;
    rpc.write.mockImplementation(() => new Promise((resolve) => (confirmSave = resolve)));
    await mounted();

    await userEvent.click(screen.getByTestId("skill-open-writer-aaa"));
    await waitFor(() => expect(bodyBox().value).toBe("aaa의 본문"));
    await userEvent.type(bodyBox(), " 추가");
    await userEvent.tab();
    await waitFor(() => expect(rpc.write).toHaveBeenCalledTimes(1));

    await userEvent.click(screen.getByTestId("skill-open-writer-bbb"));
    await waitFor(() => expect(bodyBox().value).toBe("bbb의 본문"));

    // aaa's save confirms late. Applying it here would put aaa's body in the
    // box while the heading reads bbb.
    await act(async () => {
      confirmSave({ ...full({ name: "aaa", body: "aaa의 본문 추가" }), versioned: true });
    });

    expect(bodyBox().value).toBe("bbb의 본문");
    expect(screen.getByTestId("skill-detail-name")).toHaveTextContent("bbb");
    // And the damage that does not show on screen: aaa's confirmation is also
    // what "saved" is compared against. Let it land and bbb's untouched box is
    // dirty against aaa's text, so merely leaving it writes aaa's body onto
    // bbb.
    await userEvent.click(bodyBox());
    await userEvent.tab();
    expect(rpc.write).toHaveBeenCalledTimes(1);
  });

  it("keeps what the writer typed while the save was in flight", async () => {
    rpc.read.mockResolvedValue(full({ body: "안녕" }));
    let confirmSave!: (v: unknown) => void;
    rpc.write.mockImplementation(() => new Promise((resolve) => (confirmSave = resolve)));
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    await screen.findByTestId("skill-body");

    await userEvent.type(bodyBox(), "하세요");
    await userEvent.tab();
    await waitFor(() => expect(rpc.write).toHaveBeenCalledTimes(1));

    // A writer does not stop typing because a save is travelling.
    await userEvent.type(bodyBox(), "!");
    await act(async () => {
      confirmSave({ ...full({ body: "안녕하세요" }), versioned: true });
    });

    // The confirmation is only ever allowed to replace the draft it was FOR.
    expect(bodyBox().value).toBe("안녕하세요!");
    await userEvent.tab();
    await waitFor(() => expect(rpc.write).toHaveBeenCalledTimes(2));
    expect(rpc.write).toHaveBeenLastCalledWith(
      expect.objectContaining({ body: "안녕하세요!" }),
    );
  });

  it("tells the writer's own save apart from another window's", async () => {
    rpc.read.mockResolvedValue(full({ body: "안녕" }));
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    await screen.findByTestId("skill-body");

    await userEvent.type(bodyBox(), "하세요");
    await userEvent.tab();
    await waitFor(() => expect(rpc.write).toHaveBeenCalledTimes(1));

    await userEvent.type(bodyBox(), "!");
    await fireChanged({ scope: "writer", name: "dialogue-beats", source: "writer" });
    expect(screen.queryByTestId("skill-changed")).toBeNull();

    // A second writer-sourced event has no save of ours left to account for
    // it, so it is a person in another window editing the same skill.
    await fireChanged({ scope: "writer", name: "dialogue-beats", source: "writer" });
    await screen.findByTestId("skill-changed");
  });

  it("points the textarea at its help line and its running count", async () => {
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    const body = await screen.findByTestId("skill-body");
    const help = screen.getByTestId("skill-body-help");
    const count = screen.getByTestId("skill-body-count");

    const describedBy = (body.getAttribute("aria-describedby") ?? "").split(/\s+/);
    expect(describedBy).toContain(help.id);
    expect(describedBy).toContain(count.id);
    // The count changes while the writer types, so it also has to be
    // announced. <output> is a polite live region natively — the same
    // reasoning as ProviderSection.tsx:820-834.
    expect(count.tagName).toBe("OUTPUT");
  });

  it("creates a skill from a name and a description", async () => {
    rpc.list.mockResolvedValue(listResult([]));
    await mounted();
    expect(screen.getByTestId("skills-empty")).toBeInTheDocument();

    await userEvent.click(screen.getByTestId("skills-new"));
    await userEvent.type(screen.getByTestId("skill-new-name"), "cliffhangers");
    await userEvent.type(screen.getByTestId("skill-new-description"), "회차 끝맺기");
    await userEvent.click(screen.getByTestId("skill-new-submit"));

    await waitFor(() => expect(rpc.write).toHaveBeenCalledTimes(1));
    expect(rpc.write).toHaveBeenCalledWith({
      scope: "writer",
      projectId: "work-1",
      name: "cliffhangers",
      description: "회차 끝맺기",
      body: "",
    });
    // The new skill opens, so the writer can start writing it immediately.
    await waitFor(() => expect(screen.getByTestId("skill-detail-name")).toHaveTextContent("cliffhangers"));
  });

  it("asks before deleting, and deletes only on the second click", async () => {
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    await screen.findByTestId("skill-body");

    await userEvent.click(screen.getByTestId("skill-delete"));
    // A skill is prose the writer or the agent wrote; one stray click must not
    // take it. (The history keeps it, which is what the prompt says.)
    expect(rpc.del).not.toHaveBeenCalled();
    expect(screen.getByTestId("skill-delete-prompt")).toHaveTextContent(
      "settings.skills.delete.confirm",
    );

    await userEvent.click(screen.getByTestId("skill-delete-confirm"));

    await waitFor(() => expect(rpc.del).toHaveBeenCalledWith("writer", "work-1", "dialogue-beats"));
    // The detail closes: there is nothing left to edit.
    await waitFor(() => expect(screen.queryByTestId("skill-body")).toBeNull());
  });

  it("shows the version history and restores a version", async () => {
    rpc.history.mockResolvedValue({
      versions: [
        {
          id: "v2",
          name: "dialogue-beats",
          scope: "writer",
          description: "대사 사이 호흡을 넣는 법",
          author: "agent",
          body: "에이전트가 쓴 두 번째 판",
          body_runes: 12,
          reason: "edited",
          created_at: 1780200000000,
        },
        {
          id: "v1",
          name: "dialogue-beats",
          scope: "writer",
          description: "대사 사이 호흡",
          author: "writer",
          body: "처음 쓴 판",
          body_runes: 6,
          reason: "created",
          created_at: 1780100000000,
        },
      ],
    });
    rpc.restore.mockResolvedValue({ ...full({ body: "처음 쓴 판" }), versioned: true });
    await mounted();
    await userEvent.click(screen.getByTestId("skill-open-writer-dialogue-beats"));
    await screen.findByTestId("skill-body");

    await userEvent.click(screen.getByTestId("skill-history"));

    await waitFor(() => expect(rpc.history).toHaveBeenCalledWith("writer", "work-1", "dialogue-beats"));
    const panel = await screen.findByTestId("skill-history-panel");
    // Who wrote each version is the same question the badge answers in the
    // list, asked of the past.
    expect(within(panel).getByTestId("skill-version-v2")).toHaveTextContent(
      "settings.skills.author.agent",
    );
    // The body travels with the row precisely so the writer can see what they
    // are about to revert to.
    expect(screen.getByTestId("skill-version-preview")).toHaveTextContent("에이전트가 쓴 두 번째 판");

    await userEvent.click(screen.getByTestId("skill-version-v1"));
    expect(screen.getByTestId("skill-version-preview")).toHaveTextContent("처음 쓴 판");

    await userEvent.click(screen.getByTestId("skill-history-restore"));

    await waitFor(() => expect(rpc.restore).toHaveBeenCalledWith("v1"));
    await waitFor(() => expect(bodyBox().value).toBe("처음 쓴 판"));
  });

  it("turns a skill off from the list", async () => {
    await mounted();

    const toggle = screen.getByTestId("skill-enabled-writer-dialogue-beats") as HTMLInputElement;
    expect(toggle.checked).toBe(true);
    await userEvent.click(toggle);

    // One click disables a skill the agent wrote. The body has to be read
    // first — skills.write takes the whole document, and a toggle must not
    // blank a body it never loaded.
    await waitFor(() => expect(rpc.read).toHaveBeenCalledWith("writer", "work-1", "dialogue-beats"));
    await waitFor(() =>
      expect(rpc.write).toHaveBeenCalledWith(
        expect.objectContaining({ name: "dialogue-beats", enabled: false, body: "짧게 끊는다 📖" }),
      ),
    );
  });

  it("says so when the list cannot be read at all", async () => {
    rpc.list.mockRejectedValue(new Error("agentskills: read /home/skills: permission denied"));
    render(<SkillsSection />);

    const error = await screen.findByTestId("skills-error");
    expect(error).toHaveAttribute("role", "alert");
    expect(error.textContent).toContain("permission denied");
  });
});
