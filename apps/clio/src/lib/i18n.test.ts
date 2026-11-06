import { describe, expect, it } from "vitest";
import { FEATURE_NAMES } from "./types";
import { catalogKeys, t } from "./i18n";

// Every source file, as text, so literal t("...") keys can be checked.
const sources = import.meta.glob(["../**/*.tsx", "../**/*.ts", "!../**/*.test.*", "!../lib/i18n.ts"], {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

describe("i18n catalog", () => {
  it("has every key used with t(...) in the source", () => {
    const used = new Set<string>();
    for (const text of Object.values(sources)) {
      for (const m of text.matchAll(/\bt\(\s*"([^"]+)"/g)) used.add(m[1]);
      // Keys held in data, e.g. label: "voice.tab.xPost".
      for (const m of text.matchAll(/\blabel:\s*"([^"]+)"/g)) used.add(m[1]);
    }
    expect(used.size).toBeGreaterThan(10);
    const missing = [...used].filter((k) => !(catalogKeys as string[]).includes(k));
    expect(missing).toEqual([]);
  });

  it("covers keys built at runtime", () => {
    const keys = catalogKeys as string[];
    for (const f of FEATURE_NAMES) expect(keys).toContain(`voice.feature.${f}`);
    for (const r of ["retweet", "short", "duplicate"]) expect(keys).toContain(`import.skip.${r}`);
    for (const p of ["x", "threads", "blog"])
      for (const k of ["post", "reply"]) expect(keys).toContain(`import.context.${p}.${k}`);
    for (const c of ["x/post", "x/reply", "x/*", "*"]) expect(keys).toContain(`voice.context.${c}`);
  });

  it("fills placeholders", () => {
    expect(t("import.done", { inserted: 3, skipped: 1 })).toBe("3편 가져옴, 1편 건너뜀");
  });
});
