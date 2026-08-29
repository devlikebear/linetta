import { describe, expect, it } from "vitest";

import { readSource } from "../test/readSource";

describe("FactBookPanel CSS", () => {
  // The companion prose this used to guard is gone, but the reason it existed
  // is not: a source URL has no spaces and can be longer than the panel, and
  // the dossier is made of source URLs.
  it("keeps long source URLs inside the panel", async () => {
    const css = await readSource("components/FactBookPanel.css");

    expect(css).toContain(".fact-sources a");
    expect(css).toContain("overflow-wrap: anywhere");
    expect(css).toContain("word-break: break-word");
    expect(css).toContain("text-overflow: ellipsis");
  });

  it("lets the claim and source fields fill the panel width", async () => {
    const css = await readSource("components/FactBookPanel.css");

    expect(css).toContain(".fact-new input");
    expect(css).toContain("width: 100%");
  });
});
