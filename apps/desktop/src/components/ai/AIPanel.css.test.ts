import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "../..");

async function readSource(path: string) {
  return readFile(resolve(srcRoot, path), "utf8");
}

describe("AI panel presentation and defaults", () => {
  it("keeps long generated text inside the result card", async () => {
    const css = await readSource("App.css");

    expect(css).toContain("max-height: min(44vh, 420px)");
    expect(css).toContain("overflow-y: auto");
    expect(css).toContain("overflow-wrap: anywhere");
  });

  it("defaults inline AI generation to one-paragraph output", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    expect(workspace).toContain('short_form: true');
  });
});
