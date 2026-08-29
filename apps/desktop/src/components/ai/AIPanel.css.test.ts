import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "../..");

async function readSource(path: string) {
  return readFile(resolve(srcRoot, path), "utf8");
}

// The two wiring assertions that lived here checked that inline AI generation
// ran through the companion panel. Phase 6 removed both the modal and the
// panel, so they described code that no longer exists. The remaining case is
// about the result card's own styling, which survives until components/ai is
// deleted with the settings block.
describe("AI panel presentation and defaults", () => {
  it("keeps long generated text inside the result card", async () => {
    const css = await readSource("App.css");

    expect(css).toContain("max-height: min(44vh, 420px)");
    expect(css).toContain("overflow-y: auto");
    expect(css).toContain("overflow-wrap: anywhere");
  });

});
