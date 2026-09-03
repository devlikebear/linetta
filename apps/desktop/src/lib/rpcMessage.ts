import { useI18n } from "./i18n";
import type { MessageKey } from "./i18n";

type Translate = ReturnType<typeof useI18n>["t"];

/**
 * Reason codes the engine attaches to failures a reader has to understand.
 *
 * The engine sends a stable code and an English sentence; translation happens
 * here, where the ko/en/ja catalogue already lives. Anything without a code
 * falls back to the engine's own message, so an untranslated failure is still
 * visible rather than swallowed.
 *
 * Keep in step with engine/internal/rpc/reason.go.
 */
const REASON_MESSAGE_KEYS: Record<string, MessageKey> = {
  mcp_port_in_use: "errors.mcpPortInUse",
  mcp_consent_required: "errors.mcpConsentRequired",
  // The "not found" family: a record the writer asked for is gone, usually
  // because another window or a connected agent removed it.
  node_not_found: "errors.nodeNotFound",
  project_not_found: "errors.projectNotFound",
  entity_not_found: "errors.entityNotFound",
  thread_not_found: "errors.threadNotFound",
  beat_not_found: "errors.beatNotFound",
  relationship_not_found: "errors.relationshipNotFound",
  note_not_found: "errors.noteNotFound",
  fact_card_not_found: "errors.factCardNotFound",
  // The built-in agent's provider layer (#90/#91). These must stay mapped:
  // the engine's English message for the last three carries the provider's
  // own response body, and the fallback below would print it verbatim.
  provider_not_configured: "errors.providerNotConfigured",
  provider_consent_required: "errors.providerConsentRequired",
  provider_auth_failed: "errors.providerAuthFailed",
  provider_rate_limited: "errors.providerRateLimited",
  provider_unreachable: "errors.providerUnreachable",
  // The Codex login (#92). Its message names the two ports, since the fix is
  // closing whatever holds them. A failed login is not an RPC error at all —
  // codex.login_status reports it via CodexStatus.login_failed instead — so
  // this is the only Codex reason code.
  codex_port_in_use: "errors.codexPortInUse",
};

// Matched on shape rather than `instanceof RpcError`: tests mock ../lib/rpc
// wholesale, which leaves the class undefined and would throw here.
function reasonOf(error: unknown): string | null {
  if (!error || typeof error !== "object") return null;
  const data = (error as { data?: unknown }).data;
  if (!data || typeof data !== "object") return null;
  const reason = (data as { reason?: unknown }).reason;
  return typeof reason === "string" ? reason : null;
}

/** Human-readable text for an error raised by an engine call. */
export function rpcErrorMessage(error: unknown, t: Translate): string {
  const reason = reasonOf(error);
  if (reason) {
    const key = REASON_MESSAGE_KEYS[reason];
    if (key) return t(key);
  }
  return String(error);
}
