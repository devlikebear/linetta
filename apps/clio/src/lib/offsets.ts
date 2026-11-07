import type { Finding } from "./types";

/**
 * The engine reports positions as rune (code point) offsets; DOM text APIs use
 * UTF-16 code unit indices. They differ whenever the text has a character
 * outside the BMP (most emoji), which takes two UTF-16 units.
 */

/** utf16Index[i] is the UTF-16 index of code point i; the last entry is text.length. */
export function codePointIndex(text: string): number[] {
  const idx: number[] = [];
  let unit = 0;
  for (const ch of text) {
    idx.push(unit);
    unit += ch.length;
  }
  idx.push(unit);
  return idx;
}

/** Converts a code point offset to a UTF-16 index, clamped to the text. */
export function cpToUtf16(text: string, offset: number): number {
  const idx = codePointIndex(text);
  return idx[Math.max(0, Math.min(offset, idx.length - 1))];
}

export interface UnitRange {
  start: number;
  end: number;
}

/** A finding is whole-post (list only, no underline) when both offsets are 0. */
export function isRanged(f: Pick<Finding, "start" | "end">): boolean {
  return !(f.start === 0 && f.end === 0) && f.end > f.start;
}

/** The finding's range as UTF-16 indices into `text`, or null for a whole-post finding. */
export function findingRange(text: string, f: Pick<Finding, "start" | "end">): UnitRange | null {
  if (!isRanged(f)) return null;
  const idx = codePointIndex(text);
  const last = idx.length - 1;
  const start = idx[Math.min(Math.max(f.start, 0), last)];
  const end = idx[Math.min(Math.max(f.end, 0), last)];
  return end > start ? { start, end } : null;
}
