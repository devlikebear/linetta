import type { Node as PMNode } from "@tiptap/pm/model";

// The engine numbers a scene's paragraphs one way (style.Paragraphs): render
// the doc to plain text (export.DocToPlainText), split on blank lines, drop
// the empty ones. Continuity citations, linetta_read_scene numbered=true and
// search hits all count with it. This file is the editor's copy of those
// rules, built over the ProseMirror doc so each paragraph keeps the document
// positions it came from. It mirrors the Go code step by step on purpose;
// engine/internal/style/testdata/paragraphs holds both to the same fixtures.

/** One numbered paragraph and where it sits in the editor's document. */
export interface ParagraphRange {
  /** 1-based, as the engine numbers it. */
  n: number;
  /** Document position of the paragraph's first visible character (a
   *  paragraph can keep a newline at its edge; the span leaves it out). */
  from: number;
  /** Document position just past its last visible character. */
  to: number;
  /** The paragraph's plain text, as the engine sees it. */
  text: string;
  /** For each UTF-16 unit of `text`, the document span it was drawn from
   *  (a mention's label spans the whole mention). */
  starts: number[];
  ends: number[];
}

/** A run of plain text whose every character knows its document span. */
interface Plain {
  text: string;
  starts: number[];
  ends: number[];
}

function emptyPlain(): Plain {
  return { text: "", starts: [], ends: [] };
}

function push(out: Plain, text: string, start: number, end: number) {
  for (let i = 0; i < text.length; i++) {
    out.text += text[i];
    out.starts.push(start);
    out.ends.push(end);
  }
}

// plainBlocks in export/text.go. A node's JSON carries "content" only when it
// has children, which is the test the Go side makes.
function plainBlocks(node: PMNode, pos: number, out: Plain[]) {
  const name = node.type.name;
  // Children of the doc start at 0; of any other node, one past its opening.
  const inner = name === "doc" ? 0 : pos + 1;
  switch (name) {
    case "paragraph":
    case "heading": {
      const block = emptyPlain();
      plainInline(node, inner, block);
      out.push(block);
      return;
    }
    case "codeBlock":
      if (node.childCount > 0) {
        const block = emptyPlain();
        plainInline(node, inner, block);
        out.push(block);
      }
      return;
    case "horizontalRule":
      return;
    default:
      node.forEach((child, offset) => plainBlocks(child, inner + offset, out));
  }
}

// plainInline in export/text.go: text as written, a mention as its label, a
// hard break as a newline, any other atom as nothing.
function plainInline(node: PMNode, inner: number, out: Plain) {
  node.forEach((child, offset) => {
    const pos = inner + offset;
    const name = child.type.name;
    if (child.isText) {
      const text = child.text ?? "";
      for (let i = 0; i < text.length; i++) push(out, text[i], pos + i, pos + i + 1);
      return;
    }
    if (name === "mention") {
      const label = child.attrs.label;
      if (typeof label === "string") push(out, label, pos, pos + child.nodeSize);
      return;
    }
    if (name === "hardBreak") {
      push(out, "\n", pos, pos + child.nodeSize);
      return;
    }
    plainInline(child, pos + 1, out);
  });
}

// unicode.IsSpace, which strings.TrimSpace and strings.Fields use. JavaScript's
// \s differs at the edges (it takes U+FEFF and leaves U+0085), so the engine's
// set is spelled out.
const GO_SPACE = /^[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]$/;

function isGoSpace(ch: string): boolean {
  return GO_SPACE.test(ch);
}

/** Numbers the document's paragraphs the way the engine does. */
export function paragraphRanges(doc: PMNode): ParagraphRange[] {
  const blocks: Plain[] = [];
  plainBlocks(doc, 0, blocks);

  // strings.Join(blocks, "\n\n"). A separator newline can end up at the
  // edge of a paragraph (a block that ends in a hard break leaves "\n" + the
  // separator, and the split takes the first two), so it gets the position
  // between the two blocks.
  const all = emptyPlain();
  blocks.forEach((block, i) => {
    if (i > 0) {
      const at = all.ends.length > 0 ? all.ends[all.ends.length - 1] : (block.starts[0] ?? 0);
      push(all, "\n\n", at, at);
    }
    all.text += block.text;
    // A loop, not push(...spread): a long code block would overflow the
    // argument limit.
    for (let k = 0; k < block.starts.length; k++) {
      all.starts.push(block.starts[k]);
      all.ends.push(block.ends[k]);
    }
  });

  // strings.Trim(text, "\n\r").
  let lo = 0;
  let hi = all.text.length;
  while (lo < hi && (all.text[lo] === "\n" || all.text[lo] === "\r")) lo++;
  while (hi > lo && (all.text[hi - 1] === "\n" || all.text[hi - 1] === "\r")) hi--;

  // strings.Split(text, "\n\n"), skipping the pieces TrimSpace empties.
  const out: ParagraphRange[] = [];
  let start = lo;
  while (start <= hi) {
    let cut = all.text.indexOf("\n\n", start);
    if (cut < 0 || cut + 2 > hi) cut = hi;
    const text = all.text.slice(start, cut);
    // The visible span: a paragraph can keep a newline at its edge.
    let first = start;
    let last = cut - 1;
    while (first <= last && isGoSpace(all.text[first])) first++;
    while (last >= first && isGoSpace(all.text[last])) last--;
    if (first <= last) {
      out.push({
        n: out.length + 1,
        from: all.starts[first],
        to: all.ends[last],
        text,
        starts: all.starts.slice(start, cut),
        ends: all.ends.slice(start, cut),
      });
    }
    if (cut === hi) break;
    start = cut + 2;
  }
  return out;
}

