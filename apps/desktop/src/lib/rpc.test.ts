import { beforeEach, describe, expect, it, vi } from "vitest";
import { invoke } from "@tauri-apps/api/core";
import { RpcError, rpcCall, skills } from "./rpc";

vi.mock("@tauri-apps/api/core", () => ({ invoke: vi.fn() }));

describe("rpcCall", () => {
  beforeEach(() => {
    vi.mocked(invoke).mockReset();
  });

  it("preserves JSON-RPC code, data, method, and message", async () => {
    vi.mocked(invoke).mockRejectedValue({
      code: -32009,
      message: "node content changed",
      data: { current_content_version: 4 },
      request_id: 17,
    });

    let failure: RpcError | undefined;
    try {
      await rpcCall("nodes.update_content", { id: "scene-1" });
    } catch (error) {
      failure = error as RpcError;
    }

    expect(failure instanceof RpcError).toBe(true);
    expect(failure?.code).toBe(-32009);
    expect(failure?.data).toEqual({ current_content_version: 4 });
    expect(failure?.method).toBe("nodes.update_content");
    expect(failure?.requestId).toBe(17);
    expect(failure?.message).toBe("node content changed");
  });
});

/**
 * A writer-scope skill is global, and the engine hard-refuses a work id on
 * one. The pane always has a selected work in hand, so if the wrapper passed
 * it through, every caller would have to remember to blank it — and one that
 * forgot would get -32602 on a perfectly ordinary save. #97 shipped that
 * `scope === … ? id : ""` by hand at every MemorySection call site; this
 * pins it in the wrapper instead, where nothing downstream can forget it.
 */
describe("the skills wrapper and the work id", () => {
  beforeEach(() => {
    vi.mocked(invoke).mockReset();
    vi.mocked(invoke).mockResolvedValue({} as never);
  });

  const sentWorkId = () => vi.mocked(invoke).mock.calls[0][1] as { params: { project_id: string } };

  it("drops the work id on writer scope", async () => {
    await skills.read("writer", "work-123", "fight-scenes");
    expect(sentWorkId().params.project_id).toBe("");
  });

  it("keeps it on work scope", async () => {
    await skills.read("work", "work-123", "minjun-voice");
    expect(sentWorkId().params.project_id).toBe("work-123");
  });

  it("does the same for write, delete and history", async () => {
    for (const call of [
      () =>
        skills.write({
          scope: "writer" as const,
          projectId: "work-123",
          name: "x",
          description: "d",
          body: "b",
        }),
      () => skills.delete("writer" as const, "work-123", "x"),
      () => skills.history("writer" as const, "work-123", "x"),
    ]) {
      vi.mocked(invoke).mockClear();
      await call();
      expect(sentWorkId().params.project_id).toBe("");
    }
  });
});
