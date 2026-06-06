import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "..");

async function readSource(path: string) {
  return readFile(resolve(srcRoot, path), "utf8");
}

describe("OutlinePanel CSS", () => {
  it("keeps long doctor issue lists scrollable without hiding repair actions", async () => {
    const css = await readSource("components/OutlinePanel.css");

    expect(css).toContain(".outline-doctor ul");
    expect(css).toContain("max-height: min(34vh, 280px)");
    expect(css).toContain("overflow-y: auto");
  });
});
