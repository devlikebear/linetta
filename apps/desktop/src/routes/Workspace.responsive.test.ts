import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "..");

async function readSource(path: string) {
  return readFile(resolve(srcRoot, path), "utf8");
}

describe("Workspace compact layout", () => {
  it("derives the size tier and seeds the outline rail per tier", async () => {
    const workspace = await readSource("routes/Workspace.tsx");

    expect(workspace).toContain('import { useSizeClass } from "../hooks/useSizeClass"');
    expect(workspace).toContain("const sizeClass = useSizeClass()");
    expect(workspace).toContain('import { reconcileInspector } from "../hooks/inspector"');
    expect(workspace).toContain("reconcileInspector(");
    expect(workspace).toContain('className="ws-tool icon-only mobile-outline-toggle"');
  });

  it("keeps the mobile workspace inside the viewport with drawer and bottom sheet panels", async () => {
    const css = await readSource("App.css");

    expect(css).toContain("@media (max-width: 860px)");
    expect(css).toContain("max-width: min(58vw, 244px)");
    expect(css).toContain(".ws-body.rail-collapsed");
    expect(css).toContain("grid-template-columns: 1fr");
    expect(css).toContain("max-width: 100vw");
    expect(css).toContain(".mobile-rail-backdrop");
    expect(css).toContain("width: min(86vw, 330px)");
    expect(css).toContain("height: min(82dvh, 680px)");
    expect(css).toContain("border-radius: var(--r-md) var(--r-md) 0 0");
  });

  it("scopes mobile bottom-sheet panel rules to the workspace screen", async () => {
    const css = await readSource("App.css");

    expect(css).toContain(".workspace .panel {");
    expect(css).not.toContain("\n  .panel {\n    position: fixed;");
    expect(css).not.toContain("\n  .panel-head {\n    padding: 12px 14px;");
    expect(css).toContain(".thread-view .panel.thread-lane");
    expect(css).toContain(".settings .set-row");
  });

  it("adds an ipad-tier layout block layered after the compact block", async () => {
    const css = await readSource("App.css");

    const ipadAt =
      "@media (min-width: 701px) and (max-width: 1180px) and (min-height: 600px) and (pointer: coarse)";
    expect(css).toContain(ipadAt);
    // ipad block must come AFTER the compact block so it overrides it
    expect(css.indexOf(ipadAt)).toBeGreaterThan(css.indexOf("@media (max-width: 860px)"));
    // inline pushing sidebar (no modal backdrop), right inspector
    expect(css).toContain(".mobile-rail-backdrop {\n    display: none;");
    expect(css).toContain("width: min(380px, 60vw)");
  });
});
