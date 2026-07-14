import { beforeEach, describe, expect, it, vi } from "vitest";
import { invoke } from "@tauri-apps/api/core";
import { RpcError, rpcCall } from "./rpc";

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
