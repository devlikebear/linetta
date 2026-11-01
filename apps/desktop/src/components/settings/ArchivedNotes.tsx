import { useCallback, useEffect, useRef, useState } from "react";

import { useEngineEvent } from "../../hooks/useEngineEvent";
import { useI18n } from "../../lib/i18n";
import { memory } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { ArchivedNote, MemoryChangedPayload } from "../../lib/types";

/** The lines that left one work's notes.
 *
 *  They are not in any prompt until something searches for them, which is
 *  exactly why the writer has to be able to see them here: this is the only
 *  place they are visible at all. Collapsed by default, because a long work
 *  gathers many and the notes above are what the pane is for.
 *
 *  The list follows the same ordering rule as MemorySection: a reply lands only
 *  while its ticket is still the newest, so a slow answer for the previous work
 *  cannot put that work's lines under this one. */
export function ArchivedNotes({ workId }: { workId: string }) {
  const { t } = useI18n();
  const [notes, setNotes] = useState<ArchivedNote[]>([]);
  const [error, setError] = useState<unknown>(null);
  const ticket = useRef(0);

  const load = useCallback(() => {
    const mine = (ticket.current += 1);
    memory.archive.list(workId).then(
      (next) => {
        if (ticket.current === mine) setNotes(next);
      },
      (e) => {
        if (ticket.current === mine) setError(e);
      },
    );
  }, [workId]);

  useEffect(() => {
    setError(null);
    load();
    return () => {
      ticket.current += 1;
    };
  }, [load]);

  useEngineEvent<MemoryChangedPayload>("memory-changed", (payload) => {
    if (payload?.scope === "work_notes" && (payload.project_id ?? "") === workId) load();
  });

  if (!workId || (notes.length === 0 && error == null)) return null;

  const act = (run: Promise<unknown>) => {
    setError(null);
    run.then(load, setError);
  };

  return (
    <details className="modal-field" data-testid="memory-archive">
      <summary>{t("settings.memory.archive.title", { count: String(notes.length) })}</summary>
      <p className="sd">{t("settings.memory.archive.help")}</p>
      <ul className="vault-items">
        {notes.map((note) => (
          <li key={note.id} data-testid="memory-archive-line">
            <span>{note.body}</span>{" "}
            <button
              type="button"
              className="btn ghost sm"
              onClick={() => act(memory.archive.restore(workId, note.id))}
            >
              {t("settings.memory.archive.restore")}
            </button>{" "}
            <button
              type="button"
              className="btn ghost sm"
              onClick={() => act(memory.archive.remove(workId, note.id))}
            >
              {t("settings.memory.archive.delete")}
            </button>
          </li>
        ))}
      </ul>
      {error != null && (
        <p className="sd" role="alert" data-testid="memory-archive-error">
          {rpcErrorMessage(error, t)}
        </p>
      )}
    </details>
  );
}
