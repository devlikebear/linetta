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
  ai_data_sharing_consent_required: "errors.aiDataSharingConsentRequired",
  mcp_port_in_use: "errors.mcpPortInUse",
  mcp_consent_required: "errors.mcpConsentRequired",
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
