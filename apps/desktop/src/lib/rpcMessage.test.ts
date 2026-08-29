import { describe, expect, it } from "vitest";
import { RpcError } from "./rpc";
import { rpcErrorMessage } from "./rpcMessage";
import { translate } from "./i18n";
import type { MessageKey } from "./i18n";

const ko = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("ko", key, values);
const en = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("en", key, values);

function portInUseError() {
  return new RpcError(
    "mcp.enable",
    "mcp port in use: 7391",
    -32602,
    { reason: "mcp_port_in_use" },
  );
}

describe("rpcErrorMessage", () => {
  it("translates a tagged failure into the reader's language", () => {
    const shown = rpcErrorMessage(portInUseError(), ko);
    expect(shown).toContain("포트");
    expect(shown).not.toContain("port in use");
  });

  it("uses the same reason code for every language", () => {
    const shown = rpcErrorMessage(portInUseError(), en);
    expect(shown).toContain("port");
    expect(shown).not.toContain("포트");
  });

  it("falls back to the engine message when the failure carries no reason", () => {
    const error = new RpcError("mcp.enable", "connection refused", -32603);
    expect(rpcErrorMessage(error, ko)).toContain("connection refused");
  });

  it("falls back for an unknown reason code rather than hiding the failure", () => {
    const error = new RpcError("mcp.enable", "boom", -32603, { reason: "not_mapped_yet" });
    expect(rpcErrorMessage(error, ko)).toContain("boom");
  });

  it("handles errors that are not RpcError at all", () => {
    expect(rpcErrorMessage(new Error("plain failure"), ko)).toContain("plain failure");
  });
});
