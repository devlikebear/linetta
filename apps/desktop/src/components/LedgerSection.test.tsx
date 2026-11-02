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
  relinkWork: vi.fn(),
  duplicate: vi.fn(),
}));

vi.mock("@tauri-apps/api/event", () => ({ listen: () => Promise.resolve(() => undefined) }));
vi.mock("../lib/rpc", () => ({
  settings: { get: vi.fn().mockResolvedValue({ language: "en" }) },
  ledger: { list: mocks.list, save: mocks.save, remove: mocks.remove, setArc: mocks.setArc, setPinned: mocks.setPinned },
  mentions: { relinkWork: mocks.relinkWork },
  projects: { duplicate: mocks.duplicate },
}));

const entry = (over = {}) => ({
  id: "l1", project_id: "p", kind: "promise", subject: "명패", body: "금박 밑의 비밀", status: "open",
  planted_node_id: "n1", source: "agent", created_at: 1, updated_at: 1, ...over,
});

async function renderSection() {
  render(<I18nProvider><LedgerSection projectId="p" /></I18nProvider>);
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
});
