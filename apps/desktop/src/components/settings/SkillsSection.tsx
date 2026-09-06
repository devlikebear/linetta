import { useCallback, useEffect, useRef, useState } from "react";

import { useEngineEvent } from "../../hooks/useEngineEvent";
import { useI18n } from "../../lib/i18n";
import { projects as projectsApi, skills as skillsApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type {
  Project,
  Skill,
  SkillChangedPayload,
  SkillScope,
  SkillSummary,
  SkillVersion,
} from "../../lib/types";

/** The skills a writer and their agent have written, as a writer reads,
 *  repairs and reverts them.
 *
 *  A skill is not a knob. It is prose that goes into a language model's
 *  prompt, an agent may write one without asking, and the file lives on disk
 *  where a writer can break it with any editor. Four rules follow, and they
 *  are what this component is shaped around:
 *
 *  - **A broken skill is visible.** `skills.list` never lists a SKILL.md it
 *    could not read — it must not reach a prompt — and always reports it as a
 *    diagnostic. If this pane dropped those, a writer would see their skill
 *    simply missing, with nothing anywhere saying why. So a diagnostic is a
 *    row of its own, it says which file and what is wrong, and it OPENS: the
 *    engine hands back the file verbatim (skills.read does not screen what it
 *    opens) precisely so it can be fixed here.
 *
 *  - **The author badge.** The plan's decision was that an agent writes a
 *    skill without an approval gate, and what the writer gets instead is
 *    attribution, a version history and a one-click switch. Take the badge
 *    off the row and that trade was never made.
 *
 *  - **The editor holds a DRAFT and commits on blur.** Per keystroke would be
 *    one RPC and one `skills.changed` per character. And a dirty draft is the
 *    writer's until they save it: when an agent writes underneath, the pane
 *    refetches, refreshes only a clean box, and SAYS the open one is now
 *    behind. A refused save keeps the draft too — losing what someone typed
 *    because it was twelve runes over the cap is the worst thing here.
 *
 *  - **Every reply that writes shared state is ORDERED.** See `seq` below.
 */

type Target = { scope: SkillScope; name: string };

const keyOf = (t: Target) => `${t.scope}:${t.name}`;
const rowId = (scope: SkillScope, name: string) => `${scope}-${name}`;

/** Runes, not UTF-16 units — the unit `agentskills.MaxBodyRunes` is in, and
 *  the reason `.length` will not do for prose with an emoji in it. */
const runes = (s: string) => [...s].length;

function formatTime(ts: number): string {
  const d = new Date(ts);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

/** Which skill a diagnostic is about, from the only thing a diagnostic
 *  carries: a path.
 *
 *  `agentskills.Store` lays skills out as `<home>/skills/<name>/SKILL.md` and
 *  `<home>/skills/works/<work>/<name>/SKILL.md` (store.go's `Dir`), and a
 *  diagnostic's path is either the SKILL.md or, when there is none, the
 *  directory. A skill's directory name IS its name, so both shapes name the
 *  skill — but only those two shapes do, and guessing at anything else would
 *  send skills.read after a name that is not there. So an unrecognised path
 *  yields null and the diagnostic is still SHOWN, just without an opener:
 *  visible-but-unopenable beats invisible, which is the whole point of the
 *  diagnostics existing.
 *
 *  Separators are normalised because this ships to Windows, where the engine
 *  builds these paths with backslashes. */
export function diagnosticTarget(path: string, workId: string): Target | null {
  const parts = path.replace(/\\/g, "/").split("/").filter((p) => p.length > 0);
  if (parts[parts.length - 1] === "SKILL.md") parts.pop();
  const name = parts.pop();
  if (!name) return null;
  const parent = parts[parts.length - 1];
  const grandparent = parts[parts.length - 2];
  if (workId !== "" && parent === workId && grandparent === "works") return { scope: "work", name };
  if (parent === "skills") return { scope: "writer", name };
  return null;
}

/** Whether a diagnostic points at a SKILL.md or at the directory that should
 *  have held one.
 *
 *  The two shapes need different repairs and `store.go` produces both: a file
 *  it could not parse or guard is reported by its own path, while an empty
 *  skill directory is reported as `no SKILL.md in this directory` under the
 *  DIRECTORY's path. There is no file to open in the second case —
 *  `Store.Read` answers ErrNotFound for it — so "Open and fix" would hand the
 *  writer "not found" for a folder they can see. An empty skill directory is
 *  a half-made skill, so the repair is to finish it. */
const pointsAtFile = (path: string) =>
  path.replace(/\\/g, "/").split("/").pop() === "SKILL.md";

type Lane = "list" | "detail" | "history";

export function SkillsSection() {
  const { t } = useI18n();

  const [works, setWorks] = useState<Project[]>([]);
  // null until projects.list answers — "" is a real, supported state (no work
  // in the library yet; skills.list answers it with the writer scope alone),
  // and starting at "" would fire a throwaway list whose late reply could
  // land on top of the first work's.
  const [workId, setWorkId] = useState<string | null>(null);

  const [rows, setRows] = useState<SkillSummary[] | null>(null);
  const [diagnostics, setDiagnostics] = useState<{ path: string; message: string }[]>([]);
  const [listError, setListError] = useState<unknown>(null);
  const [rowErrors, setRowErrors] = useState<Record<string, unknown>>({});

  const [selected, setSelected] = useState<Target | null>(null);
  const [detail, setDetail] = useState<Skill | null>(null);
  const [readError, setReadError] = useState<unknown>(null);
  // What the engine last confirmed, against which a draft is dirty-or-not.
  const [saved, setSaved] = useState({ description: "", body: "" });
  const [draft, setDraft] = useState({ description: "", body: "" });
  const [behind, setBehind] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);
  // The engine wrote the file but not the version row. Reported by write,
  // delete and restore alike, and never silence: the writer finds out
  // otherwise only when they go looking for the version and it is not there.
  const [notVersioned, setNotVersioned] = useState(false);
  const [asking, setAsking] = useState(false);
  // A count, not a flag: two blurs can be in flight at once (leave the
  // description, type in the body, leave that).
  const [inflight, setInflight] = useState(0);

  // Which rows have a toggle in flight, and which way it is going. A skill's
  // enabled flag takes a read then a write then a refetch — three round trips
  // for one click — and a checkbox bound to the last list reply does not move
  // for any of them, so the writer clicks it again and the second click sends
  // the same value a second time.
  const [toggling, setToggling] = useState<Record<string, boolean>>({});

  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [newDescription, setNewDescription] = useState("");
  const [newScope, setNewScope] = useState<SkillScope>("writer");
  const [newError, setNewError] = useState<unknown>(null);

  const [historyOpen, setHistoryOpen] = useState(false);
  const [versions, setVersions] = useState<SkillVersion[] | null>(null);
  const [versionId, setVersionId] = useState<string | null>(null);
  const [historyError, setHistoryError] = useState<unknown>(null);

  /** Ordering — the invariant the rest of this component rests on.
   *
   *  SIX callers write this pane's state from an await: the list load, a
   *  detail read, a blur save, a delete, a restore, and the `skills-changed`
   *  refetch. Replies come back in socket order, not request order, so an
   *  older one can land on top of a newer one — and here that is not a
   *  flicker. Open skill A, open skill B, let A's read answer last: A's body
   *  is now in the box under a heading reading B, and the next blur saves it
   *  ONTO B. MemorySection needed this after a Critical review finding and
   *  ProviderSection was rewritten around it; it is built in from the first
   *  line here rather than discovered again.
   *
   *  A ticket is claimed BEFORE every await and re-checked after it; a reply
   *  lands only while its ticket is still the newest for its lane. Three
   *  lanes, because the three move independently: a list refetch has no
   *  business discarding a save's confirmation, and opening the history must
   *  not invalidate the editor.
   *
   *  Cancellation IS a newer ticket — there is one mechanism, not a
   *  `cancelled` flag beside it that a later caller could forget to set. */
  const seq = useRef<Record<Lane, number>>({ list: 0, detail: 0, history: 0 });
  const claim = useCallback((lane: Lane) => (seq.current[lane] += 1), []);
  const newest = useCallback((lane: Lane, ticket: number) => seq.current[lane] === ticket, []);

  /** `skills.changed` says source "writer" for a save made by a person at a
   *  keyboard — ours and another window's alike (handlers/skills.go). Only
   *  ours is not news. One token per write we send, consumed by the first
   *  writer-sourced event for that skill, tells the two apart; a refused
   *  write hands its token back, because the engine emits nothing for one. */
  /** The newest document this pane has SENT for the open skill, whether or
   *  not its reply has come back yet.
   *
   *  `saved` is what the engine has CONFIRMED, and it is the wrong thing to
   *  compare a blur against on its own: while a save is travelling, `saved`
   *  still holds the pre-save document, so a second blur — leaving the
   *  description, clicking into the body, tabbing straight out — finds the
   *  draft "changed" and sends the identical document again. That is a
   *  duplicate version row, a restamped updated_at and a second
   *  `skills-changed` at every window, for an edit nobody made.
   *
   *  Cleared whenever the document moves underneath us from somewhere that is
   *  not one of our own writes (a fresh detail read, an agent's write, a
   *  restore), because then a draft equal to something we once sent is a real
   *  change again. */
  const pending = useRef<{ key: string; description: string; body: string } | null>(null);

  /** The open skill, readable from inside a promise callback. A delete's
   *  reply asks "is the writer still looking at the skill I removed?", and
   *  that is a question about the selection now, not about which detail-lane
   *  ticket happens to be newest. */
  const selectedRef = useRef<Target | null>(null);

  const ownSaves = useRef<Map<string, number>>(new Map());
  const takeToken = useCallback((key: string) => {
    ownSaves.current.set(key, (ownSaves.current.get(key) ?? 0) + 1);
  }, []);
  const giveBackToken = useCallback((key: string) => {
    const n = ownSaves.current.get(key) ?? 0;
    if (n > 0) ownSaves.current.set(key, n - 1);
  }, []);

  const applyList = useCallback(
    (id: string) => {
      const ticket = claim("list");
      // Returns the promise so a caller that has just changed a row — the
      // enabled toggle — can hold its pending state until the refreshed row
      // is actually on screen, rather than letting the checkbox snap back to
      // the old value in the gap.
      return skillsApi.list(id).then(
        (res) => {
          if (!newest("list", ticket)) return;
          setRows(res.skills ?? []);
          setDiagnostics(res.diagnostics ?? []);
          setListError(null);
        },
        (e) => {
          if (newest("list", ticket)) setListError(e);
        },
      );
    },
    [claim, newest],
  );

  useEffect(() => {
    projectsApi.list({}).then(
      (list) => {
        setWorks(list);
        setWorkId(list[0]?.id ?? "");
      },
      // A library that cannot be listed still has writer-scope skills, so the
      // pane opens on those rather than on an error.
      () => {
        setWorks([]);
        setWorkId("");
      },
    );
  }, []);

  useEffect(() => {
    if (workId === null) return;
    // A work-scoped skill belongs to one book. Keeping it open across a work
    // switch would leave the editor pointed at a file the new work has no
    // path to, and the next blur would write it somewhere else.
    setSelected(null);
    applyList(workId);
    return () => {
      claim("list");
    };
  }, [workId, applyList, claim]);

  const selScope = selected?.scope ?? null;
  const selName = selected?.name ?? null;

  useEffect(() => {
    selectedRef.current = selected;
  }, [selected]);

  useEffect(() => {
    if (workId === null || selScope === null || selName === null) {
      setDetail(null);
      return;
    }
    const ticket = claim("detail");
    setDetail(null);
    setReadError(null);
    setSaveError(null);
    setNotVersioned(false);
    setBehind(false);
    setAsking(false);
    setHistoryOpen(false);
    pending.current = null;
    skillsApi.read(selScope, workId, selName).then(
      (s) => {
        if (!newest("detail", ticket)) return;
        setDetail(s);
        setSaved({ description: s.description, body: s.body });
        setDraft({ description: s.description, body: s.body });
      },
      (e) => {
        if (newest("detail", ticket)) setReadError(e);
      },
    );
    return () => {
      claim("detail");
    };
  }, [workId, selScope, selName, claim, newest]);

  const dirty = draft.description !== saved.description || draft.body !== saved.body;

  /** Save on blur. Unchanged is not a save: writing anyway restamps
   *  updated_at, records a version row for a change nobody made, and fires
   *  `skills.changed` at every other window.
   *
   *  "Unchanged" is measured against the newest document SENT, not only
   *  against the newest confirmed — see `pending`. Two blurs in the time one
   *  save takes is ordinary (leave the description, click the textarea, tab
   *  out), and comparing against `saved` alone makes the second one resend
   *  the first one's document verbatim. */
  const commit = () => {
    if (!selected || !detail || workId === null) return;
    const sent = draft;
    const target = selected;
    const key = keyOf(target);
    const last = pending.current?.key === key ? pending.current : null;
    const base = last ?? saved;
    if (sent.description === base.description && sent.body === base.body) return;
    const ticket = claim("detail");
    pending.current = { key, description: sent.description, body: sent.body };
    takeToken(key);
    setSaveError(null);
    setNotVersioned(false);
    setInflight((n) => n + 1);
    void skillsApi
      .write({
        scope: target.scope,
        projectId: workId,
        name: target.name,
        description: sent.description,
        body: sent.body,
      })
      .then(
        (res) => {
          setInflight((n) => n - 1);
          if (!newest("detail", ticket)) return;
          setSaved({ description: res.description, body: res.body });
          // The box may have moved on: a writer who keeps typing after the
          // blur is still typing while this reply travels. Only the draft
          // that was SENT may be replaced by its own confirmation; anything
          // newer is unsent text, and unsent text is the writer's.
          setDraft((d) => ({
            description: d.description === sent.description ? res.description : d.description,
            body: d.body === sent.body ? res.body : d.body,
          }));
          setDetail(res);
          setBehind(false);
          // The file changed, the version row did not. Not an error — the
          // save DID land — but not silence either: this one edit is not on
          // the restore list and a backup carrying only the database misses
          // it.
          if (res.versioned === false) setNotVersioned(true);
        },
        (e) => {
          setInflight((n) => n - 1);
          giveBackToken(key);
          // A refused write is not a write: nothing on disk moved, so this
          // document is no longer something the engine has. Forgetting it
          // keeps a later identical blur from being mistaken for a duplicate
          // of a save that never happened. Only this document is forgotten —
          // a NEWER save may already be in flight behind it.
          const p = pending.current;
          if (p?.key === key && p.description === sent.description && p.body === sent.body) {
            pending.current = null;
          }
          if (!newest("detail", ticket)) return;
          // Over the cap, an invisible character, a missing description:
          // every refusal names something the writer can fix from these two
          // boxes, so the draft stays exactly as typed.
          setSaveError(e);
        },
      );
  };

  /** The one-click switch that stands in for an approval gate.
   *
   *  It reads before it writes because skills.write takes the whole document
   *  and a list row carries no body — a toggle that sent an empty body would
   *  silently blank the skill it was only meant to switch off.
   *
   *  Which is three round trips (read, write, refetch) for one click, so the
   *  checkbox is held at the value the writer asked for and disabled until
   *  the refreshed row lands. Without that it sits at the old value for the
   *  whole trip, the writer clicks it again, and the second click sends
   *  `enabled: false` a second time.
   *
   *  It claims a `list` ticket, like every other writer of the list: a list
   *  reply issued BEFORE this toggle carries the row this toggle is changing,
   *  so it is stale by the time it arrives, and the refetch at the end is
   *  what replaces it. That refetch is skipped if the writer changed works
   *  meanwhile — a list for the work they left must not land on the one they
   *  are looking at. */
  const toggleEnabled = (row: SkillSummary) => {
    if (workId === null) return;
    const id = workId;
    const key = `${row.scope}:${row.name}`;
    if (key in toggling) return;
    setRowErrors((m) => {
      const next = { ...m };
      delete next[key];
      return next;
    });
    setToggling((m) => ({ ...m, [key]: !row.enabled }));
    const done = () =>
      setToggling((m) => {
        const next = { ...m };
        delete next[key];
        return next;
      });
    const ticket = claim("list");
    takeToken(key);
    void skillsApi
      .read(row.scope, id, row.name)
      .then((s) =>
        skillsApi.write({
          scope: row.scope,
          projectId: id,
          name: row.name,
          description: s.description,
          body: s.body,
          enabled: !row.enabled,
        }),
      )
      .then(
        async () => {
          if (newest("list", ticket)) await applyList(id);
          done();
        },
        async (e) => {
          giveBackToken(key);
          setRowErrors((m) => ({ ...m, [key]: e }));
          // The pre-toggle list reply this ticket invalidated is gone, and
          // nothing changed on disk, so the list has to be asked again or the
          // rows stay at whatever was on screen before.
          if (newest("list", ticket)) await applyList(id);
          done();
        },
      );
  };

  const submitNew = () => {
    if (workId === null) return;
    const name = newName.trim();
    const description = newDescription.trim();
    const scope = newScope;
    const key = `${scope}:${name}`;
    setNewError(null);
    takeToken(key);
    void skillsApi.write({ scope, projectId: workId, name, description, body: "" }).then(
      (res) => {
        setCreating(false);
        setNewName("");
        setNewDescription("");
        // Opening it is the point: a skill with no body does nothing, so the
        // writer is put straight in front of the box they have to fill.
        setSelected({ scope, name: res.name });
        applyList(workId);
      },
      (e) => {
        giveBackToken(key);
        setNewError(e);
      },
    );
  };

  /** Delete.
   *
   *  The detail-lane ticket is deliberately NOT what decides whether the
   *  editor closes. Delete and restore both ride that lane, so a restore of
   *  the same skill claims a newer ticket without changing the selection —
   *  and then the delete's reply, arriving second, would skip `setSelected`
   *  and leave the writer typing into an editor for a file that is gone. The
   *  question a delete's reply actually asks is "is the writer still looking
   *  at the skill I removed?", which is about the selection, not the ticket. */
  const doDelete = () => {
    if (!selected || workId === null) return;
    const id = workId;
    const target = selected;
    const key = keyOf(target);
    takeToken(key);
    setAsking(false);
    setNotVersioned(false);
    const stillOpen = () => {
      const cur = selectedRef.current;
      return cur !== null && cur.scope === target.scope && cur.name === target.name;
    };
    void skillsApi.delete(target.scope, id, target.name).then(
      (res) => {
        if (stillOpen()) {
          setSelected(null);
          // The history is a panel INSIDE the detail and it is keyed on the
          // skill; leaving it open would offer to restore versions of
          // something no longer selected.
          setHistoryOpen(false);
          pending.current = null;
        }
        // A deletion whose version row did not land took the body with it —
        // the sharpest version of this warning. It sits at the pane level so
        // it survives the editor closing, which a delete always causes.
        if (res.versioned === false) setNotVersioned(true);
        applyList(id);
      },
      (e) => {
        giveBackToken(key);
        if (stillOpen()) setSaveError(e);
      },
    );
  };

  const openHistory = () => {
    if (!selected || workId === null) return;
    const target = selected;
    setHistoryOpen(true);
    setVersions(null);
    setVersionId(null);
    setHistoryError(null);
    const ticket = claim("history");
    skillsApi.history(target.scope, workId, target.name).then(
      (res) => {
        if (!newest("history", ticket)) return;
        setVersions(res.versions);
        setVersionId(res.versions[0]?.id ?? null);
      },
      (e) => {
        if (newest("history", ticket)) setHistoryError(e);
      },
    );
  };

  const doRestore = () => {
    if (!versionId || !selected) return;
    const key = keyOf(selected);
    const ticket = claim("detail");
    takeToken(key);
    setHistoryError(null);
    setNotVersioned(false);
    void skillsApi.restore(versionId).then(
      (res) => {
        if (!newest("detail", ticket)) return;
        // A restore is an explicit revert, so it replaces the draft — unlike
        // an agent's write underneath, which never does.
        setDetail(res);
        setSaved({ description: res.description, body: res.body });
        setDraft({ description: res.description, body: res.body });
        // The document moved from somewhere that is not one of our blurs, so
        // what we last sent says nothing about what is on disk now.
        pending.current = null;
        setSaveError(null);
        setBehind(false);
        setHistoryOpen(false);
        // A restore records a version row of its own, and it can fail to.
        // Then the reverted-to text is on disk with nothing in the history
        // marking that this is where the writer went back to.
        if (res.versioned === false) setNotVersioned(true);
        if (workId !== null) applyList(workId);
      },
      (e) => {
        giveBackToken(key);
        if (newest("detail", ticket)) setHistoryError(e);
      },
    );
  };

  useEngineEvent<SkillChangedPayload>("skills-changed", (payload) => {
    if (workId === null || !payload) return;
    const scope = payload.scope;
    if (scope !== "writer" && scope !== "work") return;
    // A writer-scope skill is global; a work-scope one belongs to one book.
    // An agent editing work-2's skill says nothing about the work-1 list on
    // screen — refetching would be a wasted round trip, and raising "an agent
    // just changed this" over a draft nothing touched is a false claim about
    // the writer's own text, on a notice with no dismiss.
    if (scope === "work" && (payload.project_id ?? "") !== workId) return;

    const key = `${scope}:${payload.name}`;
    let fromElsewhere = true;
    if (payload.source === "writer") {
      const n = ownSaves.current.get(key) ?? 0;
      if (n > 0) {
        ownSaves.current.set(key, n - 1);
        fromElsewhere = false;
      }
    }

    // The row's description, author badge and enabled flag all just moved.
    applyList(workId);

    if (!selected || selected.scope !== scope || selected.name !== payload.name) return;
    // Read dirtiness NOW, before the refetch: the answer must describe the
    // moment the agent wrote, not whatever the box holds when the reply
    // eventually lands.
    const wasDirty = dirty;
    const target = selected;
    const ticket = claim("detail");
    skillsApi.read(target.scope, workId, target.name).then(
      (s) => {
        if (!newest("detail", ticket)) return;
        setDetail(s);
        setSaved({ description: s.description, body: s.body });
        // Whatever we last sent is no longer what the file holds, so it is no
        // longer a duplicate to send it again. (Our own save's echo lands
        // here too, and re-reading the file it just wrote is the same
        // statement — the draft comparison below is what protects the box.)
        if (fromElsewhere) pending.current = null;
        if (!wasDirty) setDraft({ description: s.description, body: s.body });
        if (wasDirty && fromElsewhere) setBehind(true);
      },
      (e) => {
        if (newest("detail", ticket)) setReadError(e);
      },
    );
  });

  const noWork = workId === "";
  const busy = inflight > 0;
  const selectedVersion = versions?.find((v) => v.id === versionId) ?? null;
  const authorKey = (author: SkillSummary["author"]) =>
    author === "agent" ? "settings.skills.author.agent" : "settings.skills.author.writer";
  const scopeKey = (scope: SkillScope) =>
    scope === "work" ? "settings.skills.scope.work" : "settings.skills.scope.writer";
  /** What a version row IS. On a deleted skill's history it is the only thing
   *  telling the writer which row holds the last body they had. */
  const reasonKey = (reason: SkillVersion["reason"]) =>
    reason === "created"
      ? "settings.skills.reason.created"
      : reason === "deleted"
        ? "settings.skills.reason.deleted"
        : "settings.skills.reason.edited";

  /** Fill the create form from a diagnostic that is an empty skill directory:
   *  the folder is there, the SKILL.md is not, and writing one is the repair. */
  const startFrom = (target: Target) => {
    setSelected(null);
    setNewName(target.name);
    setNewDescription("");
    setNewScope(target.scope);
    setNewError(null);
    setCreating(true);
  };

  const bodyHelpId = "skill-body-help";
  const bodyCountId = "skill-body-count";
  const bodyErrorId = "skill-save-error";
  const describedBy = [bodyHelpId, detail ? bodyCountId : null, saveError != null ? bodyErrorId : null]
    .filter(Boolean)
    .join(" ");

  return (
    <section className="settings-section" id="skills-settings" data-testid="skills-section">
      <h3>{t("settings.skills.title")}</h3>
      <p className="sd">{t("settings.skills.description")}</p>

      {/* A work-scoped skill belongs to one book, so the book is named above
          the list rather than inferred from whatever was last open. */}
      <div className="modal-field">
        <label htmlFor="skills-work">{t("settings.skills.work")}</label>
        <select
          id="skills-work"
          data-testid="skills-work"
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

      {listError != null && (
        <p className="sd" role="alert" data-testid="skills-error">
          {rpcErrorMessage(listError, t)}
        </p>
      )}

      {/* The write landed on disk and the version row did not. It sits at the
          pane level and not in the editor because a delete raises it too, and
          a delete closes the editor — the one case where the missing version
          row is the difference between "I can undo this" and a body that is
          simply gone. */}
      {notVersioned && (
        <p className="sd" role="alert" data-testid="skills-not-versioned">
          {t("settings.skills.notVersioned")}
        </p>
      )}

      {/* The broken ones first, and never folded away. A SKILL.md the writer
          broke by hand is not listed as a skill — it must not reach a prompt —
          so this row is the only thing that says why the skill they wrote is
          missing. */}
      {diagnostics.length > 0 && (
        <div className="skills-diagnostics" data-testid="skills-diagnostics">
          {diagnostics.map((d, i) => {
            const target = diagnosticTarget(d.path, workId ?? "");
            // Two shapes, two repairs. A SKILL.md the writer broke opens on
            // its raw text (skills.read hands it back verbatim for exactly
            // this). A directory with no SKILL.md has no file to open —
            // Store.Read answers ErrNotFound and the writer would get "not
            // found" for a folder they can see — and is almost always a
            // half-made skill, so the repair is to finish writing it under
            // the name the folder already carries.
            const hasFile = pointsAtFile(d.path);
            return (
              <div className="skills-diagnostic" key={d.path} role="alert" data-testid={`skill-diagnostic-${i}`}>
                <span className="sd">{t("settings.skills.broken", { path: d.path })}</span>
                <span className="sd skills-diagnostic-why">{d.message}</span>
                {target && (
                  <button
                    type="button"
                    className="btn ghost sm"
                    data-testid={`skill-diagnostic-open-${i}`}
                    onClick={() => (hasFile ? setSelected(target) : startFrom(target))}
                  >
                    {t(hasFile ? "settings.skills.repair" : "settings.skills.repair.create")}
                  </button>
                )}
              </div>
            );
          })}
        </div>
      )}

      {rows !== null && (
        <ul className="skills-list" data-testid="skills-list">
          {rows.map((row) => {
            const id = rowId(row.scope, row.name);
            const key = `${row.scope}:${row.name}`;
            const rowError = rowErrors[key];
            const wanted = toggling[key];
            const pendingToggle = key in toggling;
            return (
              <li className="skills-row" key={id} data-testid={`skill-row-${id}`}>
                <button
                  type="button"
                  className="skills-row-main"
                  data-testid={`skill-open-${id}`}
                  onClick={() => setSelected({ scope: row.scope, name: row.name })}
                >
                  <span className="skills-row-name">{row.name}</span>
                  <span className="sd">{row.description}</span>
                </button>
                <span className="skills-badges">
                  <span className="skills-badge" data-testid={`skill-scope-${id}`}>
                    {t(scopeKey(row.scope))}
                  </span>
                  {/* Who wrote it. This badge is the entire substitute for an
                      approval gate on an agent-written skill. */}
                  <span className="skills-badge" data-testid={`skill-author-${id}`}>
                    {t(authorKey(row.author))}
                  </span>
                </span>
                <label className="skills-toggle">
                  {/* Held at what the writer asked for, and locked, for the
                      read-write-refetch the toggle takes. A checkbox that
                      does not move gets clicked again. */}
                  <input
                    type="checkbox"
                    data-testid={`skill-enabled-${id}`}
                    checked={pendingToggle ? wanted : row.enabled}
                    disabled={pendingToggle}
                    onChange={() => toggleEnabled(row)}
                  />
                  <span className="sd">{t("settings.skills.enabled")}</span>
                </label>
                {rowError != null && (
                  <p className="sd" role="alert" data-testid={`skill-row-error-${id}`}>
                    {rpcErrorMessage(rowError, t)}
                  </p>
                )}
              </li>
            );
          })}
          {rows.length === 0 && (
            <li className="sd" data-testid="skills-empty">
              {t("settings.skills.empty")}
            </li>
          )}
        </ul>
      )}

      {!creating && (
        <button type="button" className="btn ghost sm" data-testid="skills-new" onClick={() => setCreating(true)}>
          {t("settings.skills.new")}
        </button>
      )}

      {creating && (
        <div className="skills-new" data-testid="skill-new-form">
          <div className="modal-field">
            <label htmlFor="skill-new-name">{t("settings.skills.name")}</label>
            <input
              id="skill-new-name"
              data-testid="skill-new-name"
              aria-describedby="skill-new-name-help"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
            />
            {/* The name is the folder's name and there is no rename: a skill's
                identity is (scope, work, name) and that is what its history is
                keyed on. Say so before they pick one. */}
            <p className="sd" id="skill-new-name-help" data-testid="skill-new-name-help">
              {t("settings.skills.name.help")}
            </p>
          </div>
          <div className="modal-field">
            <label htmlFor="skill-new-description">{t("settings.skills.describe")}</label>
            <input
              id="skill-new-description"
              data-testid="skill-new-description"
              value={newDescription}
              onChange={(e) => setNewDescription(e.target.value)}
            />
          </div>
          <div className="modal-field">
            {/* Not `settings.skills.work`: the picker above chooses WHICH
                work's skills are listed, this chooses what a new skill
                applies to. The same word on both reads as the same control. */}
            <label htmlFor="skill-new-scope">{t("settings.skills.scope")}</label>
            <select
              id="skill-new-scope"
              data-testid="skill-new-scope"
              value={newScope}
              onChange={(e) => setNewScope(e.target.value as SkillScope)}
            >
              <option value="writer">{t("settings.skills.scope.writer")}</option>
              <option value="work" disabled={noWork}>
                {t("settings.skills.scope.work")}
              </option>
            </select>
          </div>
          {newError != null && (
            <p className="sd" role="alert" data-testid="skill-new-error">
              {rpcErrorMessage(newError, t)}
            </p>
          )}
          <div className="skills-actions">
            <button
              type="button"
              className="btn ghost sm"
              data-testid="skill-new-cancel"
              onClick={() => {
                setCreating(false);
                setNewError(null);
              }}
            >
              {t("common.cancel")}
            </button>
            <button
              type="button"
              className="btn accent sm"
              data-testid="skill-new-submit"
              disabled={newName.trim() === "" || newDescription.trim() === ""}
              onClick={submitNew}
            >
              {t("settings.skills.new")}
            </button>
          </div>
        </div>
      )}

      {selected && (
        <div className="skills-detail" data-testid="skill-detail">
          <div className="skills-detail-head">
            {/* The name the writer PICKED, not the one the last reply carried:
                a heading drawn from an in-flight read is the thing that makes
                a stale answer look like the skill they are editing. */}
            <h4 data-testid="skill-detail-name">{selected.name}</h4>
            {detail && (
              <span className="skills-badge" data-testid="skill-detail-author">
                {t(authorKey(detail.author))}
              </span>
            )}
          </div>

          {readError != null && (
            <p className="sd" role="alert" data-testid="skill-read-error">
              {rpcErrorMessage(readError, t)}
            </p>
          )}

          {detail && detail.parse_error && (
            // The file came back verbatim because it is not a skill. Without
            // this line the writer saves it straight back and wonders why the
            // skill still does not appear.
            <p className="sd" role="alert" data-testid="skill-parse-error">
              {detail.parse_error}
            </p>
          )}

          {detail && (
            <>
              <div className="modal-field">
                <label htmlFor="skill-description">{t("settings.skills.describe")}</label>
                <input
                  id="skill-description"
                  data-testid="skill-description"
                  value={draft.description}
                  onChange={(e) => setDraft((d) => ({ ...d, description: e.target.value }))}
                  onBlur={commit}
                />
              </div>

              <div className="modal-field">
                <label htmlFor="skill-body">{t("settings.skills.body")}</label>
                <textarea
                  id="skill-body"
                  data-testid="skill-body"
                  rows={12}
                  value={draft.body}
                  aria-describedby={describedBy}
                  onChange={(e) => setDraft((d) => ({ ...d, body: e.target.value }))}
                  onBlur={commit}
                />
                <p className="sd" id={bodyHelpId} data-testid={bodyHelpId}>
                  {t("settings.skills.body.help")}
                </p>
                {/* <output>, not a <p>: this line is the only thing that says
                    whether the next paragraph will be refused, and it changes
                    as the writer types. <output> is a polite live region
                    natively (ProviderSection.tsx:827-834), so it is announced
                    when the typing pauses rather than at every keystroke. */}
                <output className="sd" id={bodyCountId} data-testid={bodyCountId}>
                  {t("settings.skills.remaining", {
                    used: String(runes(draft.body)),
                    budget: String(detail.body_budget),
                  })}
                </output>
                {behind && (
                  <p className="sd" role="alert" data-testid="skill-changed">
                    {t("settings.skills.changedElsewhere")}
                  </p>
                )}
                {saveError != null && (
                  <p className="sd" role="alert" id={bodyErrorId} data-testid={bodyErrorId}>
                    {rpcErrorMessage(saveError, t)}
                  </p>
                )}
              </div>

              <div className="skills-actions">
                <button type="button" className="btn ghost sm" data-testid="skill-history" onClick={openHistory}>
                  {t("settings.skills.history")}
                </button>
                <span className="spacer" />
                {!asking && (
                  <button
                    type="button"
                    className="btn ghost sm"
                    data-testid="skill-delete"
                    onClick={() => setAsking(true)}
                  >
                    {t("settings.skills.delete")}
                  </button>
                )}
              </div>

              {asking && (
                <div className="skills-confirm">
                  {/* A skill is prose someone wrote. One stray click must not
                      take it — and the prompt says the history keeps it, so
                      the writer knows what they are risking. */}
                  <p className="sd" data-testid="skill-delete-prompt">
                    {t("settings.skills.delete.confirm")}
                  </p>
                  <div className="skills-actions">
                    <button
                      type="button"
                      className="btn ghost sm"
                      data-testid="skill-delete-cancel"
                      onClick={() => setAsking(false)}
                    >
                      {t("common.cancel")}
                    </button>
                    <button
                      type="button"
                      className="btn sm"
                      data-testid="skill-delete-confirm"
                      onClick={doDelete}
                    >
                      {t("settings.skills.delete")}
                    </button>
                  </div>
                </div>
              )}

              {historyOpen && (
                <div className="skills-history" data-testid="skill-history-panel">
                  <div className="skills-history-list">
                    {versions === null && historyError == null && (
                      <p className="sd">{t("common.loading")}</p>
                    )}
                    {versions !== null && versions.length === 0 && (
                      <p className="sd" data-testid="skill-history-empty">
                        {t("settings.skills.history.empty")}
                      </p>
                    )}
                    {(versions ?? []).map((v) => (
                      <button
                        type="button"
                        key={v.id}
                        className={"skills-version" + (v.id === versionId ? " on" : "")}
                        data-testid={`skill-version-${v.id}`}
                        onClick={() => setVersionId(v.id)}
                      >
                        <span className="skills-version-time">{formatTime(v.created_at)}</span>
                        {/* What happened, not just when. On a deleted skill's
                            history this is the only thing that says which row
                            holds the last body the writer had. */}
                        <span className="skills-badge" data-testid={`skill-version-reason-${v.id}`}>
                          {t(reasonKey(v.reason))}
                        </span>
                        <span className="skills-badge">{t(authorKey(v.author))}</span>
                      </button>
                    ))}
                  </div>
                  {/* The body travels with every row precisely so the writer
                      can see what they are about to revert to; a version list
                      you cannot preview is a list of timestamps. */}
                  <pre className="skills-version-preview" data-testid="skill-version-preview">
                    {selectedVersion?.body ?? ""}
                  </pre>
                  {historyError != null && (
                    <p className="sd" role="alert" data-testid="skill-history-error">
                      {rpcErrorMessage(historyError, t)}
                    </p>
                  )}
                  <div className="skills-actions">
                    <button
                      type="button"
                      className="btn ghost sm"
                      data-testid="skill-history-close"
                      onClick={() => setHistoryOpen(false)}
                    >
                      {t("common.close")}
                    </button>
                    <button
                      type="button"
                      className="btn accent sm"
                      data-testid="skill-history-restore"
                      disabled={!selectedVersion}
                      onClick={doRestore}
                    >
                      {t("settings.skills.history.restore")}
                    </button>
                  </div>
                </div>
              )}
            </>
          )}

          {!detail && readError == null && <p className="sd">{t("common.loading")}</p>}
        </div>
      )}
    </section>
  );
}
