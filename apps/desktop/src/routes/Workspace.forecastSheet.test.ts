import { describe, expect, it } from "vitest";

import { readSource } from "../test/readSource";

const workspace = () => readSource("routes/Workspace.tsx");

/**
 * Review finding f2 (story-assist Phase 4 review, round 2): ForecastSheet's
 * own projectId prop is load.project.id, which keeps naming the OLD project
 * until the "Initial load" effect's async fetch for a NEW one resolves —
 * load is never reset to null in between. A jump to another project while
 * the sheet is open (global search, LibraryAll.tsx) used to leave it
 * showing the old project's forecast cards, wired to the old project's
 * forecast_id, for as long as that fetch took. Workspace.tsx is too large
 * to mount, so — following Workspace.agentPanel.test.ts's own convention —
 * this watches the source directly.
 */
describe("Workspace forecast sheet wiring", () => {
  it("closes the forecast sheet the instant the URL's project id changes, not after the new project finishes loading", async () => {
    const src = await workspace();
    expect(src).toContain("useEffect(() => { setForecastSheetOpen(false); }, [projectId]);");
  });
});
