import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));

async function readWorkspace() {
  return readFile(resolve(here, "Workspace.tsx"), "utf8");
}

describe("Workspace history panel placement", () => {
  it("renders scene history inside the right side panel branch", async () => {
    const workspace = await readWorkspace();

    expect(workspace).toContain("{versionSheetNodeId && load ? (");
    expect(workspace).not.toContain("{versionSheetNodeId && (\n        <VersionSheet");
  });
});
