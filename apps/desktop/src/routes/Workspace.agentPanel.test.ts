import { describe, expect, it } from "vitest";

import { readSource } from "../test/readSource";

const workspace = () => readSource("routes/Workspace.tsx");

/**
 * Task 3 delivers the agent panel's shell only: it opens, it closes, and it
 * does not collide with the other right-hand panels. Workspace.tsx is too
 * large to mount, so — following Workspace.canon.test.ts and
 * Workspace.mcpChanges.test.ts — these assertions watch the source directly.
 */
describe("Workspace agent panel wiring", () => {
  it("mounts the panel above the ContextPanel fallback", async () => {
    const src = await workspace();
    expect(src).toContain('import { AgentPanel } from "../components/agent/AgentPanel"');
    expect(src).toContain("<AgentPanel");
    // ContextPanel is the else-fallback; the agent panel must be checked
    // before it or it can never win the slot.
    expect(src.indexOf("<AgentPanel")).toBeLessThan(src.indexOf('sizeClass === "desktop"'));
  });

  it("closes the other inspector panels when the agent panel opens", async () => {
    const src = await workspace();

    // toggleFactBook, toggleContextualEdit and toggleCanon must each close
    // the agent panel too, or the toolbar can show two panels active while
    // they share one slot.
    const toggles = src.slice(
      src.indexOf("const toggleFactBook"),
      src.indexOf("const toggleAgent"),
    );
    expect(toggles.match(/setAgentOpen\(false\)/g)?.length).toBe(3);
  });

  it("closes the other inspector panels from the agent panel's own toggle", async () => {
    const src = await workspace();

    const toggleAgentBody = src.slice(
      src.indexOf("const toggleAgent"),
      src.indexOf("const runSelectionFactCheck"),
    );
    expect(toggleAgentBody).toContain("setFactBookOpen(false)");
    expect(toggleAgentBody).toContain("setContextualEditOpen(false)");
    expect(toggleAgentBody).toContain("setCanonOpen(false)");
  });

  it("shares the inspector slot instead of stacking on iPad", async () => {
    const src = await workspace();
    expect(src).toContain("agent: agentOpen");
    expect(src).toContain(
      '(factBookOpen || contextualEditOpen || canonOpen || agentOpen) ? " right-wide" : ""',
    );
  });

  it("binds Cmd/Ctrl+J to toggleAgent", async () => {
    const src = await workspace();

    const jIndex = src.indexOf('e.key.toLowerCase() === "j"');
    expect(jIndex).toBeGreaterThan(-1);
    const jBranch = src.slice(jIndex, jIndex + 120);
    expect(jBranch).toContain("toggleAgent();");
  });

  it("corrects the stale comment that used to say Cmd+J was deliberately unbound", async () => {
    const src = await workspace();

    // That claim is false now that the binding sits right next to it — a
    // stale comment saying a binding does not exist is worse than none.
    expect(src).not.toContain(
      "Cmd+I (AI draft) and Cmd+J (companion) are gone with the companion and are",
    );
    expect(src).not.toContain("deliberately left unbound rather than reassigned. The guards");
    // Cmd+I is still genuinely unbound, and the comment must still say so.
    expect(src).toContain(
      "Cmd+I (AI draft) is gone with the companion and is deliberately left\n  // unbound rather than reassigned.",
    );
    // Cmd+J is documented as bound to the agent panel, not as absent.
    expect(src).toContain("Cmd+J agent panel");
  });
});
