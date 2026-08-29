import { describe, expect, it } from "vitest";
import { importWarningMessage } from "./importWarnings";
import { translate } from "./i18n";
import type { MessageKey } from "./i18n";

const ko = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("ko", key, values);
const en = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("en", key, values);
const ja = (key: MessageKey, values?: Record<string, string | number>) =>
  translate("ja", key, values);

describe("importWarningMessage", () => {
  // The bug this closes: the engine wrote Korean sentences, so an English or
  // Japanese writer importing a file got Korean in the middle of their own UI.
  it("speaks the reader's language, not the engine's", () => {
    expect(importWarningMessage(en, "import.no_headings")).toContain("No headings");
    expect(importWarningMessage(ja, "import.no_headings")).toContain("見出し");
    expect(importWarningMessage(ko, "import.no_headings")).toContain("헤딩");
  });

  it("fills in the value a code carries after the colon", () => {
    const shown = importWarningMessage(en, "import.unknown_outline_preset:novella");
    expect(shown).toContain("novella");
    expect(shown).not.toContain("import.unknown_outline_preset");
  });

  it("shows an unrecognised warning rather than swallowing it", () => {
    // A newer engine can emit a code this build has no string for. Losing the
    // warning entirely would be worse than showing its raw form.
    expect(importWarningMessage(en, "import.something_new")).toBe("import.something_new");
  });

  it("covers every code the engine can emit", () => {
    for (const code of [
      "import.no_headings",
      "import.frontmatter_unreadable",
      "import.relationships_skipped",
      "import.unknown_outline_preset",
    ]) {
      expect(importWarningMessage(en, code), code).not.toBe(code);
    }
  });
});
