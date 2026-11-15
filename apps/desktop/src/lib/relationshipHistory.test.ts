import { describe, expect, it } from "vitest";
import { relationshipChains } from "./relationshipHistory";
import type { Relationship } from "./types";

const row = (id: string, label: string, extra: Partial<Relationship> = {}): Relationship => ({
  id, project_id: "p", from_id: "a", to_id: "b", label, notes: "", ...extra,
});

describe("relationshipChains", () => {
  it("follows each current row back through the rows it took over from", () => {
    const rows = [
      row("r1", "동료", { until_node_id: "s3" }),
      row("r2", "적", { since_node_id: "s3", until_node_id: "s7", supersedes_id: "r1" }),
      row("r3", "연인", { since_node_id: "s7", supersedes_id: "r2", since_quote: "좋아해." }),
      row("x", "스승"),
    ];
    const chains = relationshipChains(rows);
    expect(chains.map((c) => c.steps.map((s) => s.label))).toEqual([["동료", "적", "연인"], ["스승"]]);
    expect(chains[0].current.id).toBe("r3");
  });

  it("leaves out history no current row leads back to", () => {
    expect(relationshipChains([row("old", "동료", { until_node_id: "s3" })])).toEqual([]);
  });
});
