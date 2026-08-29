import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "..");

async function workspace() {
  return readFile(resolve(srcRoot, "routes/Workspace.tsx"), "utf8");
}

/**
 * A panel with no mount site is this codebase's recurring failure — it compiles,
 * it has tests, and no writer can reach it. #28 is about reachability, so the
 * wiring is asserted, not assumed.
 */
describe("Workspace story world panel", () => {
  it("mounts the panel", async () => {
    const src = await workspace();
    expect(src).toContain('import { CanonPanel } from "../components/CanonPanel"');
    expect(src).toContain("<CanonPanel");
  });

  it("gives the writer two ways in: the toolbar and the palette", async () => {
    const src = await workspace();
    expect(src).toContain("onClick={toggleCanon}");
    expect(src).toContain('id: "toggle-canon"');
    // The palette list is a useMemo; a callback missing from its deps freezes
    // the command on a stale closure.
    expect(src).toContain("toggleCanon, gitSyncAvailable]");
  });

  it("shares the inspector slot instead of stacking", async () => {
    const src = await workspace();
    // Every other toggle must close it, or the toolbar shows it active while
    // another panel holds the slot.
    const toggles = src.slice(
      src.indexOf("const toggleFactBook"),
      src.indexOf("const toggleCanon"),
    );
    expect(toggles.match(/setCanonOpen\(false\)/g)?.length).toBe(2);
    expect(src).toContain("canon: canonOpen");
    expect(src).toContain("(factBookOpen || contextualEditOpen || canonOpen) ? \" right-wide\" : \"\"");
  });

  it("sits below the sheets, so opening a record does not blank the panel", async () => {
    const src = await workspace();
    // EntitySheet takes the same slot. If canon were checked first, clicking a
    // row would set entitySheetId and nothing on screen would change.
    expect(src.indexOf("<EntitySheet")).toBeLessThan(src.indexOf("<CanonPanel"));
    expect(src).toContain("onOpenEntity={(entityId) => setEntitySheetId(entityId)}");
  });

  it("refreshes the cast when an external agent changes the work", async () => {
    const src = await workspace();
    // linetta_apply_story_ops carries create_entity, and its notification is
    // the same one that refreshes the outline.
    const handler = src.slice(
      src.indexOf("onOutlineChanged:"),
      src.indexOf("onSceneChanged:"),
    );
    expect(handler).toContain("setCanonRefreshKey((k) => k + 1)");
    expect(src).toContain("refreshKey={canonRefreshKey}");
  });
});
