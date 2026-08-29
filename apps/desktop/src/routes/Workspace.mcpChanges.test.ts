import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "..");

async function readSource(path: string) {
  return readFile(resolve(srcRoot, path), "utf8");
}

/**
 * useMcpChanges shipped with five passing behaviour tests and no mount site:
 * the hook was never called from anywhere in the app, so an agent's writes
 * left the outline and the editor showing stale text while only the indicator
 * dot reacted. Unit tests could not catch that — nothing about the hook was
 * wrong. These assertions watch the wiring instead.
 */
describe("Workspace MCP change wiring", () => {
  it("mounts useMcpChanges with the state the guard depends on", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    expect(workspace).toContain('import { useMcpChanges } from "../hooks/useMcpChanges"');
    expect(workspace).toContain("useMcpChanges({");
    expect(workspace).toContain("openNodeId: load?.node.id ?? null");
    // Without this the hook cannot tell a safe refresh from one that would
    // throw away the sentence the writer is in the middle of.
    expect(workspace).toContain("editorDirty,");
    expect(workspace).toContain("onOutlineChanged:");
    expect(workspace).toContain("onSceneChanged:");
  });

  it("keeps the dirty flag honest on both edges", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    // Set while typing, cleared once the engine has the text, and cleared
    // again when a different scene opens. A third clear lived in the companion
    // flush, which went with the companion.
    expect(workspace).toContain("setEditorDirty(true)");
    const clears = workspace.match(/setEditorDirty\(false\)/g) ?? [];
    expect(clears.length).toBeGreaterThanOrEqual(2);
  });

  it("refreshes the outline without touching the open buffer", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    // The structural refresh must not carry node/initialDoc: that is what
    // would replace the editor's contents behind the writer's back.
    const fn = workspace.slice(
      workspace.indexOf("const refreshOutlineFromEngine"),
      workspace.indexOf("const reloadSceneFromEngine"),
    );
    expect(fn).toContain("tree: buildTree(flat)");
    expect(fn).not.toContain("initialDoc");
  });

  it("offers the agent's version rather than applying it", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    expect(workspace).toContain('data-testid="mcp-conflict"');
    expect(workspace).toContain("workspace.mcp.conflict.body");
    expect(workspace).toContain("void reloadSceneFromEngine(conflictNodeId); dismissConflict();");
    expect(workspace).toContain("workspace.mcp.conflict.keep");
  });
});
