import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { LedgerSection } from "./LedgerSection";
import { I18nProvider } from "../lib/i18n";

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  save: vi.fn(),
  remove: vi.fn(),
  setArc: vi.fn(),
  setPinned: vi.fn(),
  knowsAt: vi.fn(),
  storyTime: vi.fn(),
  relinkWork: vi.fn(),
  duplicate: vi.fn(),
}));

vi.mock("@tauri-apps/api/event", () => ({ listen: () => Promise.resolve(() => undefined) }));
vi.mock("../lib/rpc", () => ({
  settings: { get: vi.fn().mockResolvedValue({ language: "en" }) },
  ledger: { list: mocks.list, save: mocks.save, remove: mocks.remove, setArc: mocks.setArc, setPinned: mocks.setPinned, knowsAt: mocks.knowsAt, storyTime: mocks.storyTime },
  mentions: { relinkWork: mocks.relinkWork },
  projects: { duplicate: mocks.duplicate },
}));

const entry = (over = {}) => ({
  id: "l1", project_id: "p", kind: "promise", subject: "명패", body: "금박 밑의 비밀", status: "open",
  planted_node_id: "n1", source: "agent", created_at: 1, updated_at: 1, ...over,
});

async function renderSection(nodeId?: string) {
  render(<I18nProvider><LedgerSection projectId="p" nodeId={nodeId} /></I18nProvider>);
  await waitFor(() => expect(mocks.list).toHaveBeenCalled());
}

