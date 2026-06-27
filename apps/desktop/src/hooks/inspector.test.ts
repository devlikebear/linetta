import { describe, expect, it } from "vitest";
import { reconcileInspector, type InspectorState } from "./inspector";

const S = (c: boolean, f: boolean, x: boolean): InspectorState => ({
  companion: c,
  factBook: f,
  contextual: x,
});

describe("reconcileInspector", () => {
  it("leaves non-ipad tiers untouched", () => {
    const next = S(true, true, false);
    expect(reconcileInspector(S(false, false, false), next, "desktop")).toEqual(next);
    expect(reconcileInspector(S(false, false, false), next, "compact")).toEqual(next);
  });

  it("on ipad keeps the panel that just opened, closing the rest", () => {
    // companion was open; factBook just opened → keep factBook only
    const out = reconcileInspector(S(true, false, false), S(true, true, false), "ipad");
    expect(out).toEqual(S(false, true, false));
  });

  it("on ipad keeps a single open panel as-is", () => {
    const out = reconcileInspector(S(false, false, false), S(false, false, true), "ipad");
    expect(out).toEqual(S(false, false, true));
  });

  it("is idempotent on its own output", () => {
    const once = reconcileInspector(S(true, false, false), S(true, true, false), "ipad");
    const twice = reconcileInspector(S(true, false, false), once, "ipad");
    expect(twice).toEqual(once);
  });

  it("falls back to priority order when multiple are open with no fresh opener", () => {
    const out = reconcileInspector(S(false, true, true), S(false, true, true), "ipad");
    expect(out).toEqual(S(false, true, false)); // factBook before contextual
  });
});
