import { useCallback, useEffect, useRef, useState } from "react";

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
 *  Three rules follow, and they are what the component is shaped around:
 *
 *  - The textarea holds a local DRAFT and commits on blur. Committing per
 *    keystroke would be one RPC per character and one memory.changed bouncing
 *    back at this pane for each of them.
 *  - A dirty draft is the writer's until they save it. When an agent writes
 *    underneath, the pane refetches, refreshes only the clean textarea, and
 *    SAYS that the other one is now behind. Silently replacing unsent text is
 *    the one failure here nothing can undo.
 *  - Every reply that writes shared state is ORDERED. See `seq` below: three
 *    different callers ask the engine for these documents, replies arrive in
 *    socket order rather than request order, and an out-of-order one does not
 *    merely flicker — it puts one book's notes in the box while the picker
 *    names another, and the next blur saves them there.
 */

type Scope = MemoryDocument["scope"];

type ByScope<T> = Record<Scope, T>;

const bodies = (state: MemoryState): ByScope<string> => ({
  writer_profile: state.writer_profile.body,
  work_notes: state.work_notes.body,
});

const NONE_DIRTY: ByScope<boolean> = { writer_profile: false, work_notes: false };
const NO_ERROR: ByScope<unknown> = { writer_profile: null, work_notes: null };

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
  // The raw failures are kept and translated at render time, so switching
  // language redraws the message instead of leaving a stale sentence up.
  // A refused save belongs to the document it was refused for — one shared
  // line at the foot of the pane reads as "Settings is broken" when what
  // actually happened is "the work note is 12 characters too long".
  const [saveError, setSaveError] = useState<ByScope<unknown>>(NO_ERROR);
  // Pane-level, and only ever a failure to LOAD: nothing on screen is
  // trustworthy after one, so it is not about either box in particular.
  const [error, setError] = useState<unknown>(null);
  // A count, not a flag: two blurs can be in flight at once (leave one box,
  // type in the other, leave that), and the first to answer must not unlock
  // the work picker while the second is still travelling.
  const [inflight, setInflight] = useState(0);
  const busy = inflight > 0;

  /** Ordering — the invariant the rest of this component rests on.
   *
   *  Three callers write these documents from an await: the load effect, the
   *  memory-changed refetch, and memory.set's confirmation. Replies come back
   *  in whatever order the socket produces them, so an older one can land on
   *  top of a newer one. That is the interleaving ProviderSection.tsx:63,240
   *  was rewritten around after several rounds of trading bugs, and here it is
   *  worse than an un-ticked checkbox: an agent writes while work-1 is on
   *  screen, the writer moves the picker to work-2, work-2 answers first, and
   *  then work-1's stale reply lands — the box now holds work-1's notes under
   *  a picker reading work-2, and the next blur saves them onto work-2.
   *
   *  So: a ticket is claimed BEFORE the await and re-checked after, and a
   *  reply lands only while its ticket is still the newest for that scope.
   *  Per scope, not per pane, because the two documents move independently: a
   *  work_notes refetch has no business discarding a writer_profile save's
   *  confirmation, and a refetch caused by one scope's event only ever
   *  applies to that scope. */
  const seq = useRef<ByScope<number>>({ writer_profile: 0, work_notes: 0 });
  const claim = useCallback((scope: Scope) => (seq.current[scope] += 1), []);
  const claimBoth = useCallback(
    (): ByScope<number> => ({
      writer_profile: claim("writer_profile"),
      work_notes: claim("work_notes"),
    }),
    [claim],
  );
  const newest = useCallback((scope: Scope, ticket: number) => seq.current[scope] === ticket, []);

  /** memory.changed says source "writer" for a save made by a person at a
   *  keyboard — ours and the other window's alike (handlers/memory.go:86-89).
   *  Only ours is not news; the other window's is exactly the elsewhere-edit
   *  the notice exists for. One token per save we send, consumed by the first
   *  writer-sourced event for that scope, tells the two apart. A refused save
   *  hands its token back, because the engine emits nothing for one. */
  const ownSaves = useRef<ByScope<number>>({ writer_profile: 0, work_notes: 0 });

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
    // One memory.get answers both documents, so it claims both tickets.
    const ticket = claimBoth();
    memory.get(workId).then(
      (next) => {
        const fresh: ByScope<boolean> = {
          writer_profile: newest("writer_profile", ticket.writer_profile),
          work_notes: newest("work_notes", ticket.work_notes),
        };
        if (!fresh.writer_profile && !fresh.work_notes) return;
        // Take only the scopes this reply is still the newest answer for.
        const pick = <T,>(prev: ByScope<T>, from: ByScope<T>): ByScope<T> => ({
          writer_profile: fresh.writer_profile ? from.writer_profile : prev.writer_profile,
          work_notes: fresh.work_notes ? from.work_notes : prev.work_notes,
        });
        const body = bodies(next);
        setState((s) => (s ? pick(s, next) : next));
        setSaved((s) => pick(s, body));
        // A work switch replaces the note wholesale, drafts included: keeping
        // the previous work's unsent text in the box would let the next blur
        // save it onto the wrong book.
        setDrafts((d) => pick(d, body));
        setBehind((b) => pick(b, NONE_DIRTY));
        setError(null);
      },
      (e) => {
        if (newest("writer_profile", ticket.writer_profile) || newest("work_notes", ticket.work_notes)) {
          setError(e);
        }
      },
    );
    return () => {
      // Leaving this work (or the pane) invalidates whatever is still in
      // flight for it. Cancellation here IS a newer ticket — there is one
      // mechanism, not a `cancelled` flag beside it that a later caller could
      // forget to add.
      claimBoth();
    };
  }, [workId, claimBoth, newest]);

  const commit = (scope: Scope) => {
    const body = drafts[scope];
    // Unchanged is not a save. Writing anyway would restamp updated_at and
    // fire memory.changed at every other window for a document nobody edited.
    if (body === saved[scope]) return;
    const ticket = claim(scope);
    ownSaves.current[scope] += 1;
    setSaveError((s) => ({ ...s, [scope]: null }));
    setInflight((n) => n + 1);
    void memory.set(scope, scope === "work_notes" ? (workId ?? "") : "", body).then(
      (doc) => {
        setInflight((n) => n - 1);
        if (!newest(scope, ticket)) return;
        // The engine stores the body verbatim — Repo.Save trims the project
        // id and nothing else (agentmemory.go:255-291) — so `doc.body` is the
        // text that was sent, now confirmed. It is what the dirty comparison
        // must hold from here on.
        setSaved((s) => ({ ...s, [scope]: doc.body }));
        // The box, though, may have moved on: a writer who keeps typing after
        // the blur is still typing while this reply travels. Only the draft
        // that was SENT may be replaced by its own confirmation; anything
        // newer is unsent text, and unsent text is the writer's.
        setDrafts((d) => (d[scope] === body ? { ...d, [scope]: doc.body } : d));
        setState((s) => (s ? { ...s, [scope]: doc } : s));
        setBehind((b) => ({ ...b, [scope]: false }));
      },
      (e) => {
        setInflight((n) => n - 1);
        ownSaves.current[scope] -= 1;
        if (!newest(scope, ticket)) return;
        // A refused save (over budget, an invisible character, a heading)
        // leaves the draft exactly as typed. The writer's text is not the
        // server's to discard, and the fix is one keystroke away.
        setSaveError((s) => ({ ...s, [scope]: e }));
      },
    );
  };

  useEngineEvent<MemoryChangedPayload>("memory-changed", (payload) => {
    if (workId === null) return;
    const changed = payload?.scope;
    if (changed !== "writer_profile" && changed !== "work_notes") return;
    // The profile is global; a note belongs to one book. An agent editing
    // work-2's note says nothing about the work-1 note on screen — refetching
    // for it would be a wasted round trip, and raising "an agent just changed
    // this, saving will overwrite it" over a draft nothing touched is a false
    // claim about the writer's own text, on a notice with no dismiss.
    if (changed === "work_notes" && (payload.project_id ?? "") !== workId) return;
    // Read dirtiness NOW, before the refetch: the answer must describe the
    // moment the agent wrote, not whatever the box holds when the reply
    // eventually lands.
    const dirty = drafts[changed] !== saved[changed];
    let fromElsewhere = true;
    if (payload.source === "writer" && ownSaves.current[changed] > 0) {
      ownSaves.current[changed] -= 1;
      fromElsewhere = false;
    }
    // This event is about one document, so only that one is refetched into.
    const ticket = claim(changed);
    memory.get(workId).then(
      (next) => {
        if (!newest(changed, ticket)) return;
        setState((s) => (s ? { ...s, [changed]: next[changed] } : s));
        setSaved((s) => ({ ...s, [changed]: next[changed].body }));
        if (!dirty) setDrafts((d) => ({ ...d, [changed]: next[changed].body }));
        if (dirty && fromElsewhere) setBehind((b) => ({ ...b, [changed]: true }));
      },
      (e) => {
        if (newest(changed, ticket)) setError(e);
      },
    );
  });

  const doc = (scope: Scope): MemoryDocument | null => (state ? state[scope] : null);
  // The count follows the draft, in runes — the unit the engine's budget is
  // in, and the reason `.length` will not do for Korean prose with emoji in it.
  const used = (scope: Scope) => [...drafts[scope]].length;

  const noWork = workId === "";

  const field = (scope: Scope, testId: string, label: MessageKey, help: MessageKey) => {
    const d = doc(scope);
    const helpId = `${testId}-help`;
    const countId = `${testId}-count`;
    const errorId = `${testId}-error`;
    const failure = saveError[scope];
    // Sighted writers get the budget for free — it sits under the box. A
    // screen reader is told the label and then nothing at all, in a pane whose
    // whole interaction is staying under a limit, unless the box says which
    // lines describe it.
    const describedBy = [helpId, d ? countId : null, failure != null ? errorId : null]
      .filter(Boolean)
      .join(" ");
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
          aria-describedby={describedBy}
          onChange={(e) => setDrafts((prev) => ({ ...prev, [scope]: e.target.value }))}
          onBlur={() => commit(scope)}
        />
        <p className="sd" id={helpId} data-testid={helpId}>
          {t(help)}
        </p>
        {d && (
          // <output>, not a <p>: this line is the only thing that says whether
          // the next sentence will be refused, and it changes as the writer
          // types. <output> carries polite status semantics natively — the
          // same reasoning as ProviderSection.tsx:820-828 — so the count is
          // announced when the typing pauses instead of either interrupting
          // every keystroke or never being spoken at all.
          <output className="sd" id={countId} data-testid={countId}>
            {t("settings.memory.remaining", { used: String(used(scope)), budget: String(d.chars_budget) })}
          </output>
        )}
        {behind[scope] && (
          <p className="sd" role="alert" data-testid={`${testId}-changed`}>
            {t("settings.memory.changedElsewhere")}
          </p>
        )}
        {failure != null && (
          <p className="sd" role="alert" id={errorId} data-testid={errorId}>
            {/* Over budget, an invisible character, a heading: every refusal
                the engine sends names something the writer can fix from this
                box — so it belongs beside that box, not at the foot of the
                pane where it reads as a broken Settings. */}
            {rpcErrorMessage(failure, t)}
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
          {/* Only a failure to READ lands here: neither box can be trusted
              after one, so it is not about either of them in particular. */}
          {rpcErrorMessage(error, t)}
        </p>
      )}
    </section>
  );
}
