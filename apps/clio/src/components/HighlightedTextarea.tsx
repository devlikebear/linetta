import { forwardRef, useImperativeHandle, useRef } from "react";
import type { Severity } from "../lib/types";
import type { UnitRange } from "../lib/offsets";
import "./HighlightedTextarea.css";

export interface HighlightRange extends UnitRange {
  severity: Severity;
}

export interface Segment {
  text: string;
  severity?: Severity;
}

const RANK: Record<Severity, number> = { strong: 3, medium: 2, weak: 1 };

/** Splits `text` at every range boundary; overlapping ranges show the strongest severity. */
export function buildSegments(text: string, ranges: HighlightRange[]): Segment[] {
  const cuts = new Set<number>([0, text.length]);
  for (const r of ranges) {
    cuts.add(Math.max(0, Math.min(r.start, text.length)));
    cuts.add(Math.max(0, Math.min(r.end, text.length)));
  }
  const points = [...cuts].sort((a, b) => a - b);
  const out: Segment[] = [];
  for (let i = 0; i < points.length - 1; i++) {
    const [a, b] = [points[i], points[i + 1]];
    let best: Severity | undefined;
    for (const r of ranges) {
      if (r.start <= a && r.end >= b && (!best || RANK[r.severity] > RANK[best])) best = r.severity;
    }
    out.push({ text: text.slice(a, b), severity: best });
  }
  return out;
}

export interface HighlightedTextareaHandle {
  /** Focuses the textarea and selects [start, end) in UTF-16 indices. */
  selectRange: (start: number, end: number) => void;
}

interface Props {
  value: string;
  onChange: (value: string) => void;
  /** Ranges in UTF-16 indices (convert engine offsets with lib/offsets first). */
  ranges: HighlightRange[];
  id?: string;
  placeholder?: string;
  "aria-label"?: string;
}

/**
 * A textarea over a mirror div with identical font, padding and wrapping. The
 * mirror draws the text transparent with coloured underlines; the textarea
 * sits on top and takes all input.
 */
export const HighlightedTextarea = forwardRef<HighlightedTextareaHandle, Props>(function HighlightedTextarea(
  { value, onChange, ranges, id, placeholder, "aria-label": ariaLabel },
  ref,
) {
  const textRef = useRef<HTMLTextAreaElement>(null);
  const mirrorRef = useRef<HTMLDivElement>(null);

  useImperativeHandle(ref, () => ({
    selectRange(start, end) {
      const el = textRef.current;
      if (!el) return;
      el.focus();
      el.setSelectionRange(start, end);
    },
  }));

  const syncScroll = () => {
    const [ta, mirror] = [textRef.current, mirrorRef.current];
    if (ta && mirror) {
      mirror.scrollTop = ta.scrollTop;
      mirror.scrollLeft = ta.scrollLeft;
    }
  };

  const segments = buildSegments(value, ranges);

  return (
    <div className="hl-wrap">
      <div className="hl-field hl-mirror" ref={mirrorRef} aria-hidden="true" data-testid="hl-mirror">
        {segments.map((s, i) =>
          s.severity ? (
            <span key={i} className={`hl hl-${s.severity}`} data-severity={s.severity}>
              {s.text}
            </span>
          ) : (
            <span key={i}>{s.text}</span>
          ),
        )}
        {/* A trailing newline needs a character after it to take up its line. */}
        {"​"}
      </div>
      <textarea
        ref={textRef}
        id={id}
        className="hl-field hl-input"
        value={value}
        placeholder={placeholder}
        aria-label={ariaLabel}
        spellCheck={false}
        onChange={(e) => onChange(e.target.value)}
        onScroll={syncScroll}
      />
    </div>
  );
});
