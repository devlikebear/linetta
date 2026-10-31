import { describe, expect, it } from "vitest";

import { readSource } from "../test/readSource";

/** The dialog's own tests cannot see how Workspace mounts it, and these are the
 *  promises that live in the mounting. */
describe("Workspace vault wiring", () => {
  it("saves the buffer before the dialog opens", async () => {
    const workspace = await readSource("routes/Workspace.tsx");
    expect(workspace).toMatch(/const openVault = useCallback\(async \(\) => \{\s+await flushPendingSave\(\);\s+setVaultOpen\(true\);/);
  });

  it("reloads the whole work after a pull, without saving what is on screen", async () => {
    const workspace = await readSource("routes/Workspace.tsx");
    expect(workspace).toContain("onReopen={() => window.location.replace(`/workspace/${load.project.id}`)}");
  });

  it("only ever reads the work's state on its own", async () => {
    const workspace = await readSource("routes/Workspace.tsx");
    expect(workspace).toContain("vaultApi\n      .workStatus(publishProjectId)");
    expect(workspace).not.toMatch(/vaultApi\s*\.(push|pull|resolveConflict)/);
  });

  it("offers the dialog from the palette only where the vault exists", async () => {
    const workspace = await readSource("routes/Workspace.tsx");
    expect(workspace).toMatch(/if \(vaultAvailable\) \{\s+cmds\.push\(\{\s+id: "vault-work"/);
  });
});
