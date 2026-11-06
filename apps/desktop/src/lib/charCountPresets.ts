import type { CharCountBasis, CharCountPreset } from "./types";
import { charCountBasisOf } from "./charCount";

/** The episode length a work without one aims for (engine project.DefaultEpisodeCharTarget). */
export const DEFAULT_EPISODE_CHAR_TARGET = 5000;

export type BuiltinCharCountPresetNameKey =
  | "charPreset.builtin.standardWithSpaces"
  | "charPreset.builtin.standardWithoutSpaces"
  | "charPreset.builtin.shortWithoutSpaces"
  | "charPreset.builtin.longWithSpaces";

export interface BuiltinCharCountPreset {
  id: string;
  nameKey: BuiltinCharCountPresetNameKey;
  char_count_basis: CharCountBasis;
  episode_char_target: number;
}

/** Length rules every writer starts with (#239). Named for the length they
 *  set, not for a site: a site's or contest's own numbers belong in the
 *  writer's presets until they are confirmed here. */
export const BUILTIN_CHAR_COUNT_PRESETS: readonly BuiltinCharCountPreset[] = [
  { id: "standard-with-spaces", nameKey: "charPreset.builtin.standardWithSpaces", char_count_basis: "with_spaces", episode_char_target: 5000 },
  { id: "standard-without-spaces", nameKey: "charPreset.builtin.standardWithoutSpaces", char_count_basis: "without_spaces", episode_char_target: 4000 },
  { id: "short-without-spaces", nameKey: "charPreset.builtin.shortWithoutSpaces", char_count_basis: "without_spaces", episode_char_target: 3000 },
  { id: "long-with-spaces", nameKey: "charPreset.builtin.longWithSpaces", char_count_basis: "with_spaces", episode_char_target: 6000 },
];

/** The value a picker option carries, so built-in and saved ids never collide. */
export type CharCountPresetKey = `builtin:${string}` | `user:${string}`;

export interface LengthRule {
  char_count_basis: CharCountBasis;
  episode_char_target: number;
}

/** The length rule a work currently follows. */
export function lengthRuleOf(project: { char_count_basis?: CharCountBasis; episode_char_target: number }): LengthRule {
  return {
    char_count_basis: charCountBasisOf(project),
    episode_char_target: project.episode_char_target > 0 ? project.episode_char_target : DEFAULT_EPISODE_CHAR_TARGET,
  };
}

function sameRule(a: LengthRule, b: LengthRule): boolean {
  return a.char_count_basis === b.char_count_basis && a.episode_char_target === b.episode_char_target;
}

/** Every preset a picker offers, keyed, saved ones after the built-ins. */
export function allCharCountPresets(userPresets: readonly CharCountPreset[]): Array<{ key: CharCountPresetKey; rule: LengthRule }> {
  return [
    ...BUILTIN_CHAR_COUNT_PRESETS.map((p) => ({ key: `builtin:${p.id}` as const, rule: p })),
    ...userPresets.map((p) => ({ key: `user:${p.id}` as const, rule: p })),
  ];
}

/** The rule key names, or null if it names nothing on offer. */
export function ruleForPresetKey(key: string, userPresets: readonly CharCountPreset[]): LengthRule | null {
  const found = allCharCountPresets(userPresets).find((p) => p.key === key);
  return found ? { char_count_basis: found.rule.char_count_basis, episode_char_target: found.rule.episode_char_target } : null;
}

/** Which preset the work's rule is: preferred if it still matches (two
 *  presets can set the same rule), else the first that does, else "" for a
 *  rule the writer set by hand. */
export function matchingPresetKey(
  rule: LengthRule,
  userPresets: readonly CharCountPreset[],
  preferred: string | null = null,
): CharCountPresetKey | "" {
  const all = allCharCountPresets(userPresets);
  const pick = preferred ? all.find((p) => p.key === preferred && sameRule(p.rule, rule)) : undefined;
  return (pick ?? all.find((p) => sameRule(p.rule, rule)))?.key ?? "";
}

/** A new saved preset for rule. */
export function newCharCountPreset(name: string, rule: LengthRule): CharCountPreset {
  return {
    id: `p-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
    name: name.trim(),
    char_count_basis: rule.char_count_basis,
    episode_char_target: rule.episode_char_target,
  };
}

/** Matches the engine's settings.MaxCharCountPresetNameRunes. */
export const MAX_CHAR_COUNT_PRESET_NAME = 40;
