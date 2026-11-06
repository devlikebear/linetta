import { beforeEach, describe, expect, it, vi } from "vitest";
import { invoke } from "@tauri-apps/api/core";
import { RpcError, posts, rpcCall, voice } from "./rpc";

describe("rpcCall", () => {
  beforeEach(() => {
    vi.mocked(invoke).mockReset();
  });

  it("calls engine_call with the method and params", async () => {
    vi.mocked(invoke).mockResolvedValue({ ok: true });
    await rpcCall("posts.counts");
    expect(invoke).toHaveBeenCalledWith("engine_call", { method: "posts.counts", params: null });
  });

  it("preserves JSON-RPC code, data, method, and message", async () => {
    vi.mocked(invoke).mockRejectedValue({
      code: -32602,
      message: "content is required",
      data: { field: "content" },
      request_id: 3,
    });
    let failure: RpcError | undefined;
    try {
      await rpcCall("posts.import", {});
    } catch (error) {
      failure = error as RpcError;
    }
    expect(failure instanceof RpcError).toBe(true);
    expect(failure?.code).toBe(-32602);
    expect(failure?.data).toEqual({ field: "content" });
    expect(failure?.method).toBe("posts.import");
    expect(failure?.requestId).toBe(3);
    expect(failure?.message).toBe("content is required");
  });

  it("wraps a plain string failure", async () => {
    vi.mocked(invoke).mockRejectedValue("engine unavailable");
    await expect(rpcCall("ping")).rejects.toMatchObject({ message: "engine unavailable", method: "ping" });
  });

  it("normalises null lists from the engine", async () => {
    vi.mocked(invoke).mockResolvedValue(null);
    expect(await posts.list()).toEqual([]);
    expect(await posts.counts()).toEqual([]);
  });

  it("sends voice.profile params", async () => {
    vi.mocked(invoke).mockResolvedValue({});
    await voice.profile({ platform: "x", kind: "reply" });
    expect(invoke).toHaveBeenCalledWith("engine_call", {
      method: "voice.profile",
      params: { platform: "x", kind: "reply" },
    });
  });
});
