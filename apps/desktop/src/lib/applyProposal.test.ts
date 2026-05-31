import { beforeEach, describe, expect, it, vi } from "vitest";
import { applyProposal } from "./applyProposal";

const mocks = vi.hoisted(() => ({
  createThread: vi.fn(),
  updateThread: vi.fn(),
  createBeat: vi.fn(),
  updateProject: vi.fn(),
  remember: vi.fn(),
  createEntity: vi.fn(),
  updateEntity: vi.fn(),
  createPair: vi.fn(),
  createSibling: vi.fn(),
}));

vi.mock("./rpc", () => ({
  threads: {
    create: mocks.createThread,
    update: mocks.updateThread,
  },
  beats: {
    create: mocks.createBeat,
  },
  projects: {
    update: mocks.updateProject,
  },
  companion: {
    remember: mocks.remember,
  },
  entities: {
    create: mocks.createEntity,
    update: mocks.updateEntity,
  },
  relationships: {
    createPair: mocks.createPair,
    createOne: vi.fn(),
  },
  nodes: {
    createSibling: mocks.createSibling,
  },
}));

describe("applyProposal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.createThread.mockResolvedValue({ id: "thread-1" });
    mocks.createEntity.mockResolvedValue({ id: "entity-1" });
    mocks.createSibling.mockResolvedValue({ id: "node-1" });
  });

  it("resolves refs across proposal ops", async () => {
    const result = await applyProposal([
      { op: "create_thread", ref: "main", name: "Main", summary: "Arc" },
      { op: "add_beat", thread_ref: "main", label: "Opening" },
      { op: "create_entity", ref: "hero", kind: "character", name: "Hana", summary: "Lead" },
      { op: "create_relationship", from_ref: "hero", to: "entity-2", label: "친구", inverse_label: "친구" },
    ], "project-1", "scene-1");

    expect(result).toEqual({ applied: 4, failures: [] });
    expect(mocks.createBeat).toHaveBeenCalledWith({
      thread_id: "thread-1",
      node_id: "scene-1",
      label: "Opening",
      description: undefined,
      intensity: undefined,
    });
    expect(mocks.createPair).toHaveBeenCalledWith(expect.objectContaining({
      from_id: "entity-1",
      to_id: "entity-2",
    }));
  });

  it("continues after an invalid op and reports the failure", async () => {
    const result = await applyProposal([
      { op: "add_beat", thread_ref: "missing", label: "Lost" },
      { op: "set_outline", outline: "New outline" },
    ], "project-1", "scene-1");

    expect(result.applied).toBe(1);
    expect(result.failures).toHaveLength(1);
    expect(result.failures[0].error).toContain("스토리라인 참조");
    expect(mocks.updateProject).toHaveBeenCalledWith({ id: "project-1", outline: "New outline" });
  });
});
