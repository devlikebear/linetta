import type { LengthRule } from "./charCountPresets";

/** Where an episode's length stands against the work's rule.
 *  - "short": under the minimum
 *  - "long": over the maximum
 *  - "ok": within the bounds the work sets
 *  - "progress": the work sets no minimum, so the episode is only on its way
 *    to the target (what the gauge showed before bounds existed) */
export type EpisodeLengthState = "short" | "ok" | "long" | "progress";

export interface EpisodeLength {
  state: EpisodeLengthState;
  /** Characters short of the minimum (negative) or past the maximum
   *  (positive); 0 otherwise. */
  delta: number;
  /** The length the gauge fills toward: the minimum when there is one. */
  goal: number;
  /** Whether the episode reached its goal (the gauge's complete colour). */
  complete: boolean;
}

type Bounds = Pick<LengthRule, "episode_char_min" | "episode_char_target" | "episode_char_max">;

/** Judges an episode of chars characters (engine episode length rules keep
 *  the same boundaries: reaching the minimum or the maximum exactly is ok). */
export function episodeLength(chars: number, rule: Bounds): EpisodeLength {
  const min = Math.max(0, rule.episode_char_min);
  const max = Math.max(0, rule.episode_char_max);
  const target = Math.max(1, rule.episode_char_target);
  if (max > 0 && chars > max) {
    return { state: "long", delta: chars - max, goal: min > 0 ? min : target, complete: true };
  }
  if (min > 0) {
    return chars < min
      ? { state: "short", delta: chars - min, goal: min, complete: false }
      : { state: "ok", delta: 0, goal: min, complete: true };
  }
  return { state: "progress", delta: 0, goal: target, complete: chars >= target };
}
