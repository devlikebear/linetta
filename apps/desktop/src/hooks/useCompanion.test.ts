import { describe, expect, it } from "vitest";
import { stripProposalBlock } from "../lib/companionDisplay";

describe("stripProposalBlock", () => {
  it("removes proposal and query fenced blocks from displayed prose", () => {
    const text = [
      "좋아요. 먼저 확인했어요.",
      "```linetta-query",
      "{\"queries\":[]}",
      "```",
      "계속 이어지는 답.",
      "```linetta-proposal",
      "{\"ops\":[]}",
      "```",
    ].join("\n");

    expect(stripProposalBlock(text)).toBe("좋아요. 먼저 확인했어요.\n\n계속 이어지는 답.");
  });
});
