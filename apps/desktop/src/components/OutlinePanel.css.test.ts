import { describe, expect, it } from "vitest";

import { readSource } from "../test/readSource";

describe("OutlinePanel CSS", () => {
  it("keeps long doctor issue lists scrollable without hiding repair actions", async () => {
    const css = await readSource("components/OutlinePanel.css");

    expect(css).toContain(".outline-doctor ul");
    expect(css).toContain("max-height: min(34vh, 280px)");
    expect(css).toContain("overflow-y: auto");
  });

  it("keeps episode progress compact in the outline rail", async () => {
    const css = await readSource("components/OutlinePanel.css");

    expect(css).toContain(".episode-progress");
    expect(css).toContain("width: 74px");
    expect(css).toContain(".episode-meter-fill.is-complete");
  });
});
