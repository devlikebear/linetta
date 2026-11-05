import type { CharCountBasis } from "./types";

export const DEFAULT_CHAR_COUNT_BASIS: CharCountBasis = "with_spaces";

/** The basis a work counts in; older engines leave it out. */
export function charCountBasisOf(project: { char_count_basis?: CharCountBasis } | null | undefined): CharCountBasis {
  return project?.char_count_basis === "without_spaces" ? "without_spaces" : DEFAULT_CHAR_COUNT_BASIS;
}

/** Go's unicode.IsSpace, so the editor counts what the engine stores. */
function isSpace(code: number): boolean {
  if (code <= 0xff) {
    return (code >= 0x09 && code <= 0x0d) || code === 0x20 || code === 0x85 || code === 0xa0;
  }
  return (
    code === 0x1680 ||
    (code >= 0x2000 && code <= 0x200a) ||
    code === 0x2028 ||
    code === 0x2029 ||
    code === 0x202f ||
    code === 0x205f ||
    code === 0x3000
  );
}

/** Counts the code points of text under basis (engine charcount.Text). */
export function countTextChars(text: string, basis: CharCountBasis): number {
  let n = 0;
  for (const ch of text) {
    if (basis === "without_spaces" && isSpace(ch.codePointAt(0) ?? 0)) continue;
    n += 1;
  }
  return n;
}

/** Counts a Tiptap doc's text nodes under basis (engine charcount.Doc). */
export function countDocChars(node: unknown, basis: CharCountBasis): number {
  if (!node || typeof node !== "object") return 0;
  const n = node as { type?: unknown; text?: unknown; content?: unknown };
  if (n.type === "text" && typeof n.text === "string") return countTextChars(n.text, basis);
  if (Array.isArray(n.content)) {
    let total = 0;
    for (const c of n.content) total += countDocChars(c, basis);
    return total;
  }
  return 0;
}

/** The i18n key naming basis ("공백 포함" / "공백 제외"). */
export function charCountBasisLabelKey(basis: CharCountBasis): "workspace.charCountWithSpaces" | "workspace.charCountWithoutSpaces" {
  return basis === "without_spaces" ? "workspace.charCountWithoutSpaces" : "workspace.charCountWithSpaces";
}
