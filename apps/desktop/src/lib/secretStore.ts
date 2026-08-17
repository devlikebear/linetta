import type { MessageKey } from "./i18n";

/**
 * Where the engine keeps an API key on this platform.
 *
 * The engine picks a backend at compile time in
 * engine/internal/settings/defaultSecretStore():
 *   - macOS   → Keychain (secrets_darwin.go)
 *   - Windows → Credential Manager (secrets_windows.go)
 *   - else    → secrets_other.go, an unsupported store whose Set() errors
 *
 * Keep this in step with that function. Linux still has no backend, so a key
 * genuinely cannot be saved there and the UI must not claim otherwise; when a
 * libsecret backend lands, add it here and drop the unsupported copy.
 */
export type KeyStoreKind = "macos" | "windows" | "none";

export function keyStoreKind(): KeyStoreKind {
  const platform = navigator.platform.toLowerCase();
  if (platform.includes("mac")) return "macos";
  if (platform.includes("win")) return "windows";
  return "none";
}

export function hasSecureKeyStore(): boolean {
  return keyStoreKind() !== "none";
}

const STORE_LABEL_KEYS: Record<Exclude<KeyStoreKind, "none">, MessageKey> = {
  macos: "settings.keyStore.macos",
  windows: "settings.keyStore.windows",
};

/**
 * Message key naming the store, for interpolating into `{store}`. Returns null
 * on platforms with no store, where the caller must use the *Unsupported copy
 * instead of naming a place the key does not go.
 */
export function keyStoreLabelKey(): MessageKey | null {
  const kind = keyStoreKind();
  return kind === "none" ? null : STORE_LABEL_KEYS[kind];
}
