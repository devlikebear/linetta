import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { LedgerTimeline } from "../lib/types";

const mocks = vi.hoisted(() => ({ timeline: vi.fn() }));

vi.mock("../lib/rpc", () => ({ ledger: { timeline: mocks.timeline } }));
vi.mock("../hooks/useEngineEvent", () => ({ useEngineEvent: () => {} }));
vi.mock("../lib/i18n", () => ({
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(",")}` : key,
  }),
  displayNodeLabel: (_l: string, label: string) => label,
}));

import { ForeshadowMap } from "./ForeshadowMap";

const TIMELINE: LedgerTimeline = {
  episodes: [1, 2, 3, 4].map((n) => ({ ordinal: n, node_id: `e${n}`, label: `${n}화` })),
  parts: [
    { node_id: "v1", label: "1권", from: 1, to: 2 },
    { node_id: "v2", label: "2권", from: 3, to: 4 },
  ],
  latest: 3,
  current: 3,
  entries: [
    { id: "key", kind: "promise", subject: "은빛 열쇠", body: "열쇠의 주인", status: "paid",
      planted: { episode: 1, node_id: "s1", quote: "열쇠가 빛났다" }, due: { episode: 3, node_id: "e3" },
      paid: { episode: 3, node_id: "s3", quote: "문이 열렸다" } },
    { id: "late", kind: "question", subject: "스승의 정체", body: "누구인가", status: "open",
      planted: { episode: 1, node_id: "s1" }, due: { episode: 2, node_id: "e2" }, overdue: true, open_for: 2 },
    { id: "gone", kind: "promise", subject: "첫 편지", body: "편지", status: "dropped",
      planted: { episode: 2, node_id: "s2" } },
    { id: "gag", kind: "gag", subject: "쌍둥이 소문", body: "소문", status: "open",
      planted: { episode: 3, node_id: "s3" }, open_for: 0 },
  ],
};

function rows() {
  return screen.getAllByTestId(/^foreshadow-row-/).map((el) => el.getAttribute("data-testid")?.slice("foreshadow-row-".length));
}

async function renderMap(props: Partial<Parameters<typeof ForeshadowMap>[0]> = {}) {
  const onClose = vi.fn();
  const onRevealPassage = vi.fn();
  const onOpenNode = vi.fn();
  render(<ForeshadowMap projectId="p1" nodeId="s3" onClose={onClose} onRevealPassage={onRevealPassage} onOpenNode={onOpenNode} {...props} />);
  await screen.findByTestId("foreshadow-chart");
  return { onClose, onRevealPassage, onOpenNode };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.timeline.mockResolvedValue(TIMELINE);
});

describe("ForeshadowMap", () => {
  it("asks for the open scene's episode and counts every status", async () => {
    await renderMap();
    expect(mocks.timeline).toHaveBeenCalledWith("p1", "s3");
    expect(screen.getByTestId("foreshadow-counts").textContent).toBe("foreshadow.counts:open=2,paid=1,dropped=1,overdue=1");
    expect(screen.getByTestId("foreshadow-current")).toBeTruthy();
  });

  it("puts what is overdue first and hides dropped entries until asked", async () => {
    await renderMap();
    expect(rows()).toEqual(["late", "gag", "key"]);
    fireEvent.click(screen.getByTestId("foreshadow-status-dropped"));
    expect(rows()).toContain("gone");
    fireEvent.click(screen.getByTestId("foreshadow-kind-promise"));
    expect(rows()).toEqual(["late", "gag"]);
  });

  it("narrows the axis to one part", async () => {
    await renderMap();
    fireEvent.change(screen.getByTestId("foreshadow-part"), { target: { value: "v2" } });
    // Planted in 1권 but still open: it reaches into 2권, so it stays.
    expect(rows()).toEqual(["late", "gag", "key"]);
    fireEvent.change(screen.getByTestId("foreshadow-part"), { target: { value: "v1" } });
    expect(rows()).toEqual(["late", "key"]);
  });

  it("opens a quoted point at its quote, and a due episode as a scene", async () => {
    const { onClose, onRevealPassage, onOpenNode } = await renderMap();
    await act(async () => {
      fireEvent.click(screen.getByTestId("foreshadow-paid-key"));
    });
    expect(onRevealPassage).toHaveBeenCalledWith({ nodeId: "s3", paragraphFrom: 0, quote: "문이 열렸다" });
    expect(onClose).toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("foreshadow-due-late"));
    expect(onOpenNode).toHaveBeenCalledWith("e2");
  });

  it("shows a row's details when its name is picked", async () => {
    await renderMap();
    fireEvent.click(screen.getByTestId("foreshadow-row-late"));
    const detail = screen.getByTestId("foreshadow-detail");
    expect(detail.textContent).toContain("누구인가");
    expect(detail.textContent).toContain("foreshadow.openFor:n=2");
  });

  it("says when the ledger has nothing owed", async () => {
    mocks.timeline.mockResolvedValue({ ...TIMELINE, entries: [] });
    render(<ForeshadowMap projectId="p1" onClose={vi.fn()} />);
    expect((await screen.findByTestId("foreshadow-empty")).textContent).toBe("foreshadow.empty");
  });
});
