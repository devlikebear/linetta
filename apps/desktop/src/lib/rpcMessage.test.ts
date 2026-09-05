import { describe, expect, it } from "vitest";
import { RpcError } from "./rpc";
import { reasonMessage, rpcErrorMessage } from "./rpcMessage";
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

  // The Codex login (#92) has a single reason code: port_in_use. A failed
  // login attempt is not an RPC error at all — codex.login_status reports it
  // via CodexStatus.login_failed instead — so there is no second code here.
  it("covers the Codex port-in-use code", () => {
    for (const reason of ["codex_port_in_use"]) {
      const err = new RpcError("codex.login_start", "raw engine message", -32602, { reason });
      for (const [name, t] of [["ko", ko], ["en", en], ["ja", ja]] as const) {
        const shown = rpcErrorMessage(err, t);
        expect(shown, `${reason} in ${name}`).not.toContain("raw engine message");
        expect(shown.length, `${reason} in ${name}`).toBeGreaterThan(0);
      }
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

  it("translates the built-in agent's reason codes in every language", () => {
    // An unmapped code falls through to the engine's English sentence. For
    // the agent that sentence names internal limits; for its provider
    // neighbours it carries the provider's own response body.
    for (const reason of ["agent_busy", "agent_iteration_limit"]) {
      const error = new RpcError("agent.run", "raw engine sentence", -32602, {
        reason,
      });
      for (const t of [ko, en, ja]) {
        const shown = rpcErrorMessage(error, t);
        expect(shown).not.toContain("raw engine sentence");
        expect(shown).not.toContain(reason);
        expect(shown.length).toBeGreaterThan(0);
      }
    }
  });

  // agent_internal_error matters most: the panic-recovery path in the loop
  // puts the raw Go panic value into the English Message (see
  // engine/internal/agent/loop.go), so an unmapped code would show that
  // panic text to the writer. agent_undo_unavailable was mapped by an
  // earlier fix round but had no direct test; covered here alongside its
  // siblings so all four of the loop's reason codes stay in step with
  // engine/internal/rpc/reason.go.
  it("never shows the raw panic value for an internal agent error, and covers undo too", () => {
    const panicBody = "runtime error: index out of range [3] with length 3";
    for (const reason of ["agent_internal_error", "agent_undo_unavailable"]) {
      const error = new RpcError(
        "agent.run",
        `internal error: ${panicBody}`,
        -32603,
        { reason },
      );
      for (const [name, t] of [["ko", ko], ["en", en], ["ja", ja]] as const) {
        const shown = rpcErrorMessage(error, t);
        expect(shown, `${reason} in ${name}`).not.toContain(panicBody);
        expect(shown, `${reason} in ${name}`).not.toContain(reason);
        expect(shown.length, `${reason} in ${name}`).toBeGreaterThan(0);
      }
    }
  });
});

// A failure that arrives as a notification (agent.error, #95) has a reason
// code and nothing else — no error object, no engine message. There is no
// engine sentence to fall back to, so the fallback has to be its own.
describe("reasonMessage", () => {
  it("translates a mapped code the same as a tagged error would", () => {
    const err = new RpcError("agent.run", "raw engine sentence", -32603, {
      reason: "provider_unreachable",
    });
    for (const t of [ko, en, ja]) {
      expect(reasonMessage("provider_unreachable", t)).toBe(rpcErrorMessage(err, t));
    }
  });

  it("names an unmapped code instead of stringifying the shape it arrived in", () => {
    for (const [name, t] of [["ko", ko], ["en", en], ["ja", ja]] as const) {
      const shown = reasonMessage("not_mapped_yet", t);
      // "[object Object]" is what String({data:{reason}}) produces, and it is
      // what the panel's notice used to render for an unmapped code.
      expect(shown, name).not.toContain("[object Object]");
      expect(shown, name).toContain("not_mapped_yet");
    }
  });
});
