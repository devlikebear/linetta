import { localeForLanguage } from "./i18n";
import type { AppLanguage, VaultItemView, VaultOverview, VaultWorkState } from "./types";

export function formatVaultBytes(bytes: number): string {
  if (bytes < 1 << 20) return `${(bytes / (1 << 10)).toFixed(0)} KiB`;
  return `${(bytes / (1 << 20)).toFixed(1)} MiB`;
}

/** PocketBase writes a space where ISO has a T, which not every parser takes. */
export function formatVaultDate(value: string, language: AppLanguage): string {
  const date = new Date(value.replace(" ", "T"));
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString(localeForLanguage(language));
}

/** Whether this device can act on a work at all: anything less goes through the
 *  settings pane, which owns every step of getting there. */
export function vaultReady(overview: VaultOverview): boolean {
  const mode = overview.account?.mode;
  return overview.connected && overview.key_stored && !overview.key_mismatch && mode !== undefined && mode !== "none";
}

/** Works on this device with a newer version on the server. Only ever shown:
 *  pulling replaces the manuscript, so it waits for the writer. */
export function vaultNewVersions(overview: VaultOverview): VaultItemView[] {
  return overview.items.filter((item) => item.project_id !== "" && item.head_revision > item.base_revision);
}

/** The two states that mean the server has something this device does not. */
export function vaultNeedsAttention(state: VaultWorkState | null): boolean {
  return state === "remote_changes" || state === "conflict";
}
