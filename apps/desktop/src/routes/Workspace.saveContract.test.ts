import { describe, expect, it } from "vitest";

import { readSource } from "../test/readSource";

const workspace = () => readSource("routes/Workspace.tsx");

/**
 * Autosave/save-contract fixes (#103, #104, #105, #106). Workspace.tsx is too
 * large to mount, so — following Workspace.agentPanel.test.ts and friends —
 * these assertions watch the source directly rather than rendering it.
 */
describe("Workspace save contract", () => {
  it("saveNow's catch block never calls setError (#105)", async () => {
    const src = await workspace();

    const saveNowStart = src.indexOf("const saveNow = useCallback(");
    expect(saveNowStart).toBeGreaterThan(-1);
    const saveNowEnd = src.indexOf("const debouncedSave = useKeyedDebouncedCallback(saveNow", saveNowStart);
    expect(saveNowEnd).toBeGreaterThan(saveNowStart);
    const saveNowBody = src.slice(saveNowStart, saveNowEnd);

    const catchIndex = saveNowBody.indexOf("} catch (e) {");
    expect(catchIndex).toBeGreaterThan(-1);
    const catchBlock = saveNowBody.slice(catchIndex);
    expect(catchBlock).not.toContain("setError(");
    expect(catchBlock).toContain('setSaveStatus({ kind: "error"');
  });

  it("handleManualSave's catch block never calls setError (#105)", async () => {
    const src = await workspace();

    const start = src.indexOf("const handleManualSave = useCallback(");
    expect(start).toBeGreaterThan(-1);
    const end = src.indexOf("\n  );", start);
    expect(end).toBeGreaterThan(start);
    const body = src.slice(start, end);

    const catchIndex = body.indexOf("} catch (e) {");
    expect(catchIndex).toBeGreaterThan(-1);
    const catchBlock = body.slice(catchIndex);
    expect(catchBlock).not.toContain("setError(");
    expect(catchBlock).toContain('setSaveStatus({ kind: "error"');
  });

  it("a save-error status is rendered near the editor, not as a route-level error screen", async () => {
    const src = await workspace();
    expect(src).toContain('saveStatus.kind === "error" && (');
    expect(src).toContain('data-testid="save-error-banner"');
  });

  it("editRevisionRef is bumped in both onChange sites (normal and ZEN)", async () => {
    const src = await workspace();
    const bumpCount = src.match(/editRevisionRef\.current \+= 1;/g)?.length ?? 0;
    expect(bumpCount).toBe(2);
  });

  it("saveNow captures the revision before awaiting the save and only clears dirty if it still matches", async () => {
    const src = await workspace();
    const saveNowStart = src.indexOf("const saveNow = useCallback(");
    const saveNowEnd = src.indexOf("const debouncedSave = useKeyedDebouncedCallback(saveNow", saveNowStart);
    const saveNowBody = src.slice(saveNowStart, saveNowEnd);

    const revIndex = saveNowBody.indexOf("const rev = editRevisionRef.current;");
    expect(revIndex).toBeGreaterThan(-1);
    // Captured before the await, not after.
    expect(revIndex).toBeLessThan(saveNowBody.indexOf("await sceneSaveQueue.save("));
    expect(saveNowBody).toContain("if (rev === editRevisionRef.current) setEditorDirty(false);");
  });

  it("handleManualSave applies the same revision-gated dirty clear as saveNow", async () => {
    const src = await workspace();
    const start = src.indexOf("const handleManualSave = useCallback(");
    const end = src.indexOf("\n  );", start);
    const body = src.slice(start, end);

    const revIndex = body.indexOf("const rev = editRevisionRef.current;");
    expect(revIndex).toBeGreaterThan(-1);
    expect(revIndex).toBeLessThan(body.indexOf("await sceneSaveQueue.save("));
    expect(body).toContain("if (rev === editRevisionRef.current) setEditorDirty(false);");
  });

  it("flushPendingSave flushes the debounce then waits for the save queue to go idle (#104)", async () => {
    const src = await workspace();
    const start = src.indexOf("const flushPendingSave = useCallback(");
    expect(start).toBeGreaterThan(-1);
    const end = src.indexOf("\n  }, [debouncedSave, sceneSaveQueue]);", start);
    expect(end).toBeGreaterThan(start);
    const body = src.slice(start, end);
    expect(body).toContain("debouncedSave.flush();");
    expect(body).toContain("await sceneSaveQueue.idle();");
  });

  it("flushPendingSave is awaited before navigating away from the workspace", async () => {
    const src = await workspace();

    // Cmd+R reload.
    expect(src).toContain("void flushPendingSave().finally(() => window.location.reload());");
    // The "back to library" breadcrumb link.
    expect(src).toContain('void flushPendingSave().finally(() => navigate("/"));');
    // Cross-project search jump.
    expect(src).toContain("await flushPendingSave();\n      navigate(`/workspace/${result.project_id}`");
    // Command-palette navigation to Threads view and Settings.
    expect(src).toContain('run: async () => { await flushPendingSave(); navigate(`/workspace/${load.project.id}/threads`); },');
    expect(src).toContain('run: async () => { await flushPendingSave(); navigate("/settings"); },');
  });

  it("enterZen updates load.initialDoc with the live document before opening ZEN, mirroring exitZen", async () => {
    const src = await workspace();
    const enterZenStart = src.indexOf("const enterZen = useCallback(");
    const exitZenStart = src.indexOf("const exitZen = useCallback(");
    expect(enterZenStart).toBeGreaterThan(-1);
    expect(exitZenStart).toBeGreaterThan(enterZenStart);
    const enterZenBody = src.slice(enterZenStart, exitZenStart);

    const getDocIndex = enterZenBody.indexOf("editorRef.current?.getDoc()");
    const setLoadIndex = enterZenBody.indexOf("setLoad((prev) => (prev ? { ...prev, initialDoc: liveDoc } : prev));");
    const setZenOpenIndex = enterZenBody.indexOf("setZenOpen(true);");
    expect(getDocIndex).toBeGreaterThan(-1);
    expect(setLoadIndex).toBeGreaterThan(getDocIndex);
    expect(setZenOpenIndex).toBeGreaterThan(setLoadIndex);
  });
});
