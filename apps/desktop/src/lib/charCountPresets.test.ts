import { describe, expect, it } from "vitest";
import type { CharCountPreset } from "./types";
import {
  BUILTIN_CHAR_COUNT_PRESETS,
  lengthRuleOf,
  matchingPresetKey,
  newCharCountPreset,
  ruleForPresetKey,
  validLengthRule,
  type LengthRule,
} from "./charCountPresets";

const mine: CharCountPreset = { id: "mine", name: "내 공모전", char_count_basis: "with_spaces", episode_char_target: 5000 };
const contest: CharCountPreset = {
  id: "contest",
  name: "범위 공모전",
  char_count_basis: "with_spaces",
  episode_char_min: 4500,
  episode_char_target: 5000,
  episode_char_max: 5500,
};
const rule = (target: number, extra: Partial<LengthRule> = {}): LengthRule => ({
  char_count_basis: "with_spaces",
  episode_char_target: target,
  episode_char_min: 0,
  episode_char_max: 0,
  ...extra,
});

describe("char count presets", () => {
  it("reads a work's rule with the defaults an older engine leaves out", () => {
    expect(lengthRuleOf({ episode_char_target: 0 })).toEqual(rule(5000));
    expect(lengthRuleOf({ char_count_basis: "without_spaces", episode_char_target: 3200 })).toEqual({
      char_count_basis: "without_spaces",
      episode_char_target: 3200,
      episode_char_min: 0,
      episode_char_max: 0,
    });
    expect(lengthRuleOf({ episode_char_target: 5300, episode_char_min: 5000 })).toEqual(rule(5300, { episode_char_min: 5000 }));
  });

  it("gives every built-in preset a distinct, valid rule", () => {
    const rules = BUILTIN_CHAR_COUNT_PRESETS.map(
      (p) => `${p.char_count_basis}:${p.episode_char_min}:${p.episode_char_target}:${p.episode_char_max}`,
    );
    expect(new Set(rules).size).toBe(rules.length);
    for (const p of BUILTIN_CHAR_COUNT_PRESETS) expect(validLengthRule(lengthRuleOf(p))).toBe(true);
  });

  it("offers the web novel default (at least 5,000 with spaces) as a built-in", () => {
    expect(ruleForPresetKey("builtin:minimum-with-spaces", [])).toEqual(rule(5300, { episode_char_min: 5000 }));
    expect(matchingPresetKey(rule(5300, { episode_char_min: 5000 }), [])).toBe("builtin:minimum-with-spaces");
  });

  it("names the preset a rule matches, preferring the one the writer picked", () => {
    expect(matchingPresetKey(rule(5000), [mine])).toBe("builtin:standard-with-spaces");
    expect(matchingPresetKey(rule(5000), [mine], "user:mine")).toBe("user:mine");
    // A preference that no longer matches the work falls back to what does.
    expect(matchingPresetKey(rule(6000), [mine], "user:mine")).toBe("builtin:long-with-spaces");
    expect(matchingPresetKey(rule(4321), [mine])).toBe("");
    // Bounds are part of the rule: the same target with a minimum is another preset.
    expect(matchingPresetKey(rule(5000, { episode_char_min: 4500, episode_char_max: 5500 }), [mine, contest])).toBe("user:contest");
    expect(matchingPresetKey(rule(5000, { episode_char_min: 4000 }), [mine])).toBe("");
  });

  it("looks a rule up by key and refuses keys on offer nowhere", () => {
    expect(ruleForPresetKey("user:mine", [mine])).toEqual(rule(5000));
    expect(ruleForPresetKey("user:contest", [contest])).toEqual(rule(5000, { episode_char_min: 4500, episode_char_max: 5500 }));
    expect(ruleForPresetKey("builtin:short-without-spaces", [])).toEqual({
      char_count_basis: "without_spaces",
      episode_char_target: 3000,
      episode_char_min: 0,
      episode_char_max: 0,
    });
    expect(ruleForPresetKey("user:gone", [mine])).toBeNull();
    expect(ruleForPresetKey("", [mine])).toBeNull();
  });

  it("makes a saved preset from a rule with a trimmed name and a fresh id", () => {
    const r = { ...rule(3500), char_count_basis: "without_spaces" as const };
    const a = newCharCountPreset("  연재처  ", r);
    const b = newCharCountPreset("연재처", r);
    expect(a).toEqual({ id: a.id, name: "연재처", char_count_basis: "without_spaces", episode_char_target: 3500 });
    expect(a.id).not.toBe(b.id);
    expect(newCharCountPreset("범위", rule(5000, { episode_char_min: 4500, episode_char_max: 5500 }))).toMatchObject({
      episode_char_min: 4500,
      episode_char_max: 5500,
    });
  });

  it("checks a rule the way the engine does", () => {
    expect(validLengthRule(rule(5000))).toBe(true);
    expect(validLengthRule(rule(5000, { episode_char_min: 5000 }))).toBe(true);
    expect(validLengthRule(rule(5500, { episode_char_max: 5500 }))).toBe(true);
    expect(validLengthRule(rule(4999, { episode_char_min: 5000 }))).toBe(false);
    expect(validLengthRule(rule(5501, { episode_char_max: 5500 }))).toBe(false);
    expect(validLengthRule(rule(6000, { episode_char_min: 6000, episode_char_max: 5000 }))).toBe(false);
  });
});
