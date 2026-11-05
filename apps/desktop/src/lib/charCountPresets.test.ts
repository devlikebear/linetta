import { describe, expect, it } from "vitest";
import type { CharCountPreset } from "./types";
import {
  BUILTIN_CHAR_COUNT_PRESETS,
  lengthRuleOf,
  matchingPresetKey,
  newCharCountPreset,
  ruleForPresetKey,
} from "./charCountPresets";

const mine: CharCountPreset = { id: "mine", name: "내 공모전", char_count_basis: "with_spaces", episode_char_target: 5000 };

describe("char count presets", () => {
  it("reads a work's rule with the defaults an older engine leaves out", () => {
    expect(lengthRuleOf({ episode_char_target: 0 })).toEqual({ char_count_basis: "with_spaces", episode_char_target: 5000 });
    expect(lengthRuleOf({ char_count_basis: "without_spaces", episode_char_target: 3200 })).toEqual({
      char_count_basis: "without_spaces",
      episode_char_target: 3200,
    });
  });

  it("gives every built-in preset a distinct rule", () => {
    const rules = BUILTIN_CHAR_COUNT_PRESETS.map((p) => `${p.char_count_basis}:${p.episode_char_target}`);
    expect(new Set(rules).size).toBe(rules.length);
  });

  it("names the preset a rule matches, preferring the one the writer picked", () => {
    const rule = { char_count_basis: "with_spaces" as const, episode_char_target: 5000 };
    expect(matchingPresetKey(rule, [mine])).toBe("builtin:standard-with-spaces");
    expect(matchingPresetKey(rule, [mine], "user:mine")).toBe("user:mine");
    // A preference that no longer matches the work falls back to what does.
    expect(matchingPresetKey({ ...rule, episode_char_target: 6000 }, [mine], "user:mine")).toBe("builtin:long-with-spaces");
    expect(matchingPresetKey({ ...rule, episode_char_target: 4321 }, [mine])).toBe("");
  });

  it("looks a rule up by key and refuses keys on offer nowhere", () => {
    expect(ruleForPresetKey("user:mine", [mine])).toEqual({ char_count_basis: "with_spaces", episode_char_target: 5000 });
    expect(ruleForPresetKey("builtin:short-without-spaces", [])).toEqual({ char_count_basis: "without_spaces", episode_char_target: 3000 });
    expect(ruleForPresetKey("user:gone", [mine])).toBeNull();
    expect(ruleForPresetKey("", [mine])).toBeNull();
  });

  it("makes a saved preset from a rule with a trimmed name and a fresh id", () => {
    const rule = { char_count_basis: "without_spaces" as const, episode_char_target: 3500 };
    const a = newCharCountPreset("  연재처  ", rule);
    const b = newCharCountPreset("연재처", rule);
    expect(a).toMatchObject({ name: "연재처", char_count_basis: "without_spaces", episode_char_target: 3500 });
    expect(a.id).not.toBe(b.id);
  });
});
