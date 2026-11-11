/** X's per-post weight limit. */
export const X_LIMIT = 280;
/** Every URL counts as this many characters, whatever its length. */
export const X_URL_WEIGHT = 23;

const URL_RE = /https?:\/\/[^\s]+/g;

// Code point ranges X counts as weight 1; everything else (Hangul, CJK,
// Japanese, fullwidth forms, emoji) counts 2. Mirrors twitter-text's config.
const LIGHT_RANGES: [number, number][] = [
  [0, 4351],
  [8192, 8205],
  [8208, 8223],
  [8242, 8247],
];

function weightOf(cp: number): number {
  for (const [lo, hi] of LIGHT_RANGES) if (cp >= lo && cp <= hi) return 1;
  return 2;
}

/** X's weighted length of `text`: each URL is 23, other characters 1 or 2. */
export function xLength(text: string): number {
  let total = 0;
  const rest = text.replace(URL_RE, () => {
    total += X_URL_WEIGHT;
    return "";
  });
  for (const ch of rest) total += weightOf(ch.codePointAt(0) ?? 0);
  return total;
}
