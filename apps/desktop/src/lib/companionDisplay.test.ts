import { describe, expect, it } from "vitest";
import { extractApplyOpsProposal } from "./companionDisplay";

describe("companionDisplay", () => {
  it("keeps set_scene_text ops from inline apply-ops payloads", () => {
    const proposal = extractApplyOpsProposal(
      [
        "교정안을 적용할게요.",
        "{\"summary\":\"퇴고\",\"ops_json\":\"[{\\\"op\\\":\\\"set_scene_text\\\",\\\"text\\\":\\\"새 본문\\\"}]\"}",
      ].join("\n"),
    );

    expect(proposal?.ops).toEqual([{ op: "set_scene_text", text: "새 본문" }]);
  });
});
