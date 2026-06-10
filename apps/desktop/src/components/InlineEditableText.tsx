import { useCallback, useEffect, useRef, useState } from "react";

interface Props {
  value: string;
  ariaLabel: string;
  className?: string;
  autoFocus?: boolean;
  allowEmpty?: boolean;
  placeholder?: string;
  onCommit: (value: string) => void | Promise<void>;
  onCancel?: () => void;
}

export function InlineEditableText({ value, ariaLabel, className, autoFocus, allowEmpty = false, placeholder, onCommit, onCancel }: Props) {
  const [draft, setDraft] = useState(value);
  const inputRef = useRef<HTMLInputElement>(null);
  const committingRef = useRef(false);
  const cancelingRef = useRef(false);

  useEffect(() => {
    setDraft(value);
  }, [value]);

  useEffect(() => {
    if (!autoFocus) return;
    inputRef.current?.focus();
    inputRef.current?.select();
  }, [autoFocus]);

  const commit = useCallback(async () => {
    if (cancelingRef.current) {
      cancelingRef.current = false;
      return;
    }
    if (committingRef.current) return;
    const next = draft.trim();
    if ((!allowEmpty && !next) || next === value) {
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
  }, [allowEmpty, draft, onCommit, value]);

  return (
    <input
      ref={inputRef}
      aria-label={ariaLabel}
      className={`inline-edit-input${className ? ` ${className}` : ""}`}
      value={draft}
      placeholder={placeholder}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => { void commit(); }}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          void commit();
          e.currentTarget.blur();
        } else if (e.key === "Escape") {
          e.preventDefault();
          cancelingRef.current = true;
          setDraft(value);
          onCancel?.();
          e.currentTarget.blur();
        }
      }}
    />
  );
}
