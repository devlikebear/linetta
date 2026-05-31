import { beforeEach, describe, expect, it, vi } from "vitest";
import { applyProposal } from "./applyProposal";

const mocks = vi.hoisted(() => ({
  applyOps: vi.fn(),
}));

vi.mock("./rpc", () => ({
  companion: {
    applyOps: mocks.applyOps,
  },
}));

describe("applyProposal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.applyOps.mockResolvedValue({ applied: 4, failures: [] });
  });

  it("delegates selected ops to the engine-side companion.applyOps RPC", async () => {
    const ops = [
      { op: "create_thread", ref: "main", name: "Main", summary: "Arc" },
      { op: "add_beat", thread_ref: "main", label: "Opening" },
      { op: "create_entity", ref: "hero", kind: "character", name: "Hana", summary: "Lead" },
      { op: "create_relationship", from_ref: "hero", to: "entity-2", label: "친구", inverse_label: "친구" },
    ] as const;

    const result = await applyProposal([...ops], "project-1", "scene-1");

    expect(mocks.applyOps).toHaveBeenCalledWith("project-1", "scene-1", "", ops);
    expect(result).toEqual({ applied: 4, failures: [] });
  });

  it("maps engine failure indexes back to the original ops for the proposal card", async () => {
    mocks.applyOps.mockResolvedValue({
      applied: 1,
      failures: [{ index: 0, op: "add_beat", error: "스토리라인 참조를 해소할 수 없음" }],
    });
    const ops = [
      { op: "add_beat", thread_ref: "missing", label: "Lost" },
      { op: "set_outline", outline: "New outline" },
    ] as const;

    const result = await applyProposal([
      { op: "add_beat", thread_ref: "missing", label: "Lost" },
      { op: "set_outline", outline: "New outline" },
    ], "project-1", "scene-1");

    expect(result.applied).toBe(1);
    expect(result.failures).toHaveLength(1);
    expect(result.failures[0].op).toEqual(ops[0]);
    expect(result.failures[0].error).toContain("스토리라인 참조");
  });
});
