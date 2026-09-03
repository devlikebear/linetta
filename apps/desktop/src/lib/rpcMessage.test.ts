import { describe, expect, it } from "vitest";
import { RpcError } from "./rpc";
import { rpcErrorMessage } from "./rpcMessage";
import { translate } from "./i18n";
import type { MessageKey } from "./i18n";

const ko = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("ko", key, values);
const en = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("en", key, values);
const ja = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("ja", key, values);

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

  // The engine reports a missing record with a code because the message it
  // writes is for logs; the reader gets it in their own language (#44).
  it("explains a record that is gone in the reader's language", () => {
    const gone = new RpcError("nodes.get", "node not found", -32602, {
      reason: "node_not_found",
    });
    expect(rpcErrorMessage(gone, ko)).toContain("씬");
    expect(rpcErrorMessage(gone, en)).toContain("scene");
    expect(rpcErrorMessage(gone, en)).not.toContain("node not found");
  });

  it("covers every not-found code the engine can send", () => {
    for (const reason of [
      "node_not_found",
      "project_not_found",
      "entity_not_found",
      "thread_not_found",
      "beat_not_found",
      "relationship_not_found",
      "note_not_found",
      "fact_card_not_found",
    ]) {
      const err = new RpcError("x", "raw engine message", -32602, { reason });
      expect(rpcErrorMessage(err, en), reason).not.toContain("raw engine message");
    }
  });

  // The engine's English message for a provider failure carries up to 200
  // bytes of the provider's own response body. If a code is unmapped the
  // fallback prints that body verbatim as UI text, which is exactly what the
  // reason codes exist to prevent — so every one of them must translate.
  it("never shows the provider's own response body to the reader", () => {
    const body = '{"error":{"message":"Incorrect API key provided: sk-abc123"}}';
    for (const reason of [
      "provider_not_configured",
      "provider_consent_required",
      "provider_auth_failed",
      "provider_rate_limited",
      "provider_unreachable",
    ]) {
      const err = new RpcError("providers.test", `anthropic: ${body}`, -32602, { reason });
      for (const [name, t] of [["ko", ko], ["en", en], ["ja", ja]] as const) {
        const shown = rpcErrorMessage(err, t);
        expect(shown, `${reason} in ${name}`).not.toContain("sk-abc123");
        expect(shown, `${reason} in ${name}`).not.toContain("RpcError");
        expect(shown.length, `${reason} in ${name}`).toBeGreaterThan(0);
      }
    }
  });
});
