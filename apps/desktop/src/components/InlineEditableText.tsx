import { useCallback, useEffect, useRef, useState } from "react";

interface Props {
  value: string;
  ariaLabel: string;
  className?: string;
  onCommit: (value: string) => void | Promise<void>;
}

export function InlineEditableText({ value, ariaLabel, className, onCommit }: Props) {
  const [draft, setDraft] = useState(value);
  const committingRef = useRef(false);

  useEffect(() => {
    setDraft(value);
  }, [value]);

  const commit = useCallback(async () => {
    if (committingRef.current) return;
    const next = draft.trim();
    if (!next || next === value) {
      setDraft(value);
      return;
    }
    committingRef.current = true;
    try {
      await onCommit(next);
      setDraft(next);
    } catch {
      setDraft(value);
    } finally {
      committingRef.current = false;
    }
  }, [draft, onCommit, value]);

  return (
    <input
      aria-label={ariaLabel}
      className={`inline-edit-input${className ? ` ${className}` : ""}`}
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => { void commit(); }}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          void commit();
          e.currentTarget.blur();
        } else if (e.key === "Escape") {
          e.preventDefault();
          setDraft(value);
          e.currentTarget.blur();
        }
      }}
    />
  );
}