describe("LedgerSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.list.mockResolvedValue({
      entries: [entry(), entry({ id: "l2", kind: "decision", body: "서린은 남자" }), entry({ id: "l3", kind: "cut_plan", body: "지운 화" })],
      parts: [{ node_id: "part", label: "1부", role: "기", goal: "기연", tension: 3 }],
      labels: { n1: "3화" },
    });
    mocks.save.mockResolvedValue({});
    mocks.setArc.mockResolvedValue({});
    mocks.setPinned.mockResolvedValue({});
  });

  it("pins a settled entry through its own RPC", async () => {
    await renderSection();
    fireEvent.click(await screen.findByRole("tab", { name: "Settled" }));
    fireEvent.click(await screen.findByRole("button", { name: "Pin" }));
    await waitFor(() => expect(mocks.setPinned).toHaveBeenCalledWith("l2", true));
    expect(mocks.save).not.toHaveBeenCalled();
  });

  it("shows a fact's attribute, span and standing", async () => {
    mocks.list.mockResolvedValue({
      entries: [entry({ id: "l4", kind: "number", attribute: "위치", strength: "soft", valid_to_node_id: "n1", superseded_by: "l9" })],
      parts: [],
      labels: { n1: "3화" },
    });
    await renderSection();
    fireEvent.click(await screen.findByRole("tab", { name: "Settled" }));
    expect(await screen.findByText("위치")).toBeTruthy();
    expect(screen.getByText("unconfirmed")).toBeTruthy();
    expect(screen.getByText("replaced")).toBeTruthy();
    expect(screen.getByText("until 3화")).toBeTruthy();
  });

  it("sets how far a promise reaches", async () => {
    await renderSection();
    const reach = (await screen.findAllByLabelText("Reach"))[0];
    fireEvent.change(reach, { target: { value: "work" } });
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith({ id: "l1", reach: "work" }));
  });

  it("rebuilds the links and duplicates the work on request", async () => {
    mocks.relinkWork.mockResolvedValue({ scenes: 2, links: 5 });
    mocks.duplicate.mockResolvedValue({ project_id: "copy" });
    await renderSection();
    fireEvent.click(await screen.findByRole("button", { name: "Rebuild the appearance index" }));
    expect((await screen.findByRole("status")).textContent).toContain("5");
    fireEvent.click(screen.getByRole("button", { name: "Duplicate this work" }));
    await waitFor(() => expect(mocks.duplicate).toHaveBeenCalledWith("p", " (copy)"));
  });

  it("offers no pin on what the story still owes", async () => {
    await renderSection();
    await screen.findByText("금박 밑의 비밀");
    expect(screen.queryByRole("button", { name: "Pin" })).toBeNull();
  });

  it("puts pinned entries first and warns when they outgrow the brief", async () => {
    mocks.list.mockResolvedValue({
      entries: [
        entry({ id: "l2", kind: "decision", body: "둘째" }),
        entry({ id: "l3", kind: "decision", body: "가".repeat(3100), pinned: true }),
      ],
      parts: [],
      labels: {},
    });
    await renderSection();
    fireEvent.click(await screen.findByRole("tab", { name: "Settled" }));
    const items = await screen.findAllByRole("listitem");
    expect(items[0].className).toContain("is-pinned");
    expect(screen.getByRole("status").textContent).toMatch(/brief/i);
  });

  it("shows what is owed first and keeps settled matters and cut plans on their own tabs", async () => {
    await renderSection();
    expect(await screen.findByText("금박 밑의 비밀")).toBeTruthy();
    expect(screen.queryByText("서린은 남자")).toBeNull();
    fireEvent.click(screen.getAllByRole("tab")[1]);
    expect(await screen.findByText("서린은 남자")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("tab")[3]);
    expect(await screen.findByText("지운 화")).toBeTruthy();
  });

  it("adds what the writer typed", async () => {
    await renderSection();
    await screen.findByText("금박 밑의 비밀");
    const box = document.querySelector(".ledger-add input") as HTMLInputElement;
    fireEvent.change(box, { target: { value: "새 복선" } });
    fireEvent.click(document.querySelector(".ledger-add button") as HTMLElement);
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith({ project_id: "p", kind: "promise", body: "새 복선" }));
  });

  it("saves a part's arc when its field loses focus, and only if it changed", async () => {
    await renderSection();
    const role = await screen.findByDisplayValue("기");
    fireEvent.blur(role);
    expect(mocks.setArc).not.toHaveBeenCalled();
    fireEvent.change(role, { target: { value: "승" } });
    fireEvent.blur(role);
    await waitFor(() => expect(mocks.setArc).toHaveBeenCalledWith(expect.objectContaining({ node_id: "part", role: "승", tension: 3 })));
  });

  describe("secrets and knowledge (longform Phase 7)", () => {
    const question = entry({ id: "q1", kind: "question", subject: "", body: "하람은 누구인가", answer: "사라진 왕자" });

    it("keeps a question's answer out of the page until the writer unfolds it, and saves an edit", async () => {
      mocks.list.mockResolvedValue({ entries: [question], parts: [], labels: {} });
      await renderSection();
      await screen.findByText("하람은 누구인가");
      expect(screen.getByText("answered")).toBeTruthy();
      expect(screen.queryByDisplayValue("사라진 왕자")).toBeNull();
      expect(document.body.textContent).not.toContain("사라진 왕자");

      const toggle = screen.getByRole("button", { name: "Show answer (only you know)" });
      expect(toggle.getAttribute("aria-expanded")).toBe("false");
      fireEvent.click(toggle);
      const box = screen.getByLabelText("Answer (only you know)");
      expect((box as HTMLTextAreaElement).value).toBe("사라진 왕자");
      fireEvent.blur(box);
      expect(mocks.save).not.toHaveBeenCalled();
      fireEvent.change(box, { target: { value: "  쌍둥이 왕자 " } });
      fireEvent.blur(box);
      await waitFor(() => expect(mocks.save).toHaveBeenCalledWith({ id: "q1", answer: "쌍둥이 왕자" }));

      fireEvent.click(screen.getByRole("button", { name: "Hide answer" }));
      expect(screen.queryByLabelText("Answer (only you know)")).toBeNull();
    });

    it("pins an open question through the same RPC as a settled fact", async () => {
      mocks.list.mockResolvedValue({ entries: [question, entry()], parts: [], labels: {} });
      await renderSection();
      await screen.findByText("하람은 누구인가");
      const pins = screen.getAllByRole("button", { name: "Pin" });
      expect(pins).toHaveLength(1);
      fireEvent.click(pins[0]);
      await waitFor(() => expect(mocks.setPinned).toHaveBeenCalledWith("q1", true));
      expect(screen.getByText(/no agent can change their answers/)).toBeTruthy();
    });

    it("offers no answer field on anything but a question", async () => {
      await renderSection();
      await screen.findByText("금박 밑의 비밀");
      expect(screen.queryByRole("button", { name: "Show answer (only you know)" })).toBeNull();
    });

    it("reads who knows what at the scene open in the editor, reader first", async () => {
      mocks.list.mockResolvedValue({ entries: [question], parts: [], labels: {} });
      mocks.knowsAt.mockResolvedValue({
        scenes: [{ node_id: "n1", label: "씬 1", parent_label: "1화" }, { node_id: "n2", label: "씬 1", parent_label: "7화" }],
        node_id: "n1",
        secrets: [{ id: "q1", body: "하람은 누구인가", paid_node_id: "n2" }, { id: "q2", subject: "명패", body: "누가 새겼나" }],
        rows: [
          { subject: "하람", entity_id: "e1", cells: { q1: { knows: true, node_id: "n1" }, q2: { knows: false } } },
          { subject: "서린", entity_id: "e2", cells: {} },
        ],
      });
      await renderSection("n1");
      await screen.findByText("하람은 누구인가");
      expect(mocks.knowsAt).not.toHaveBeenCalled();

      const details = document.querySelector(".knows-table") as HTMLDetailsElement;
      details.open = true;
      fireEvent(details, new Event("toggle"));
      await waitFor(() => expect(mocks.knowsAt).toHaveBeenCalledWith("p", "n1"));
      const rows = await screen.findAllByRole("row");
      expect(rows.map((r) => r.querySelector("th")?.textContent)).toEqual(["Who", "Reader", "하람", "서린"]);
      expect(rows[1].textContent).toContain("told in 7화 · 씬 1");
      expect([...rows[2].querySelectorAll("td")].map((c) => c.textContent)).toEqual(["knows", "doesn't know"]);
      expect([...rows[3].querySelectorAll("td")].map((c) => c.textContent)).toEqual(["no record", "no record"]);
      expect(document.body.textContent).not.toContain("사라진 왕자");

      fireEvent.change(screen.getByLabelText("Scene"), { target: { value: "n2" } });
      await waitFor(() => expect(mocks.knowsAt).toHaveBeenCalledWith("p", "n2"));
    });
  });
  describe("story time (longform Phase 8)", () => {
    const event = entry({ id: "ev", kind: "timeline", subject: "", body: "등대지기 실종", planted_node_id: "n1", value_num: 10 });
    const count = entry({ id: "el", kind: "timeline", subject: "등대지기 실종", attribute: "경과", body: "석 달", about: "ev" });

    it("adds an elapsed count about an event, in days, at the open scene", async () => {
      mocks.list.mockResolvedValue({ entries: [event, count, entry({ id: "d", kind: "timeline", attribute: "작중 날짜", body: "봄" })], parts: [], labels: { n1: "1화", n3: "3화" } });
      await renderSection("n3");
      fireEvent.change(await screen.findByLabelText("Kind"), { target: { value: "timeline" } });
      fireEvent.change(screen.getByLabelText("Attribute"), { target: { value: "경과" } });
      const picker = screen.getByLabelText("Event") as HTMLSelectElement;
      // Events only: an elapsed count is never something to count from.
      expect([...picker.options].map((o) => o.value)).toEqual(["", "ev", "d"]);
      expect(screen.getByText(/A month is 30 days, a year 365/).textContent).toContain("Recorded in 3화.");
      fireEvent.change(screen.getByLabelText("Entry"), { target: { value: "석 달" } });
      fireEvent.change(screen.getByLabelText("Days"), { target: { value: "90" } });
      const add = screen.getByRole("button", { name: "Add" });
      expect(add).toBeDisabled();
      fireEvent.change(picker, { target: { value: "ev" } });
      fireEvent.click(add);
      await waitFor(() => expect(mocks.save).toHaveBeenCalledWith({
        project_id: "p", kind: "timeline", body: "석 달", attribute: "경과", planted_node_id: "n3", value_num: 90,
        about: "ev", subject: "등대지기 실종",
      }));
    });

    it("keeps a plain entry plain", async () => {
      await renderSection("n3");
      fireEvent.change(await screen.findByLabelText("Kind"), { target: { value: "timeline" } });
      fireEvent.change(screen.getByLabelText("Entry"), { target: { value: "등대지기 실종" } });
      fireEvent.click(screen.getByRole("button", { name: "Add" }));
      await waitFor(() => expect(mocks.save).toHaveBeenCalledWith({ project_id: "p", kind: "timeline", body: "등대지기 실종" }));
    });

    it("lays out each event by scene, marking a conflict, a jump and a flashback scene", async () => {
      mocks.list.mockResolvedValue({ entries: [event], parts: [], labels: {} });
      const fact = (id: string, node: string, body: string, days?: number) =>
        entry({ id, kind: "timeline", subject: "등대지기 실종", attribute: "경과", body, about: "ev", planted_node_id: node, value_num: days });
      mocks.storyTime.mockResolvedValue({
        scenes: [{ node_id: "n1", label: "1화" }, { node_id: "n2", label: "2화" }, { node_id: "n3", label: "3화" },
          { node_id: "n5", label: "5화", flashback: true }, { node_id: "n7", label: "7화" }],
        clocks: { n3: 100 },
        events: [{
          event, subject: "등대지기 실종",
          facts: [
            { entry: fact("a", "n3", "석 달", 90), conflicts: [] },
            { entry: fact("b", "n5", "사흘", 3), conflicts: [] },
            { entry: fact("c", "n7", "삼 년", 1095), conflicts: [], previous: { entry_id: "a", value_num: 90, delta: 1005, jump_days: 1005, scenes_between: 4 } },
          ],
        }],
        ages: [{
          subject: "하람",
          facts: [
            { entry: entry({ id: "g1", kind: "decision", subject: "하람", attribute: "나이", body: "열일곱", planted_node_id: "n1", value_num: 17 }), conflicts: [] },
            { entry: entry({ id: "g2", kind: "decision", subject: "하람", attribute: "나이", body: "열다섯", planted_node_id: "n3", value_num: 15 }),
              conflicts: [{ entry_id: "g1", body: "열일곱", reason: "하람는 1화에서 이미 17살이었는데 3화에서는 15살입니다" }] },
          ],
        }],
      });
      await renderSection();
      await screen.findByText("Story time");
      expect(mocks.storyTime).not.toHaveBeenCalled();
      const details = document.querySelector(".time-table") as HTMLDetailsElement;
      details.open = true;
      fireEvent(details, new Event("toggle"));
      await waitFor(() => expect(mocks.storyTime).toHaveBeenCalledWith("p"));

      const times = await waitFor(() => {
        const table = details.querySelector("table");
        expect(table?.querySelectorAll("tbody tr").length).toBe(4);
        return table as HTMLTableElement;
      });
      // 2화 recorded nothing and is left out; 5화 is marked a flashback.
      expect([...times.querySelectorAll("thead th")].map((th) => th.textContent)).toEqual(["Event", "1화", "3화", "5화flashback", "7화"]);
      const eventRow = [...times.querySelectorAll("tbody tr")].find((r) => r.querySelector("th")?.textContent?.startsWith("등대지기 실종"));
      expect(eventRow?.querySelector("th")?.textContent).toContain("on day 10");
      const cells = [...(eventRow?.querySelectorAll("td") ?? [])];
      expect(cells.map((c) => c.getAttribute("data-time"))).toEqual(["none", "ok", "ok", "jump"]);
      expect(cells[3].textContent).toContain("+1,005 days");
      const rows = [...times.querySelectorAll("tbody tr")];
      // The clock row, the event, the ages heading, then each person's ages.
      expect(rows.map((r) => r.querySelector("th")?.textContent)).toEqual(["Story date", "등대지기 실종on day 10", "Age", "하람"]);
      const ageCells = [...rows[3].querySelectorAll("td")];
      expect(ageCells.map((c) => c.getAttribute("data-time"))).toEqual(["ok", "conflict", "none", "none"]);
      expect(ageCells[1].textContent).toContain("이미 17살");
    });
  });
});
