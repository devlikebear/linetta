import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { notes as notesApi } from "../lib/rpc";
import type { Note } from "../lib/types";
import "./NotePopover.css";

interface Props {
  noteId: string;
  targetEl: HTMLElement | null;
  mode: "read" | "edit";
  onClose: () => void;
  onSaved?: (n: Note) => void;
  onDeleted?: (noteId: string) => void;
}

export function NotePopover({ noteId, targetEl, mode, onClose, onSaved, onDeleted }: Props) {
  const popoverRef = useRef<HTMLDivElement | null>(null);
  const [note, setNote] = useState<Note | null>(null);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);

  useEffect(() => {
    let cancelled = false;
    notesApi.get(noteId).then(
      (n) => {
        if (cancelled) return;
        setNote(n);
        setDraft(n.body);
      },
      () => { if (!cancelled) onClose(); },
    );
    return () => { cancelled = true; };
  }, [noteId, onClose]);

  useLayoutEffect(() => {
    if (!targetEl) return;
    const rect = targetEl.getBoundingClientRect();
    const popW = 280;
    let left = rect.left + window.scrollX;
    if (left + popW > window.innerWidth - 12) {
      left = window.innerWidth - popW - 12;
    }
    const top = rect.bottom + window.scrollY + 6;
    setPos({ left, top });
  }, [targetEl, note]);

  useEffect(() => {
    if (mode !== "edit") return;
    const handler = (e: MouseEvent) => {
      if (!popoverRef.current) return;
      if (popoverRef.current.contains(e.target as Node)) return;
      if (targetEl && targetEl.contains(e.target as Node)) return;
      onClose();
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
  }, [mode, onClose, targetEl]);

  if (!note || !pos) return null;

  const save = async () => {
    const body = draft.trim();
    if (!body) return;
    setBusy(true);
    try {
      const updated = await notesApi.update({ id: noteId, body });
      onSaved?.(updated);
      onClose();
    } finally { setBusy(false); }
  };

  const remove = async () => {
    setBusy(true);
    try {
      await notesApi.delete(noteId);
      onDeleted?.(noteId);
      onClose();
    } finally { setBusy(false); }
  };

  return (
    <div
      ref={popoverRef}
      className="note-popover"
      style={{ left: pos.left, top: pos.top }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      {mode === "read" ? (
        <div className="note-body">{note.body}</div>
      ) : (
        <>
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            autoFocus
            disabled={busy}
          />
          <div className="note-actions">
            <button className="danger" onClick={remove} disabled={busy}>삭제</button>
            <button className="primary" onClick={save} disabled={busy || !draft.trim()}>저장</button>
          </div>
        </>
      )}
    </div>
  );
}
