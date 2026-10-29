import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));

describe("illustration images", () => {
  // The hook hands <img> a blob: URL. The dev server sends no CSP, so only a
  // built app shows the picture as broken when the policy leaves blob: out.
  it("are allowed by the app's content security policy", async () => {
    const config = JSON.parse(await readFile(resolve(here, "../../../src-tauri/tauri.conf.json"), "utf8"));
    const imgSrc = String(config.app.security.csp)
      .split(";")
      .map((d) => d.trim().split(/\s+/))
      .find(([name]) => name === "img-src");
    expect(imgSrc).toContain("blob:");
  });
});
