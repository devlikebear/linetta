import type { CharCountBasis, CharCountPreset } from "./types";
import { charCountBasisOf } from "./charCount";

/** The episode length a work without one aims for (engine project.DefaultEpisodeCharTarget). */
export const DEFAULT_EPISODE_CHAR_TARGET = 5000;

export type BuiltinCharCountPresetNameKey =
  | "charPreset.builtin.minimumWithSpaces"
  | "charPreset.builtin.standardWithSpaces"
  | "charPreset.builtin.standardWithoutSpaces"
  | "charPreset.builtin.shortWithoutSpaces"
  | "charPreset.builtin.longWithSpaces";

export interface BuiltinCharCountPreset {
  id: string;
  nameKey: BuiltinCharCountPresetNameKey;
  char_count_basis: CharCountBasis;
  episode_char_target: number;
  episode_char_min: number;
  episode_char_max: number;
}

/** Length rules every writer starts with (#239). Named for the length they
 *  set, not for a site: a site's or contest's own numbers belong in the
 *  writer's presets until they are confirmed here. */
export const BUILTIN_CHAR_COUNT_PRESETS: readonly BuiltinCharCountPreset[] = [
  // What a new web novel starts on (engine project.DefaultWebnovelEpisodeChar*):
  // "공백 포함 5,000자 이상", aiming a little past the minimum.
  { id: "minimum-with-spaces", nameKey: "charPreset.builtin.minimumWithSpaces", char_count_basis: "with_spaces", episode_char_min: 5000, episode_char_target: 5300, episode_char_max: 0 },
  { id: "standard-with-spaces", nameKey: "charPreset.builtin.standardWithSpaces", char_count_basis: "with_spaces", episode_char_min: 0, episode_char_target: 5000, episode_char_max: 0 },
  { id: "standard-without-spaces", nameKey: "charPreset.builtin.standardWithoutSpaces", char_count_basis: "without_spaces", episode_char_min: 0, episode_char_target: 4000, episode_char_max: 0 },
  { id: "short-without-spaces", nameKey: "charPreset.builtin.shortWithoutSpaces", char_count_basis: "without_spaces", episode_char_min: 0, episode_char_target: 3000, episode_char_max: 0 },
  { id: "long-with-spaces", nameKey: "charPreset.builtin.longWithSpaces", char_count_basis: "with_spaces", episode_char_min: 0, episode_char_target: 6000, episode_char_max: 0 },
];

/** The rule the work-wide suggestion offers a web novel with no bounds. */
export const MINIMUM_PRESET_ID = "minimum-with-spaces";

/** The value a picker option carries, so built-in and saved ids never collide. */
export type CharCountPresetKey = `builtin:${string}` | `user:${string}`;

/** One episode's length rule: the basis it is counted in, the length to aim
 *  for, and the bounds (0 = none). */
export interface LengthRule {
  char_count_basis: CharCountBasis;
  episode_char_target: number;
  episode_char_min: number;
  episode_char_max: number;
}

interface RuleFields {
  char_count_basis?: CharCountBasis;
  episode_char_target: number;
  episode_char_min?: number;
  episode_char_max?: number;
}

const bound = (v: number | undefined) => (v && v > 0 ? v : 0);

/** The length rule a work or a saved preset follows, with the defaults an
 *  older engine or an older preset leaves out. */
export function lengthRuleOf(project: RuleFields): LengthRule {
  return {
    char_count_basis: charCountBasisOf(project),
    episode_char_target: project.episode_char_target > 0 ? project.episode_char_target : DEFAULT_EPISODE_CHAR_TARGET,
    episode_char_min: bound(project.episode_char_min),
    episode_char_max: bound(project.episode_char_max),
  };
}

/** Whether a rule holds together (engine charcount.ValidEpisodeRange). */
export function validLengthRule(rule: LengthRule): boolean {
  const { episode_char_min: min, episode_char_target: target, episode_char_max: max } = rule;
  if (min < 0 || max < 0 || target <= 0) return false;
  if (max > 0 && min > max) return false;
  if (target < min) return false;
  if (max > 0 && target > max) return false;
  return true;
}

function sameRule(a: LengthRule, b: LengthRule): boolean {
  return (
    a.char_count_basis === b.char_count_basis &&
    a.episode_char_target === b.episode_char_target &&
    a.episode_char_min === b.episode_char_min &&
    a.episode_char_max === b.episode_char_max
  );
}

/** Every preset a picker offers, keyed, saved ones after the built-ins. */
export function allCharCountPresets(userPresets: readonly CharCountPreset[]): Array<{ key: CharCountPresetKey; rule: LengthRule }> {
  return [
    ...BUILTIN_CHAR_COUNT_PRESETS.map((p) => ({ key: `builtin:${p.id}` as const, rule: lengthRuleOf(p) })),
    ...userPresets.map((p) => ({ key: `user:${p.id}` as const, rule: lengthRuleOf(p) })),
  ];
}

/** The rule key names, or null if it names nothing on offer. */
export function ruleForPresetKey(key: string, userPresets: readonly CharCountPreset[]): LengthRule | null {
  const found = allCharCountPresets(userPresets).find((p) => p.key === key);
  return found ? { ...found.rule } : null;
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
    ...(rule.episode_char_min > 0 ? { episode_char_min: rule.episode_char_min } : {}),
    ...(rule.episode_char_max > 0 ? { episode_char_max: rule.episode_char_max } : {}),
  };
}

/** Matches the engine's settings.MaxCharCountPresetNameRunes. */
export const MAX_CHAR_COUNT_PRESET_NAME = 40;
