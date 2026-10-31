import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { LedgerSection } from "./LedgerSection";
import { I18nProvider } from "../lib/i18n";

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  save: vi.fn(),
  remove: vi.fn(),
  setArc: vi.fn(),
}));

vi.mock("@tauri-apps/api/event", () => ({ listen: () => Promise.resolve(() => undefined) }));
vi.mock("../lib/rpc", () => ({
  settings: { get: vi.fn().mockResolvedValue({ language: "en" }) },
  ledger: { list: mocks.list, save: mocks.save, remove: mocks.remove, setArc: mocks.setArc },
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
