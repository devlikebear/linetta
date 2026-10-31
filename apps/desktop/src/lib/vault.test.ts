import { describe, expect, it, vi } from "vitest";

vi.mock("./i18n", () => ({ localeForLanguage: () => "ko-KR" }));

import { formatVaultBytes, formatVaultDate, vaultNeedsAttention, vaultNewVersions, vaultReady } from "./vault";
import type { VaultAccount, VaultItemView, VaultOverview } from "./types";

const account = { mode: "full" } as VaultAccount;
const overview = (over: Partial<VaultOverview> = {}): VaultOverview => ({
  connected: true,
  key_stored: true,
  key_mismatch: false,
  account,
  items: [],
  ...over,
});
const item = (over: Partial<VaultItemView>): VaultItemView => ({
  item_id: "i",
  project_id: "p",
  title: "t",
  head_revision: 1,
  base_revision: 1,
  updated: "",
  bytes: 0,
  on_this_device: true,
  ...over,
});

describe("vault helpers", () => {
  it("is ready only with a connection, a key that opens things, and access", () => {
    expect(vaultReady(overview())).toBe(true);
    expect(vaultReady(overview({ account: { mode: "read_only" } as VaultAccount }))).toBe(true);
    expect(vaultReady(overview({ connected: false }))).toBe(false);
    expect(vaultReady(overview({ key_stored: false }))).toBe(false);
    expect(vaultReady(overview({ key_mismatch: true }))).toBe(false);
    expect(vaultReady(overview({ account: null }))).toBe(false);
    expect(vaultReady(overview({ account: { mode: "none" } as VaultAccount }))).toBe(false);
  });

  it("counts a new version only for a work on this device", () => {
    const newer = item({ item_id: "a", head_revision: 3, base_revision: 2 });
    const same = item({ item_id: "b", head_revision: 3, base_revision: 3 });
    const elsewhere = item({ item_id: "c", project_id: "", head_revision: 3, base_revision: 0 });
    expect(vaultNewVersions(overview({ items: [newer, same, elsewhere] }))).toEqual([newer]);
  });

  it("flags only what the server has and this device does not", () => {
    expect(vaultNeedsAttention("remote_changes")).toBe(true);
    expect(vaultNeedsAttention("conflict")).toBe(true);
    expect(vaultNeedsAttention("local_changes")).toBe(false);
    expect(vaultNeedsAttention("synced")).toBe(false);
    expect(vaultNeedsAttention(null)).toBe(false);
  });

  it("formats sizes and PocketBase dates, and keeps a date it cannot read", () => {
    expect(formatVaultBytes(50 << 20)).toBe("50.0 MiB");
    expect(formatVaultBytes(2048)).toBe("2 KiB");
    expect(formatVaultDate("2026-11-30 12:00:00.000Z", "ko")).toContain("2026");
    expect(formatVaultDate("soon", "ko")).toBe("soon");
  });
});
