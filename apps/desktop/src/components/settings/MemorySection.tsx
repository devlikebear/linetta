import { useEffect, useState } from "react";

import { useEngineEvent } from "../../hooks/useEngineEvent";
import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { memory, projects as projectsApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { MemoryChangedPayload, MemoryDocument, MemoryState, Project } from "../../lib/types";

/** The two curated memories, as a writer reads and edits them.
 *
 *  Everything else in Settings is a knob: a path, a port, a number. This pane
 *  is prose the writer and the agent both write, which makes it the one place
 *  in Settings where a background event can destroy something a person typed.
 *  Two rules follow, and they are what the component is shaped around:
 *
 *  - The textarea holds a local DRAFT and commits on blur. Committing per
 *    keystroke would be one RPC per character and one memory.changed bouncing
 *    back at this pane for each of them.
 *  - A dirty draft is the writer's until they save it. When an agent writes
 *    underneath, the pane refetches, refreshes only the clean textarea, and
 *    SAYS that the other one is now behind. Silently replacing unsent text is
 *    the one failure here nothing can undo.
 */

type Scope = MemoryDocument["scope"];

type ByScope<T> = Record<Scope, T>;

const bodies = (state: MemoryState): ByScope<string> => ({
  writer_profile: state.writer_profile.body,
  work_notes: state.work_notes.body,
});

const NONE_DIRTY: ByScope<boolean> = { writer_profile: false, work_notes: false };

export function MemorySection() {
  const { t } = useI18n();
  const [works, setWorks] = useState<Project[]>([]);
  // null until projects.list answers. The distinction matters: "" is a real,
  // supported state (no work in the library yet — memory.get answers it with
  // the profile and an empty note carrying the right budget), and starting at
  // "" instead would fire a throwaway fetch whose late reply could land on top
  // of the first work's and reset a draft mid-keystroke.
  const [workId, setWorkId] = useState<string | null>(null);
  const [state, setState] = useState<MemoryState | null>(null);
  // What the engine last confirmed, against which a draft is dirty-or-not.
  const [saved, setSaved] = useState<ByScope<string>>({ writer_profile: "", work_notes: "" });
  const [drafts, setDrafts] = useState<ByScope<string>>({ writer_profile: "", work_notes: "" });
  const [behind, setBehind] = useState<ByScope<boolean>>(NONE_DIRTY);
  // The raw failure is kept and translated at render time, so switching
  // language redraws the message instead of leaving a stale sentence up.
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    projectsApi.list({}).then(
      (list) => {
        setWorks(list);
        setWorkId(list[0]?.id ?? "");
      },
      // A library that cannot be listed still has a writer profile, so the
      // pane opens on the global memory rather than on an error.
      () => {
        setWorks([]);
        setWorkId("");
      },
    );
  }, []);

  useEffect(() => {
    if (workId === null) return;
    let cancelled = false;
    memory.get(workId).then(
      (next) => {
        if (cancelled) return;
        setState(next);
        setSaved(bodies(next));
        // A work switch replaces the note wholesale, drafts included: keeping
        // the previous work's unsent text in the box would let the next blur
        // save it onto the wrong book.
        setDrafts(bodies(next));
        setBehind(NONE_DIRTY);
        setError(null);
      },
      (e) => {
        if (!cancelled) setError(e);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [workId]);

  const guard = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const commit = (scope: Scope) => {
    const body = drafts[scope];
    // Unchanged is not a save. Writing anyway would restamp updated_at and
    // fire memory.changed at every other window for a document nobody edited.
    if (body === saved[scope]) return;
    void guard(async () => {
      const doc = await memory.set(scope, scope === "work_notes" ? (workId ?? "") : "", body);
      // The engine trims and normalises, so the confirmed body — not the
      // draft — is what both the box and the dirty comparison now hold.
      setSaved((s) => ({ ...s, [scope]: doc.body }));
      setDrafts((d) => ({ ...d, [scope]: doc.body }));
      setState((s) => (s ? { ...s, [scope]: doc } : s));
      setBehind((b) => ({ ...b, [scope]: false }));
    });
    // A refused save (over budget, an invisible character, a heading) leaves
    // the draft exactly as typed. The writer's text is not the server's to
    // discard, and the fix is one keystroke away.
  };

  useEngineEvent<MemoryChangedPayload>("memory-changed", (payload) => {
    if (workId === null) return;
    // Read dirtiness NOW, before the refetch: the answer must describe the
    // moment the agent wrote, not whatever the boxes hold when the reply
    // eventually lands.
    const dirty: ByScope<boolean> = {
      writer_profile: drafts.writer_profile !== saved.writer_profile,
      work_notes: drafts.work_notes !== saved.work_notes,
    };
    const changed = payload?.scope;
    // Our own save round-trips through this event; it is not news.
    const fromElsewhere = payload?.source !== "writer";
    memory.get(workId).then(
      (next) => {
        setState(next);
        setSaved(bodies(next));
        setDrafts((d) => ({
          writer_profile: dirty.writer_profile ? d.writer_profile : next.writer_profile.body,
          work_notes: dirty.work_notes ? d.work_notes : next.work_notes.body,
        }));
        setBehind((b) => ({
          ...b,
          ...(changed && dirty[changed] && fromElsewhere ? { [changed]: true } : {}),
        }));
      },
      (e) => setError(e),
    );
  });

  const doc = (scope: Scope): MemoryDocument | null => (state ? state[scope] : null);
  // The count follows the draft, in runes — the unit the engine's budget is
  // in, and the reason `.length` will not do for Korean prose with emoji in it.
  const used = (scope: Scope) => [...drafts[scope]].length;

  const noWork = workId === "";

  const field = (scope: Scope, testId: string, label: MessageKey, help: MessageKey) => {
    const d = doc(scope);
    return (
      <div className="modal-field">
        <label htmlFor={testId}>{t(label)}</label>
        <textarea
          id={testId}
          data-testid={testId}
          rows={6}
          value={drafts[scope]}
          placeholder={t("settings.memory.empty")}
          disabled={scope === "work_notes" && noWork}
          onChange={(e) => setDrafts((prev) => ({ ...prev, [scope]: e.target.value }))}
          onBlur={() => commit(scope)}
        />
        <p className="sd">{t(help)}</p>
        {d && (
          <p className="sd" data-testid={`${testId}-count`}>
            {t("settings.memory.remaining", { used: String(used(scope)), budget: String(d.chars_budget) })}
          </p>
        )}
        {behind[scope] && (
          <p className="sd" role="alert" data-testid={`${testId}-changed`}>
            {t("settings.memory.changedElsewhere")}
          </p>
        )}
      </div>
    );
  };

  return (
    <section className="settings-section" id="memory-settings" data-testid="memory-section">
      <h3>{t("settings.memory.title")}</h3>
      <p className="sd">{t("settings.memory.description")}</p>

      {field(
        "writer_profile",
        "memory-writer-profile",
        "settings.memory.writerProfile",
        "settings.memory.writerProfile.help",
      )}

      {/* The note belongs to one book, so the book is named right above it
          rather than inferred from whatever the writer last had open. */}
      <div className="modal-field">
        <label htmlFor="memory-work">{t("settings.memory.work")}</label>
        <select
          id="memory-work"
          data-testid="memory-work"
          value={workId ?? ""}
          disabled={busy || works.length === 0}
          onChange={(e) => setWorkId(e.target.value)}
        >
          {works.map((w) => (
            <option key={w.id} value={w.id}>
              {w.title}
            </option>
          ))}
        </select>
      </div>

      {field(
        "work_notes",
        "memory-work-notes",
        "settings.memory.workNotes",
        "settings.memory.workNotes.help",
      )}

      {error != null && (
        <p className="sd" role="alert" data-testid="memory-error">
          {/* Over budget, an invisible character, a heading: every refusal the
              engine sends names something the writer can fix from this box. */}
          {rpcErrorMessage(error, t)}
        </p>
      )}
    </section>
  );
}
