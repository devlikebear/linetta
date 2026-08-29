import { describe, expect, it } from "vitest";

import { readAppFile } from "../test/readSource";
import { entityRolePresets } from "./i18n";
import type { AppLanguage } from "./types";

/**
 * The engine decides which entities are "core" — the story skeleton a connected
 * agent gets even when the open scene has not mentioned them. It decides by
 * matching `Entity.Role` against a table of role labels.
 *
 * The role labels come from here, in the writer's own language. So the table
 * has to hold every preset, in all three, and nothing checks that: the table is
 * Go, the presets are TypeScript, and a missing entry fails silently — the
 * agent just gets a story context with no cast (#45).
 */
const LANGUAGES: AppLanguage[] = ["ko", "en", "ja"];

async function coreRoleTable(): Promise<string> {
  const src = await readAppFile("../../engine/internal/entity/entity.go");
  return src.slice(
    src.indexOf("var coreRolesByKind"),
    src.indexOf("// IsCoreRole"),
  );
}

describe("core role parity between the app and the engine", () => {
  it("finds the table (guards against the extraction silently breaking)", async () => {
    const table = await coreRoleTable();
    expect(table).toContain("KindCharacter");
    expect(table).toContain("KindPlace");
  });

  it("covers every character role the app offers, in every language", async () => {
    const table = await coreRoleTable();
    const characters = table.slice(
      table.indexOf("KindCharacter"),
      table.indexOf("KindPlace"),
    );
    for (const language of LANGUAGES) {
      for (const role of entityRolePresets(language, "character")) {
        expect(characters, `${language} character role "${role}"`).toContain(`"${role}"`);
      }
    }
  });

  it("covers every place role the app offers, in every language", async () => {
    const table = await coreRoleTable();
    const places = table.slice(table.indexOf("KindPlace"));
    for (const language of LANGUAGES) {
      for (const role of entityRolePresets(language, "place")) {
        expect(places, `${language} place role "${role}"`).toContain(`"${role}"`);
      }
    }
  });
});
