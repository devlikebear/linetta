import { describe, expect, it } from "vitest";
import { autoMentionDoc } from "./autoMention";
import type { Entity } from "../types";

const baseEntity = {
  project_id: "p1",
  kind: "character",
  role: "",
  summary: "",
  attributes: {},
  created_at: 1,
  updated_at: 1,
} satisfies Omit<Entity, "id" | "name" | "aliases">;

function entity(id: string, name: string, aliases: string[] = []): Entity {
  return { ...baseEntity, id, name, aliases };
}

describe("autoMentionDoc", () => {
  it("converts registered entity names in text nodes", () => {
    const doc = {
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [{ type: "text", text: "카엘은 붉은 성문 앞에 섰다." }],
        },
      ],
    };

    const result = autoMentionDoc(doc, [
      entity("e1", "카엘"),
      { ...entity("e2", "붉은 성문"), kind: "place" },
    ]);

    expect(result.applied).toBe(2);
    expect(result.doc).toEqual({
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [
            { type: "mention", attrs: { id: "e1", label: "카엘" } },
            { type: "text", text: "은 " },
            { type: "mention", attrs: { id: "e2", label: "붉은 성문" } },
            { type: "text", text: " 앞에 섰다." },
          ],
        },
      ],
    });
  });

  it("keeps existing mention atoms and converts plain @names", () => {
    const doc = {
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [
            { type: "mention", attrs: { id: "e1", label: "카엘" } },
            { type: "text", text: "과 @아린이 만났다." },
          ],
        },
      ],
    };

    const result = autoMentionDoc(doc, [entity("e1", "카엘"), entity("e2", "아린")]);

    expect(result.applied).toBe(1);
    expect((result.doc as any).content[0].content).toEqual([
      { type: "mention", attrs: { id: "e1", label: "카엘" } },
      { type: "text", text: "과 " },
      { type: "mention", attrs: { id: "e2", label: "아린" } },
      { type: "text", text: "이 만났다." },
    ]);
  });

  it("prefers the longest registered surface", () => {
    const doc = {
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text: "카엘 왕자가 돌아왔다." }] }],
    };

    const result = autoMentionDoc(doc, [
      entity("short", "카엘"),
      entity("long", "카엘 왕자"),
    ]);

    expect(result.applied).toBe(1);
    expect((result.doc as any).content[0].content[0]).toEqual({
      type: "mention",
      attrs: { id: "long", label: "카엘 왕자" },
    });
  });

  it("uses aliases while preserving the matched surface label", () => {
    const doc = {
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text: "불의 화신이 검을 들었다." }] }],
    };

    const result = autoMentionDoc(doc, [entity("e1", "카엘", ["불의 화신"])]);

    expect(result.applied).toBe(1);
    expect((result.doc as any).content[0].content[0]).toEqual({
      type: "mention",
      attrs: { id: "e1", label: "불의 화신" },
    });
  });
});
