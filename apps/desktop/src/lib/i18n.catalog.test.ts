import { describe, expect, it } from "vitest";
import { messageCatalogs } from "./i18n";

const placeholders = (value: string) =>
  [...value.matchAll(/\{([A-Za-z0-9_]+)\}/g)].map((match) => match[1]).sort();

describe("i18n message catalogs", () => {
  it("keeps the same keys in Korean, English, and Japanese", () => {
    const baseline = Object.keys(messageCatalogs.ko).sort();
    expect(Object.keys(messageCatalogs.en).sort()).toEqual(baseline);
    expect(Object.keys(messageCatalogs.ja).sort()).toEqual(baseline);
  });

  it("keeps interpolation placeholders aligned across languages", () => {
    for (const key of Object.keys(messageCatalogs.ko) as Array<keyof typeof messageCatalogs.ko>) {
      expect(placeholders(messageCatalogs.en[key]), `English placeholders for ${key}`).toEqual(
        placeholders(messageCatalogs.ko[key]),
      );
      expect(placeholders(messageCatalogs.ja[key]), `Japanese placeholders for ${key}`).toEqual(
        placeholders(messageCatalogs.ko[key]),
      );
    }
  });
});
