import { describe, expect, it } from "vitest";

import { readSource } from "../test/readSource";

const workspace = () => readSource("routes/Workspace.tsx");

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
    expect(src).toContain("toggleCanon, gitSyncAvailable, toggleAgent, agentAvailable, flushPendingSave, publishAvailable, openPublish, vaultAvailable, openVault]");
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
    expect(src).toContain("(factBookOpen || contextualEditOpen || canonOpen || agentOpen) ? \" right-wide\" : \"\"");
  });

  it("sits below the sheets, so opening a record does not blank the panel", async () => {
    const src = await workspace();
    // EntitySheet takes the same slot. If canon were checked first, clicking a
    // row would set entitySheetId and nothing on screen would change.
    expect(src.indexOf("<EntitySheet")).toBeLessThan(src.indexOf("<CanonPanel"));
    expect(src).toContain("onOpenEntity={(entityId) => setEntitySheetId(entityId)}");
  });

  it("clears the continuity highlight target when any inspector-slot panel opens", async () => {
    const src = await workspace();
    // factBookHighlightId (a continuity observation's fact ref, story-assist
    // Phase 2) must be reset by every one of these four toggles, the same
    // way factBookSelectedClaimRequest already is — otherwise reopening the
    // Fact Book plainly later still carries a stale highlight target from
    // whatever observation was last opened, scoping the list to the whole
    // work instead of this scene and highlighting an unrelated card (code
    // review caught this).
    for (const name of ["toggleFactBook", "toggleContextualEdit", "toggleCanon", "toggleAgent"]) {
      const start = src.indexOf(`const ${name} = `);
      const body = src.slice(start, src.indexOf("}, []);", start));
      expect(body).toContain("setFactBookHighlightId(null)");
    }
  });

  it("clears the continuity focus request when any inspector-slot panel opens", async () => {
    const src = await workspace();
    // continuityFocusRequest (the open-observations badge's own "pull the
    // section open" signal) must be reset by every one of these four
    // toggles, the same way factBookHighlightId already is above —
    // otherwise opening, say, the Fact Book plainly later and then Story
    // World again would replay a stale badge click and force the
    // continuity section open/scene-scoped even though the writer never
    // clicked the badge this time.
    for (const name of ["toggleFactBook", "toggleContextualEdit", "toggleCanon", "toggleAgent"]) {
      const start = src.indexOf(`const ${name} = `);
      const body = src.slice(start, src.indexOf("}, []);", start));
      expect(body).toContain("setContinuityFocusRequest(null)");
    }
  });

  it("opens Story World's continuity section scoped to the current scene from the open-observations badge", async () => {
    const src = await workspace();
    // ContextPanel's badge must not just flip canonOpen — it needs its own
    // handler so CanonPanel/ContinuitySection can tell "plain open" apart
    // from "open and show me this scene's open observations", including
    // when canonOpen was already true (a plain setCanonOpen(true) would be
    // a no-op re-render there).
    expect(src).toContain("onOpenContinuity={openContinuityForScene}");

    const start = src.indexOf("const openContinuityForScene = ");
    expect(start).toBeGreaterThan(-1);
    const body = src.slice(start, src.indexOf("}, []);", start));
    // Same "close every other inspector-slot panel and sheet, clear stale
    // highlight state" shape the toggles above follow...
    expect(body).toContain("setFactBookHighlightId(null)");
    expect(body).toContain("setFactBookOpen(false)");
    expect(body).toContain("setContextualEditOpen(false)");
    expect(body).toContain("setAgentOpen(false)");
    expect(body).toContain("setEntitySheetId(null)");
    expect(body).toContain("setThreadSheetId(null)");
    // ...then actually opens Story World and bumps the counter
    // ContinuitySection watches, even if it was already open.
    expect(body).toContain("setCanonOpen(true)");
    expect(body).toContain("setContinuityFocusRequest(");
  });

  it("forwards the continuity focus request into CanonPanel", async () => {
    const src = await workspace();
    expect(src).toContain("continuityFocusRequest={continuityFocusRequest}");
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
