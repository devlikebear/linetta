import { describe, expect, it } from "vitest";
import { reconcileInspector, type InspectorState } from "./inspector";

const S = (f: boolean, x: boolean, c = false): InspectorState => ({
  factBook: f,
  contextual: x,
  canon: c,
});

describe("reconcileInspector", () => {
  it("leaves non-ipad tiers untouched", () => {
    const next = S(true, true);
    expect(reconcileInspector(S(false, false), next, "desktop")).toEqual(next);
    expect(reconcileInspector(S(false, false), next, "compact")).toEqual(next);
  });

  it("on ipad keeps the panel that just opened, closing the rest", () => {
    // factBook was open; contextual just opened → keep contextual only
    const out = reconcileInspector(S(true, false), S(true, true), "ipad");
    expect(out).toEqual(S(false, true));
  });

  it("on ipad keeps a single open panel as-is", () => {
    const out = reconcileInspector(S(false, false), S(false, true), "ipad");
    expect(out).toEqual(S(false, true));
  });

  it("is idempotent on its own output", () => {
    const once = reconcileInspector(S(true, false), S(true, true), "ipad");
    const twice = reconcileInspector(S(true, false), once, "ipad");
    expect(twice).toEqual(once);
  });

  it("falls back to priority order when multiple are open with no fresh opener", () => {
    const out = reconcileInspector(S(true, true), S(true, true), "ipad");
    expect(out).toEqual(S(true, false)); // factBook before contextual
  });

  it("treats the story world as one of the exclusive panels", () => {
    // Three panels now share one slot; opening canon must close the others on
    // the narrow tier rather than stacking behind them (#28).
    const out = reconcileInspector(S(true, false, false), S(true, false, true), "ipad");
    expect(out).toEqual(S(false, false, true));
  });

  it("closes the story world when another panel takes the slot", () => {
    const out = reconcileInspector(S(false, false, true), S(false, true, true), "ipad");
    expect(out).toEqual(S(false, true, false));
  });
});