/** Where a cited passage was found in the document. */
export interface PassageMatch {
  from: number;
  to: number;
}

/**
 * Finds `quote` inside paragraphs [from, to] the way evidence.VerifyQuote
 * accepts it: the paragraphs joined by newlines, both sides NFC-normalized and
 * their whitespace runs folded to one space. Marks and mentions split a
 * sentence into several text nodes; this searches across them. Returns null
 * when the range is out of bounds or the quote is not there.
 */
export function findQuoteInParagraphs(
  ranges: ParagraphRange[],
  from: number,
  to: number,
  quote: string,
): PassageMatch | null {
  if (from < 1 || to < from || to > ranges.length) return null;
  const wanted = normalizeQuote(quote);
  if (!wanted) return null;

  // The haystack, one entry per normalized character, each pointing back at
  // the paragraph characters it came from.
  let text = "";
  const starts: number[] = [];
  const ends: number[] = [];
  let pendingSpace: { start: number; end: number } | null = null;
  for (let p = from - 1; p < to; p++) {
    const para = ranges[p];
    if (p > from - 1) pendingSpace ??= { start: ranges[p - 1].to, end: para.from };
    for (const unit of normalizationUnits(para)) {
      if (unit.space) {
        pendingSpace ??= { start: unit.start, end: unit.end };
        continue;
      }
      if (pendingSpace && text.length > 0) {
        text += " ";
        starts.push(pendingSpace.start);
        ends.push(pendingSpace.end);
      }
      pendingSpace = null;
      for (const ch of unit.text) {
        text += ch;
        // A composed character can be several UTF-16 units; each points at
        // the whole unit it came from.
        for (let k = 0; k < ch.length; k++) {
          starts.push(unit.start);
          ends.push(unit.end);
        }
      }
    }
  }
  const at = text.indexOf(wanted);
  if (at < 0) return null;
  return { from: starts[at], to: ends[at + wanted.length - 1] };
}

/** evidence.Normalize: NFC, then whitespace runs folded to one space. */
export function normalizeQuote(s: string): string {
  const out: string[] = [];
  let word = "";
  for (const ch of s.normalize("NFC")) {
    if (isGoSpace(ch)) {
      if (word) out.push(word);
      word = "";
    } else {
      word += ch;
    }
  }
  if (word) out.push(word);
  return out.join(" ");
}

interface Unit {
  text: string;
  space: boolean;
  start: number;
  end: number;
}

// A combining mark, or a Hangul vowel or final consonant jamo, composes with
// what comes before it under NFC; anything else starts a new character.
const JOINS_PREVIOUS = /^[\p{M}\u1160-\u11ff\ud7b0-\ud7ff]$/u;

// Splits a paragraph into pieces that NFC-normalize on their own, each with
// the document span it covers, so a match in normalized text maps back. A
// space is always a piece of its own: strings.Fields splits on it before
// anything could compose with it.
function normalizationUnits(para: ParagraphRange): Unit[] {
  const out: Unit[] = [];
  let lo = 0;
  let hi = 0;
  const flush = () => {
    if (hi === lo) return;
    const text = para.text.slice(lo, hi).normalize("NFC");
    out.push({ text, space: isGoSpace(text), start: para.starts[lo], end: para.ends[hi - 1] });
    lo = hi;
  };
  for (const ch of para.text) {
    const startsNew = hi === lo || isGoSpace(ch) || isGoSpace(para.text.slice(lo, hi)) || !JOINS_PREVIOUS.test(ch);
    if (startsNew) flush();
    hi += ch.length;
  }
  flush();
  return out;
}
