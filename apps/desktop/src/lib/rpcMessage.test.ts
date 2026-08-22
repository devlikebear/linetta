import { describe, expect, it } from "vitest";
import { RpcError } from "./rpc";
import { rpcErrorMessage } from "./rpcMessage";
import { translate } from "./i18n";
import type { MessageKey } from "./i18n";

const ko = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("ko", key, values);
const en = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("en", key, values);

function consentError() {
  return new RpcError(
    "providers.test",
    "AI data sharing consent is required before sending manuscript content to a provider",
    -32603,
    { reason: "ai_data_sharing_consent_required" },
  );
}

describe("rpcErrorMessage", () => {
  it("translates a tagged failure into the reader's language", () => {
    const shown = rpcErrorMessage(consentError(), ko);
    expect(shown).toContain("동의");
    expect(shown).not.toContain("consent is required");
  });

  it("uses the same reason code for every language", () => {
    const shown = rpcErrorMessage(consentError(), en);
    expect(shown).toContain("consent");
    expect(shown).not.toContain("동의");
  });

  it("falls back to the engine message when the failure carries no reason", () => {
    const error = new RpcError("providers.test", "connection refused", -32603);
    expect(rpcErrorMessage(error, ko)).toContain("connection refused");
  });

  it("falls back for an unknown reason code rather than hiding the failure", () => {
    const error = new RpcError("providers.test", "boom", -32603, { reason: "not_mapped_yet" });
    expect(rpcErrorMessage(error, ko)).toContain("boom");
  });

  it("handles errors that are not RpcError at all", () => {
    expect(rpcErrorMessage(new Error("plain failure"), ko)).toContain("plain failure");
  });
});
