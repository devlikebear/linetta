import { describe, expect, it } from "vitest";
import { countAutoMentionCandidates } from "./autoMention";
import type { Entity } from "../types";

function entity(id: string, name: string, aliases: string[] = []): Entity {
  return {
    id,
    project_id: "p1",
    kind: "character",
    name,
    aliases,
    role: "",
    summary: "",
    attributes: {},
    created_at: 1,
    updated_at: 1,
  } as Entity;
}

function doc(...paragraphs: string[]) {
  return {
    type: "doc",
    content: paragraphs.map((text) => ({
      type: "paragraph",
      content: [{ type: "text", text }],
    })),
  };
}

const cast = [entity("e1", "해윤"), entity("e2", "서린"), entity("e3", "루카")];

describe("countAutoMentionCandidates", () => {
  // The regression this closes: prose naming two registered characters showed
  // "등장 0" until the writer thought to press a scan button (#32).
  it("counts registered names appearing in prose", () => {
    const scene = doc(
      "해윤은 문을 열었다. 복도는 비어 있었다.",
      "서린이 뒤따라 들어왔고, 둘은 아무 말도 하지 않았다.",
    );
    expect(countAutoMentionCandidates(scene, cast)).toBe(2);
  });

  it("counts every occurrence, so a repeated name is not undercounted", () => {
    const scene = doc("해윤과 해윤의 그림자.");
    expect(countAutoMentionCandidates(scene, cast)).toBe(2);
  });

  it("ignores a cast member who is not in the scene", () => {
    expect(countAutoMentionCandidates(doc("해윤은 혼자였다."), cast)).toBe(1);
  });

  it("does not count a name that is already linked", () => {
    const scene = {
      type: "doc",
      content: [{
        type: "paragraph",
        content: [
          { type: "mention", attrs: { id: "e1", label: "해윤" } },
          { type: "text", text: "은 문을 열었다." },
        ],
      }],
    };
    expect(countAutoMentionCandidates(scene, cast)).toBe(0);
  });

  it("finds a character by an alias", () => {
    const withAlias = [entity("e1", "해윤", ["윤 선생"])];
    expect(countAutoMentionCandidates(doc("윤 선생이 들어왔다."), withAlias)).toBe(1);
  });

  it("returns zero when the work has no registered cast", () => {
    expect(countAutoMentionCandidates(doc("해윤은 문을 열었다."), [])).toBe(0);
  });

  it("leaves the document alone", () => {
    const scene = doc("해윤은 문을 열었다.");
    const before = JSON.stringify(scene);
    countAutoMentionCandidates(scene, cast);
    // Counting must not rewrite prose — that is the whole reason it is separate
    // from applying.
    expect(JSON.stringify(scene)).toBe(before);
  });
});
