import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "../../../..");

async function readRepo(path: string) {
  return readFile(resolve(repoRoot, path), "utf8");
}

describe("iOS dev simulator launcher", () => {
  it("waits for the target simulator to boot before tauri installs the app", async () => {
    const makefile = await readRepo("Makefile");
    const script = await readRepo("scripts/dev-mobile-ios.sh");

    expect(makefile).toContain("bash scripts/dev-mobile-ios.sh");
    expect(script).toContain("xcrun simctl boot");
    expect(script).toContain("xcrun simctl bootstatus");
    expect(script).toContain("runtimeVersion");
    expect(script).toContain("matches.sort");
    expect(script.indexOf("xcrun simctl bootstatus")).toBeLessThan(
      script.indexOf("pnpm tauri ios dev"),
    );
  });
});
