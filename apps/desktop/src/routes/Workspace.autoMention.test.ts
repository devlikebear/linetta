import { describe, expect, it } from "vitest";

import { readSource } from "../test/readSource";

/**
 * The detector existed before this; what was missing was anything calling it
 * without the writer pressing a button (#32). A hook with no mount site is a
 * failure mode this codebase has produced before, so the wiring is asserted
 * rather than assumed.
 */
describe("Workspace auto-mention detection", () => {
  it("counts after the writer stops typing", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    expect(workspace).toContain("countAutoMentionCandidates");
    expect(workspace).toContain("AUTO_MENTION_SCAN_MS");
    // Re-armed on every keystroke, so the count follows the prose.
    expect(workspace).toContain("setAutoMentionScanKey((k) => k + 1)");
    expect(workspace).toContain("autoMentionScanKey]");
  });

  it("re-counts when the buffer is replaced, not just when typed into", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    // `load` is in the effect's deps, so an agent write or a scene change
    // re-runs the scan — the case the issue calls out for set_scene_text.
    const effect = workspace.slice(
      workspace.indexOf("// Re-count after the writer stops typing"),
      workspace.indexOf("const handleAutoMentionScene"),
    );
    expect(effect).toContain("[load, autoMentionScanKey]");
  });

  it("reports the count instead of rewriting the prose", async () => {
    const workspace = await readSource("routes/Workspace.tsx");
    const panel = await readSource("components/ContextPanel.tsx");

    // The scan effect must not touch the document: linking is the writer's
    // decision because a homonym would be linked to the wrong record.
    const effect = workspace.slice(
      workspace.indexOf("// Re-count after the writer stops typing"),
      workspace.indexOf("const handleAutoMentionScene"),
    );
    expect(effect).not.toContain("setContent");
    expect(effect).not.toContain("sceneSaveQueue");

    expect(panel).toContain("workspace.scanSceneFound");
    expect(workspace).toContain("autoMentionFound={autoMentionFound}");
  });

  it("keeps the manual scan as the way to apply", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    expect(workspace).toContain("onAutoMention={handleAutoMentionScene}");
    expect(workspace).toContain("autoMentionDoc(doc, allEntities)");
  });
});
