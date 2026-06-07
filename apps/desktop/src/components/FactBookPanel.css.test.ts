import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "..");

async function readSource(path: string) {
  return readFile(resolve(srcRoot, path), "utf8");
}

describe("FactBookPanel CSS", () => {
  it("keeps long companion URLs and tool payloads inside the panel", async () => {
    const css = await readSource("components/FactBookPanel.css");

    expect(css).toContain(".fact-companion-box");
    expect(css).toContain("overflow: hidden");
    expect(css).toContain(".fact-companion-prose .cmp-md a");
    expect(css).toContain("overflow-wrap: anywhere");
    expect(css).toContain("word-break: break-word");
  });
});
