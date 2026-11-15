import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RelationshipGraph as Graph } from "../lib/types";

const mocks = vi.hoisted(() => ({ graph: vi.fn() }));
vi.mock("../lib/rpc", () => ({ relationships: { graph: mocks.graph } }));
vi.mock("../hooks/useEngineEvent", () => ({ useEngineEvent: () => {} }));
vi.mock("../lib/i18n", () => ({
  useI18n: () => ({
    language: "ko",
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(",")}` : key,
  }),
  displayNodeLabel: (_: string, label: string) => label,
  entityKindLabel: (_: string, kind: string) => kind,
}));

import { RelationshipGraph } from "./RelationshipGraph";

function graphAt(asOf: number): Graph {
  const changed = asOf === 2;
  return {
    episodes: [1, 2, 3].map((o) => ({ ordinal: o, node_id: `ep${o}`, label: `${o}화` })),
    latest: 2,
    as_of: asOf,
    nodes: [
      { entity_id: "a", kind: "character", name: "하린", appearances: 2, first_episode: 1 },
      { entity_id: "b", kind: "character", name: "유나", appearances: 2, first_episode: 1 },
      { entity_id: "c", kind: "character", name: "서진", appearances: 0 },
      { entity_id: "p", kind: "place", name: "성", appearances: 1, first_episode: 1 },
    ],
    edges: [{
      id: "r2", from_id: "a", to_id: "b",
      label: asOf >= 2 ? "적" : "동료", inverse_label: asOf >= 2 ? "원수" : "동료", changed,
      steps: asOf >= 2
        ? [{ label: "동료", inverse_label: "동료" }, { label: "적", inverse_label: "원수", since_episode: 2, since_node_id: "s2", quote: "넌 적이야." }]
        : [{ label: "동료", inverse_label: "동료" }],
    }],
    co_appearances: [{ from_id: "a", to_id: "b", scenes: 2 }],
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  mocks.graph.mockImplementation((_: string, at: { episode?: number }) => Promise.resolve(graphAt(at.episode ?? 2)));
});

describe("RelationshipGraph", () => {
  it("starts at the open scene's episode and marks the tie that changed there", async () => {
    render(<RelationshipGraph projectId="p1" nodeId="s2" onClose={vi.fn()} />);
    expect(await screen.findByTestId("relationship-graph-episode")).toHaveTextContent("2화");
    expect(mocks.graph).toHaveBeenCalledWith("p1", { nodeId: "s2" });
    // The layout lands a tick after the data: the edges draw once it has.
    const edge = await screen.findByTestId("relationship-graph-edge-r2");
    expect(edge.getAttribute("class")).toContain("is-changed");
    expect(edge.textContent).toBe("◆ 적 / 원수");
    // Places are off by default; 서진 has not appeared yet.
    expect(screen.queryByTestId("relationship-graph-node-p")).toBeNull();
    expect((await screen.findByTestId("relationship-graph-node-c")).getAttribute("class")).toContain("is-pending");
  });

  it("moves through the episodes and keeps everyone where they were", async () => {
    render(<RelationshipGraph projectId="p1" onClose={vi.fn()} />);
    const node = await screen.findByTestId("relationship-graph-node-a");
    await waitFor(() => expect(node.getAttribute("transform")).toMatch(/translate/));
    const before = node.getAttribute("transform");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "relGraph.prev" }));
    });
    expect(mocks.graph).toHaveBeenLastCalledWith("p1", { episode: 1 });
    await waitFor(() => expect(screen.getByTestId("relationship-graph-edge-r2").textContent).toBe("동료"));
    expect(screen.getByTestId("relationship-graph-node-a").getAttribute("transform")).toBe(before);
  });

  it("shows a tie's history and opens the line it changed at", async () => {
    const onRevealPassage = vi.fn();
    const onClose = vi.fn();
    render(<RelationshipGraph projectId="p1" onClose={onClose} onRevealPassage={onRevealPassage} />);
    fireEvent.click(await screen.findByTestId("relationship-graph-edge-r2"));
    const detail = screen.getByTestId("relationship-graph-detail");
    expect(detail.textContent).toContain("relGraph.fromStart동료");
    expect(detail.textContent).toContain("relGraph.since:episode=2화적 / 원수");
    fireEvent.click(screen.getByRole("button", { name: "relGraph.go" }));
    expect(onRevealPassage).toHaveBeenCalledWith({ nodeId: "s2", paragraphFrom: 0, quote: "넌 적이야." });
    expect(onClose).toHaveBeenCalled();
  });

  it("opens a node's sheet on a click that is not a drag", async () => {
    const onOpenEntity = vi.fn();
    render(<RelationshipGraph projectId="p1" onClose={vi.fn()} onOpenEntity={onOpenEntity} />);
    const node = await screen.findByTestId("relationship-graph-node-b");
    fireEvent.pointerDown(node, { pointerId: 1 });
    fireEvent.pointerUp(node, { pointerId: 1 });
    expect(onOpenEntity).toHaveBeenCalledWith("b");
  });

  it("filters to the ties that changed and adds co-appearances on request", async () => {
    render(<RelationshipGraph projectId="p1" onClose={vi.fn()} />);
    await screen.findByTestId("relationship-graph-edge-r2");
    fireEvent.click(screen.getByLabelText("relGraph.coAppearances"));
    expect(document.querySelectorAll(".rg-co")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "relGraph.next" }));
    await waitFor(() => expect(mocks.graph).toHaveBeenLastCalledWith("p1", { episode: 3 }));
    fireEvent.click(screen.getByLabelText("relGraph.changedOnly"));
    await waitFor(() => expect(screen.queryByTestId("relationship-graph-edge-r2")).toBeNull());
  });
});
