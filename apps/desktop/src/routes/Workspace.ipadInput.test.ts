import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const read = (p: string) => readFile(resolve(here, "..", p), "utf8");

describe("iPad input affordances", () => {
  it("adds a shortcuts command-bar button on the ipad tier", async () => {
    const ws = await read("routes/Workspace.tsx");
    expect(ws).toContain('sizeClass === "ipad"');
    expect(ws).toContain("setShortcutsOpen(true)");
    expect(ws).toContain("ipad-shortcuts-toggle");
  });

  it("tracks the soft-keyboard inset via visualViewport", async () => {
    const ws = await read("routes/Workspace.tsx");
    expect(ws).toContain("window.visualViewport");
    expect(ws).toContain("--kbd-inset");
  });
});
