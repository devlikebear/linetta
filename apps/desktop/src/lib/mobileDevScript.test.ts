import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "../../../..");

async function readRepo(path: string) {
  return readFile(resolve(repoRoot, path), "utf8");
}

describe("iOS dev simulator launcher", () => {
  it("applies the Swift compatibility wrapper to CI and signed iOS builds", async () => {
    for (const workflow of ["mobile-engine.yml", "mobile-release.yml"]) {
      const source = await readRepo(`.github/workflows/${workflow}`);
      expect(source).toContain('echo "${GITHUB_WORKSPACE}/scripts/ios-toolchain" >> "${GITHUB_PATH}"');
    }
  });

  it.skipIf(process.platform === "win32")("uses the native Swift build system without changing compiler query arguments", async () => {
    const dir = await mkdtemp(resolve(tmpdir(), "linetta-swift-test-"));
    try {
      await writeFile(resolve(dir, "xcrun"), '#!/bin/sh\nprintf "%s\\n" "$@"\n', { mode: 0o755 });
      const run = (...args: string[]) => execFileSync("sh", [
        resolve(repoRoot, "scripts/ios-toolchain/swift"), ...args,
      ], { env: { ...process.env, PATH: `${dir}:${process.env.PATH}` }, encoding: "utf8" }).trim().split("\n");
      expect(run("build", "--sdk", "/SDK with spaces")).toEqual([
        "swift", "build", "--build-system", "native", "--sdk", "/SDK with spaces",
      ]);
      expect(run("-print-target-info")).toEqual(["swift", "-print-target-info"]);
      expect(await readRepo("Makefile")).toContain('PATH="$(CURDIR)/scripts/ios-toolchain:$$PATH"');
      expect(await readRepo("scripts/dev-mobile-ios.sh")).toContain('export PATH="${ROOT}/scripts/ios-toolchain:${PATH}"');
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });

  it("uses the same supported iOS deployment target for the app and embedded engine", async () => {
    const config = JSON.parse(await readRepo("apps/desktop/src-tauri/tauri.conf.json"));
    expect(config.bundle.iOS?.minimumSystemVersion).toBe("15.0");
    const rustBuild = await readRepo("apps/desktop/src-tauri/build.rs");
    const engineBuild = await readRepo("apps/desktop/src-tauri/scripts/build-engine-ios.sh");
    for (const source of [rustBuild, engineBuild]) {
      expect(source).toContain("-mios-simulator-version-min=15.0");
      expect(source).toContain("-miphoneos-version-min=15.0");
    }
  });

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
