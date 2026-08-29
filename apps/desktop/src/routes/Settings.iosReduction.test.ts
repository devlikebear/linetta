import { describe, expect, it } from "vitest";

import { readSource } from "../test/readSource";

// The provider-restriction note went with the provider chooser: a build that
// cannot reach some providers no longer has a list to filter.
describe("Settings iOS feature-reduction UX", () => {
  it("shows a disabled git-sync note when gitSyncAvailable is false", async () => {
    const settings = await readSource("routes/Settings.tsx");

    expect(settings).toContain("!gitSyncAvailable && (");
    expect(settings).toContain('t("settings.git.unavailableNote")');
  });

  it("has the new i18n keys in both ko and en maps", async () => {
    const i18n = await readSource("lib/i18n.tsx");

    // ko map
    const koGitIdx = i18n.indexOf('"settings.git.unavailableNote"');
    expect(koGitIdx).toBeGreaterThan(-1);

    // en map must contain a second occurrence after the ko occurrence
    const enGitIdx = i18n.indexOf('"settings.git.unavailableNote"', koGitIdx + 1);
    expect(enGitIdx).toBeGreaterThan(-1);
  });
});
