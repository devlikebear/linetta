import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "..");

async function readSource(path: string) {
  return readFile(resolve(srcRoot, path), "utf8");
}

describe("Settings iOS feature-reduction UX", () => {
  it("shows a disabled git-sync note when gitSyncAvailable is false", async () => {
    const settings = await readSource("routes/Settings.tsx");

    expect(settings).toContain("!gitSyncAvailable && (");
    expect(settings).toContain('t("settings.git.unavailableNote")');
  });

  it("shows an API-key-only note when providers are filtered", async () => {
    const settings = await readSource("routes/Settings.tsx");

    expect(settings).toContain("unavailableProviders.length > 0");
    expect(settings).toContain('t("settings.provider.restrictedNote")');
  });

  it("has the new i18n keys in both ko and en maps", async () => {
    const i18n = await readSource("lib/i18n.tsx");

    // ko map
    const koGitIdx = i18n.indexOf('"settings.git.unavailableNote"');
    const koProvIdx = i18n.indexOf('"settings.provider.restrictedNote"');
    expect(koGitIdx).toBeGreaterThan(-1);
    expect(koProvIdx).toBeGreaterThan(-1);

    // en map must contain a second occurrence after the ko occurrence
    const enGitIdx = i18n.indexOf('"settings.git.unavailableNote"', koGitIdx + 1);
    const enProvIdx = i18n.indexOf('"settings.provider.restrictedNote"', koProvIdx + 1);
    expect(enGitIdx).toBeGreaterThan(-1);
    expect(enProvIdx).toBeGreaterThan(-1);
  });
});
