import { useEffect, useRef, useState } from "react";
import { Bot, Send, Square, X } from "lucide-react";
import { Link } from "react-router-dom";
import { agent as agentApi, providers as providersApi } from "../../lib/rpc";
import type { AgentHistoryRow } from "../../lib/types";
import { useI18n } from "../../lib/i18n";
import { toolKind, toolLabelKey, toolVerbKey } from "../../lib/agentTools";
import { reasonMessage, rpcErrorMessage } from "../../lib/rpcMessage";
import { useEngineEvent } from "../../hooks/useEngineEvent";
import { useSmoothStream } from "../../hooks/useSmoothStream";
import { Markdown } from "./Markdown";
import "./AgentPanel.css";

/** The built-in agent's panel (#95).
 *
 *  Task 3 built the shell: it opens, it closes, and it says whether a
 *  provider is ready to talk to. Task 4 filled the log with the writer's
 *  turn, the agent's reply, and the reply arriving as it streams. Task 5
 *  rendered the tool line Task 4 kept but never showed, and added undo for
 *  the lines that carry a batch id. Task 6 added the composer, the stop
 *  button, the three starting chips, and the usage line at the end of a
 *  turn — the panel can now actually send something. This task (7) restores
 *  the conversation from agent.history when the panel reopens, and renders
 *  agent.error and agent.cancelled — the two terminal wire events nothing
 *  before this task ever drew.
 */

/** One line in the transcript. Mirrors the shape in the task-4 brief.
 *
 *  A tool line collapses two wire events (`state: "started"` then `"done"`
 *  or `"error"`) into one row. There is no per-call id on `agent.tool`'s
 *  payload — only `run_id` and `name` — so `id` is built from the one
 *  correlator that exists: run id + tool name + how many times that name
 *  has started in this turn.
 *
 *  `undoing`/`undone`/`undoError` are Task 5 additions, local to this
 *  component's undo flow — they never arrive over the wire.
 *
 *  `summary` is kept but never rendered. Whatever the engine puts there is
 *  machine text in both directions — raw call arguments on "started", the
 *  JSON serialisation of the tool's output on "done", an English engine
 *  sentence on "error" — so the line is built from `name` instead (see
 *  lib/agentTools.ts). It stays on the type because the "started" event's
 *  copy must be actively discarded rather than merely hidden; see the
 *  handler below. */
type Line =
  | { kind: "user"; id: string; text: string }
  | { kind: "assistant"; id: string; text: string; usage?: { input: number; output: number } }
  | {
      kind: "tool";
      id: string;
      name: string;
      summary: string;
      state: "running" | "ok" | "error";
      batchId?: string;
      undoing?: boolean;
      undone?: boolean;
      undoError?: string;
    }
  | NoticeLine;

/** A terminal wire event with nothing else to collapse into (Task 7):
 *  agent.error or agent.cancelled, or the panel's own reading of a restored
 *  conversation — that a prior turn may still be running, or that a restored
 *  turn ended in a failure. Its own Line variant rather than folding into
 *  `sendError` (the composer's synchronous-refusal banner, which sits outside
 *  the log and only ever holds one message): a mid-turn failure belongs
 *  beside the partial reply that produced it, in the transcript, not in a
 *  banner the NEXT turn's own error would silently overwrite.
 *
 *  `reason` is required only on the "error" variant — it is the one thing
 *  rendered from it, and only "error" needs it (never `message`; see
 *  AgentErrorPayload and reasonMessage). Text is resolved at render time
 *  from `variant`/`reason`, the same as a tool line's verb+label, so this
 *  carries no pre-translated copy to go stale under a language switch.
 *
 *  "failed" is the restored counterpart of "error" and carries no reason:
 *  the transcript stamps rows with a status ("failed"), not with the reason
 *  code that produced it, so a restored failure can honestly say only THAT
 *  the turn failed. Inventing a reason to reuse the "error" variant would
 *  put a sentence on screen the row does not support. */
type NoticeLine =
  | { kind: "notice"; id: string; variant: "restored" }
  | { kind: "notice"; id: string; variant: "cancelled" }
  | { kind: "notice"; id: string; variant: "failed" }
  | { kind: "notice"; id: string; variant: "error"; reason: string };

interface AgentDeltaPayload {
  run_id: string;
  text: string;
}

interface AgentToolPayload {
  run_id: string;
  name: string;
  state: "started" | "done" | "error";
  /** Never rendered. See the `summary` note on `Line` and lib/agentTools.ts:
   *  on every event, in both directions, this field is engine JSON or an
   *  English engine sentence. */
  summary?: string;
  batch_id?: string;
  /** The scenes the call touched, as opaque node ids. The panel would like to
   *  name the scene on the line ("씀 · 4-2 씬"), but ids are not labels and
   *  this component has no tree to resolve them against — a lookup per tool
   *  event is a bigger change than this task should make. Rendering the id
   *  itself would be worse than rendering nothing, so the line carries the
   *  tool's label alone until a later task hands the panel the outline. */
  node_ids?: string[];
}

interface AgentDonePayload {
  run_id: string;
  model?: string;
  usage: { input: number; output: number };
}

interface AgentErrorPayload {
  run_id: string;
  reason: string;
  message: string;
}

interface AgentCancelledPayload {
  run_id: string;
}

/** The composer's growth cap, in px. Must match `max-height` on
 *  `.cmp-input textarea` in App.css — the CSS enforces it for a box that has
 *  never been autosized, this constant for one that has. */
const COMPOSER_MAX_HEIGHT = 120;

/** What a `role === "tool"` history row's `content` decodes to — the exact
 *  wire shape agent.tool's own resolving event carries (see AgentToolPayload
 *  above), because the engine writes one straight from the other (see
 *  agent/transcript.go's toolEvent, appended by loop.go's runTool). Only the
 *  two fields the line cannot be drawn without are required here. */
interface RestoredToolEvent {
  name: string;
  ok: boolean;
  batch_id?: string;
}

/** Parses one tool row's `content`. Null for anything that is not a usable
 *  toolEvent — malformed JSON, or JSON missing `name`/`ok` — so the caller
 *  can skip the row instead of crashing the whole restore over it. */
function parseToolEvent(content: string): RestoredToolEvent | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(content);
  } catch {
    return null;
  }
  if (!parsed || typeof parsed !== "object") return null;
  const { name, ok, batch_id: batchId } = parsed as Record<string, unknown>;
  if (typeof name !== "string" || typeof ok !== "boolean") return null;
  return { name, ok, batch_id: typeof batchId === "string" ? batchId : undefined };
}

/** How a restored turn ended, read off the last row of its run.
 *
 *  The engine stamps every row of a run with one status when the turn ends
 *  (transcript.go's markRun), so any row of the run answers this — the last
 *  one is simply the one the boundary check below already has in hand.
 *
 *  Only two statuses mean the turn ended badly. A provider failure or a panic
 *  stamps "failed" (loop.go's endWithError / the panic recovery); a cancel
 *  stamps "cancelled". Everything else is "done", INCLUDING the iteration
 *  wall, which the engine keeps "done" on purpose: that turn's tool calls
 *  really ran and its partial reply is real work, so it must not be drawn
 *  with a failure beneath it (see endAtWall's comment). A normally completed
 *  turn is never marked at all and keeps the "done" its rows were written
 *  with. */
function endNoticeVariant(status: string): "failed" | "cancelled" | null {
  if (status === "failed") return "failed";
  if (status === "cancelled") return "cancelled";
  return null;
}

/** agent.history's rows -> the Line shape the log already renders (Task 7).
 *  A row this function cannot turn into a Line — an unrecognised role, or a
 *  tool row whose content fails to parse — is skipped rather than thrown:
 *  one bad row must not blank the whole restored conversation.
 *
 *  A run that ended badly gets a notice line closing it out, so a turn that
 *  died mid-reply does not restore looking exactly like one that finished.
 *  Live, that is what agent.error and agent.cancelled draw; restored, `status`
 *  is the only trace either of them leaves. */
function linesFromHistory(rows: AgentHistoryRow[]): Line[] {
  const lines: Line[] = [];
  for (let i = 0; i < rows.length; i++) {
    const row = rows[i];
    if (row.role === "user") {
      lines.push({ kind: "user", id: `history:${row.id}`, text: row.content });
    } else if (row.role === "assistant") {
      lines.push({ kind: "assistant", id: `history:${row.id}`, text: row.content });
    } else if (row.role === "tool") {
      const ev = parseToolEvent(row.content);
      if (ev) {
        lines.push({
          kind: "tool",
          id: `history:${row.id}`,
          name: ev.name,
          // Same rule as the live handler below: never render what the engine
          // put in `summary`, restored or not. Nothing reads it back off a
          // Line — this is "" purely so the shape matches a live tool line.
          summary: "",
          state: ev.ok ? "ok" : "error",
          batchId: ev.batch_id,
        });
      }
      // An unparseable tool row still counts as a row of its run for the
      // boundary check below — skipping the LINE must not also skip the
      // notice that says how that run ended.
    }
    // Any other role is a row this panel has no drawing for. There should
    // not be one — role is "user" | "assistant" | "tool" — but the wire is
    // not the type system, so it is skipped rather than guessed at.

    // A run's rows are contiguous and ordered, so a run ends where the next
    // row belongs to a different one. (Rows written before run ids existed
    // all share `undefined` and read as a single run; they are the removed
    // 1.0 companion's, which this intent-scoped query never returns.)
    const endsRun = i === rows.length - 1 || rows[i + 1].run_id !== row.run_id;
    if (!endsRun) continue;
    const variant = endNoticeVariant(row.status);
    if (variant) lines.push({ kind: "notice", id: `history:${row.id}:${variant}`, variant });
  }
  return lines;
}

/** Whether the restored conversation might still have a turn running in the
 *  engine — the notice from the Task 7 brief's 7-3, widened past "the last row
 *  is a user row" (which missed the abandoned turn that got a tool call in).
 *
 *  What the rows can actually say:
 *   - A "failed" or "cancelled" status means markRun ran, which only happens
 *     on the way out of a turn. That turn is over, and linesFromHistory has
 *     already said so above.
 *   - An assistant row is text the model produced. The loop writes one only
 *     after a completed Chat call, and a turn that keeps going writes a tool
 *     row after it — so an assistant row at the very end is a turn that got
 *     its answer out.
 *   - Anything else — a user row with no reply, a tool row with no reply
 *     after it — is a conversation that stops mid-turn, which is exactly what
 *     an abandoned run leaves behind.
 *
 *  What they cannot say: the loop writes the assistant row BEFORE running the
 *  tool calls that came with it, so a turn killed in that window ends on an
 *  assistant row and reads here as settled. Nothing in the transcript
 *  distinguishes it (the row carries no "and it also asked for tools"), and
 *  the panel has no run-status RPC to ask — the plan deliberately does not add
 *  one. A missed notice is the safe direction of that error: the notice is a
 *  hedge, and claiming a finished turn may still be running would be worse
 *  than staying quiet. */
function turnMayBeRunning(rows: AgentHistoryRow[]): boolean {
  const last = rows[rows.length - 1];
  if (!last) return false;
  if (endNoticeVariant(last.status)) return false;
  return last.role !== "assistant";
}

interface Props {
  onClose: () => void;
  /** The active project — agent.run's first argument. */
  projectId: string;
  /** The currently open editor's node id — agent.run's scope argument and
   *  the only material the engine gets for "which scene do you mean". */
  nodeId: string;
}

export function AgentPanel({ onClose, projectId, nodeId }: Props) {
  const { t } = useI18n();
  // "Ready" means the active row is both configured AND consented. A
  // credential without consent is refused server-side — Source.Client()
  // requires both — so a turn sent while only configured comes back as
  // provider_consent_required. Telling the writer up front beats sending it
  // and rendering an error.
  //
  // Tri-state, not boolean: `null` means "still checking". Defaulting to
  // `false` would flash the unconfigured notice on every open, even for a
  // writer who is fully set up — a false claim about their own setup, not
  // just an ugly flash.
  const [ready, setReady] = useState<boolean | null>(null);
  const [lines, setLines] = useState<Line[]>([]);
  // Whether the current run is still producing text. Feeds useSmoothStream:
  // while true the reply reveals gradually; once false it snaps to the full
  // text (also covers the mount-with-nothing-streaming-yet case).
  const [running, setRunning] = useState(false);

  // The composer (Task 6). `draft` is the textarea's own value. `sending` is
  // true only for the round trip of agent.run itself — before a run id
  // exists there is nothing to show a stop button for yet. `turnRunId` is
  // the run the composer considers "in progress": set the moment agent.run
  // resolves, cleared by that same run's terminal event (done/error/
  // cancelled). It drives the send-vs-stop toggle; it is state (not the
  // acceptRun ref) because the button needs a re-render when it changes.
  // `canceling` guards agent.cancel the same way handleUndo guards
  // agent.undo — one click, one call, even if the writer clicks again before
  // the round trip returns.
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [turnRunId, setTurnRunId] = useState<string | null>(null);
  const [canceling, setCanceling] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);
  // The same value as `turnRunId`, readable from an async callback that
  // closed over an older render — handleStop's rejection has to know whether
  // the turn is *still* running, and its `turnRunId` closure was captured
  // before the round trip started. Written only through setTurn below, so the
  // two can never disagree.
  const turnRunIdRef = useRef<string | null>(null);
  function setTurn(runId: string | null) {
    turnRunIdRef.current = runId;
    setTurnRunId(runId);
  }

  // The run this mount follows. Locked onto the first run id any agent.*
  // event names; every later event carrying a different run id is dropped.
  // There is no composer yet to hand this panel an authoritative run id up
  // front (that lands in Task 6), so the first event this mount sees is the
  // only signal available — and once picked, it must not be overridden by a
  // stray late event from some other run. #94 spent three fix rounds on
  // exactly this class of bug (a late response for an abandoned selection
  // overwriting the screen); this guard is built in from the start rather
  // than discovered later.
  const currentRunIdRef = useRef<string | null>(null);
  // agent.tool fires "started" and then "done"/"error" with no call id to
  // tie them together — arrival order is the only correlator. This tracks,
  // per run+tool-name, the id of the most recently started (and not yet
  // resolved) call, so the resolving event can find the right line.
  const pendingToolRef = useRef<Map<string, string>>(new Map());
  // How many times each run+tool-name pair has started, to build the id
  // scheme described on `Line` above.
  const toolOccurrenceRef = useRef<Map<string, number>>(new Map());

  // acceptRun locks onto the first run id this mount sees and refuses any
  // event from a different run. This guard prevents a late straggler event
  // from an abandoned run — one that started before the user abandoned it,
  // then arrived after the run was abandoned — from being mistakenly adopted
  // as the "current" run simply because it was the first event to land for
  // this mount. But the lock is not reset on a terminal event (done/error/
  // cancelled); resetting it would reopen the same hazard on a subtler axis:
  // an event arriving after terminal_state but before the next agent.run()
  // call completes could still be adopted first. handleSend below is what
  // makes that safe: it sets currentRunIdRef.current directly from
  // agent.run()'s returned run_id the moment the response lands, rather than
  // leaving it to be decided by the first arriving event. Without that step, a
  // second turn in the same mount silently drops every event from the second
  // run and renders no output — and the events that arrive before that
  // response are held rather than judged (see the send window below).

  // Runs this panel started for a work it is no longer showing. #93 made a
  // turn outlive the RPC call, so jumping to another project does not stop
  // one — the engine keeps running it and keeps emitting its events, and
  // those events carry a run id but no project id. Without naming them, the
  // first straggler to arrive would be adopted by acceptRun's first-wins null
  // check (the switch below clears currentRunIdRef) and one work's prose
  // would stream into another's log.
  //
  // They stay refused even if the writer jumps back: by then that turn's
  // rows have been restored from history, and re-adopting the run would
  // append its remaining deltas to a bubble that no longer exists. The
  // "may still be running" notice restore puts under it is what that writer
  // gets instead — which is what it is for.
  const abandonedRunsRef = useRef<Set<string>>(new Set());

  function acceptRun(runId: string): boolean {
    if (abandonedRunsRef.current.has(runId)) return false;
    if (currentRunIdRef.current === null) currentRunIdRef.current = runId;
    return currentRunIdRef.current === runId;
  }

  // The send window. Between agent.run being called and its response landing,
  // this mount has no authoritative id for the turn it just started:
  // currentRunIdRef still names the PREVIOUS turn, so acceptRun would judge
  // the new run's events against the old id. Every one of the three ways that
  // goes wrong is a consequence of judging too early, so nothing is judged
  // inside the window at all — events that arrive in it are held, and
  // replayed the moment agent.run's response assigns the real id (see
  // handleSend), which is the first instant acceptRun can be right:
  //   - the new turn's opening chunk would be dropped (prose lost from state,
  //     not merely from the reveal),
  //   - a straggler belonging to the FINISHED previous turn would be accepted
  //     and appended to its already-complete bubble,
  //   - the new turn's own terminal event would be dropped, leaving the
  //     composer stuck on stop with no further event ever coming for that run
  //     — and the agent-cancelled from clicking stop dropped along with it.
  // Service.Run mints the run id in a synchronous call and only then starts
  // the turn, so the window is normally empty; a provider that fails without
  // a round trip (offline, 401) is what actually lands in it.
  const pendingSendRef = useRef(false);
  const bufferedEventsRef = useRef<Array<() => void>>([]);

  /** Holds `apply` until the in-flight send resolves, if there is one.
   *  Returns true when it did — the caller must not run now. */
  function deferDuringSend(apply: () => void): boolean {
    if (!pendingSendRef.current) return false;
    bufferedEventsRef.current.push(apply);
    return true;
  }

  /** Closes the send window and replays what it held, in arrival order.
   *  Synchronous, so no later event can interleave ahead of the replay. */
  function flushSendWindow() {
    pendingSendRef.current = false;
    const held = bufferedEventsRef.current;
    bufferedEventsRef.current = [];
    for (const apply of held) apply();
  }

  /** Closes the window and throws away what it held — the panel is gone. */
  function discardSendWindow() {
    pendingSendRef.current = false;
    bufferedEventsRef.current = [];
  }

  // The work this panel is currently showing, readable from an async callback
  // that closed over an older render — handleSend's round trip has to know
  // whether the writer has jumped elsewhere since it left. Written only in
  // the reset below, so it and `projectId` can never disagree after a render.
  const projectIdRef = useRef(projectId);

  // A jump to another work (#95 Task 7 review, C1). Neither Workspace nor
  // AgentPanel is keyed on the project id and the route is not remounted on a
  // param change, so global search's cross-project jump changes `projectId`
  // under a panel that stays mounted with everything from the previous work
  // still in it. Without this, the previous work's conversation stays on
  // screen and the next restore prepends the new work's rows to it: two
  // writers' — or one writer's two books' — private conversations interleaved
  // in one log, unlabelled.
  //
  // Reset during render rather than in an effect, the pattern React documents
  // for "reset all state when a prop changes": an effect runs after paint, so
  // the previous work's messages would be shown for a frame under the new
  // work's title. For a leak whose whole harm is "this text was visible where
  // it should not have been", not painting it at all is the fix.
  //
  // `draft` deliberately survives: it is the writer's own unsent typing, not
  // the other work's conversation, and throwing away what someone typed is a
  // worse trade than carrying a sentence across.
  const [shownProjectId, setShownProjectId] = useState(projectId);
  if (shownProjectId !== projectId) {
    setShownProjectId(projectId);
    projectIdRef.current = projectId;
    // A turn may still be running for the work being left. Refuse its events
    // from here on rather than letting the cleared currentRunIdRef adopt them.
    if (currentRunIdRef.current) abandonedRunsRef.current.add(currentRunIdRef.current);
    currentRunIdRef.current = null;
    pendingToolRef.current.clear();
    toolOccurrenceRef.current.clear();
    // Held events belong to the run being abandoned, and the send that opened
    // the window (if any) is disowned below in handleSend.
    discardSendWindow();
    setLines([]);
    setRunning(false);
    setTurn(null);
    setCanceling(false);
    setSending(false);
    setSendError(null);
  }

  /** Wraps a wire handler so anything arriving inside the send window waits
   *  for a run id to judge it against, instead of being judged against the
   *  previous turn's. */
  function whileNotSending<T>(apply: (payload: T) => void) {
    return (payload: T) => {
      if (deferDuringSend(() => apply(payload))) return;
      apply(payload);
    };
  }

  useEngineEvent<AgentDeltaPayload>("agent-delta", whileNotSending<AgentDeltaPayload>((payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(true);
    const id = `assistant:${payload.run_id}`;
    setLines((prev) => {
      const idx = prev.findIndex((l) => l.kind === "assistant" && l.id === id);
      if (idx === -1) return [...prev, { kind: "assistant", id, text: payload.text }];
      const next = [...prev];
      const line = next[idx] as Extract<Line, { kind: "assistant" }>;
      next[idx] = { ...line, text: line.text + payload.text };
      return next;
    });
  }));

  useEngineEvent<AgentToolPayload>("agent-tool", whileNotSending<AgentToolPayload>((payload) => {
    if (!acceptRun(payload.run_id)) return;
    const key = `${payload.run_id}:${payload.name}`;
    if (payload.state === "started") {
      const occurrence = (toolOccurrenceRef.current.get(key) ?? 0) + 1;
      toolOccurrenceRef.current.set(key, occurrence);
      const id = `${key}:${occurrence}`;
      pendingToolRef.current.set(key, id);
      setLines((prev) => [
        ...prev,
        {
          kind: "tool",
          id,
          name: payload.name,
          // Deliberately not `payload.summary`: on "started" that field is
          // the tool's raw call arguments (see runTool in
          // engine/internal/agent/loop.go), which can carry a full scene
          // body — never the full arguments. Discarding it here, rather
          // than storing it and filtering it out only in the render loop,
          // also means the "done"/"error" merge below can never fall back
          // to it: `payload.summary ?? l.summary` only reaches `l.summary`
          // if a resolving event ever arrived with no summary of its own,
          // and starting from "" makes that fallback safe by construction
          // rather than dependent on any one render site. Nothing renders
          // `summary` today, and this keeps that true even if something does.
          summary: "",
          state: "running",
          batchId: payload.batch_id,
        },
      ]);
      return;
    }
    // "done" or "error": resolve the most recently started call for this
    // run+name. A resolution with no matching start (shouldn't happen per
    // the engine's contract, but the wire is not the type system) is
    // dropped rather than fabricating a row for it.
    const id = pendingToolRef.current.get(key);
    pendingToolRef.current.delete(key);
    if (!id) return;
    setLines((prev) =>
      prev.map((l) =>
        l.kind === "tool" && l.id === id
          ? {
              ...l,
              state: payload.state === "error" ? "error" : "ok",
              summary: payload.summary ?? l.summary,
              batchId: payload.batch_id ?? l.batchId,
            }
          : l,
      ),
    );
  }));

  useEngineEvent<AgentDonePayload>("agent-done", whileNotSending<AgentDonePayload>((payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(false);
    // The composer's send control returns from "stop" to "send" once this
    // run's own terminal event lands — not on a timer, not on any other
    // run's terminal event (acceptRun above already refused those).
    setTurn(null);
    setCanceling(false);
    const id = `assistant:${payload.run_id}`;
    setLines((prev) =>
      prev.map((l) => (l.kind === "assistant" && l.id === id ? { ...l, usage: payload.usage } : l)),
    );
  }));

  // A mid-turn failure (Task 7). Stops the streaming indicator honestly
  // instead of leaving the reply looking like it is still arriving, hands
  // the composer back to the writer, and appends a notice line — the
  // partial reply already in `lines` from agent-delta is left exactly as
  // it is, right above it. `reason` is kept on the Line, not translated
  // here: render time is where every other piece of copy in this log
  // resolves (see toolVerbKey/toolLabelKey), and `message` is never even
  // read — agent_internal_error's carries a raw Go panic value.
  useEngineEvent<AgentErrorPayload>("agent-error", whileNotSending<AgentErrorPayload>((payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(false);
    setTurn(null);
    setCanceling(false);
    setLines((prev) => [
      ...prev,
      { kind: "notice", id: `notice:${payload.run_id}:error`, variant: "error", reason: payload.reason },
    ]);
  }));

  useEngineEvent<AgentCancelledPayload>("agent-cancelled", whileNotSending<AgentCancelledPayload>((payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(false);
    setTurn(null);
    setCanceling(false);
    setLines((prev) => [
      ...prev,
      { kind: "notice", id: `notice:${payload.run_id}:cancelled`, variant: "cancelled" },
    ]);
  }));

  // Undo is a per-line action, not a turn-level one: mark only the clicked
  // line pending, then resolve it to undone or to a translated failure.
  // Failure — most commonly `agent_undo_unavailable`, since the service
  // keeps only the last 8 batches — is rendered beside the line and the
  // button is dropped; it never becomes a panel-level error.
  // Undo resolves outside any effect, so it cannot use an effect-scoped
  // cancelled flag. Reassigned on mount, not just initialised, so a
  // StrictMode remount does not leave the panel permanently thinking it is
  // gone.
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  function handleUndo(id: string, batchId: string) {
    // No `undoError: undefined` reset here: `showUndo` in the render loop
    // requires `!line.undoError`, so a line that already has one has no
    // button to click and can never reach this function.
    setLines((prev) => prev.map((l) => (l.kind === "tool" && l.id === id ? { ...l, undoing: true } : l)));
    agentApi
      .undo(batchId)
      .then(() => {
        // Same guard as the providers effect below: this resolves after an
        // await, and the writer may have closed the panel in between.
        if (!mountedRef.current) return;
        setLines((prev) =>
          prev.map((l) => (l.kind === "tool" && l.id === id ? { ...l, undoing: false, undone: true } : l)),
        );
      })
      .catch((err: unknown) => {
        if (!mountedRef.current) return;
        setLines((prev) =>
          prev.map((l) =>
            l.kind === "tool" && l.id === id ? { ...l, undoing: false, undoError: rpcErrorMessage(err, t) } : l,
          ),
        );
      });
  }

  // Sends the composer's text as a new turn. A no-op while a turn is already
  // in flight (busy below already hides the send control in that state, but
  // Enter can still reach here) or while the draft is empty.
  function handleSend() {
    const prompt = draft.trim();
    if (!prompt || sending || turnRunId !== null) return;
    // The work this turn is for. Compared against projectIdRef when the round
    // trip lands: a jump to another project in between must not install this
    // turn — its prompt line, its run id, its stop button — into a panel now
    // showing different work.
    const sentProjectId = projectId;
    setDraft("");
    setSendError(null);
    setSending(true);
    // Open the send window before the call, not after: the engine has already
    // minted this turn's run id by the time run() returns, and an event for
    // it can reach the listener before the response reaches us.
    pendingSendRef.current = true;
    agentApi
      .run(projectId, prompt, nodeId)
      .then((result) => {
        if (!mountedRef.current) {
          discardSendWindow();
          return;
        }
        if (projectIdRef.current !== sentProjectId) {
          // The writer jumped to another work while this send was in flight.
          // The turn is real and still running in the engine, but it belongs
          // to a conversation this panel is no longer showing, so nothing of
          // it is installed here and its events are refused from now on.
          //
          // The send window is deliberately NOT touched: the project-switch
          // reset already emptied and closed it, and if the writer has since
          // sent something in the new work, that window is theirs — closing
          // it here would drop their turn's opening events instead.
          abandonedRunsRef.current.add(result.run_id);
          return;
        }
        // Set the authoritative run id directly from agent.run's response,
        // bypassing acceptRun's first-wins null check. See the comment
        // above acceptRun: an id assigned here, before any wire event has
        // been judged, closes the window a stale straggler from the previous
        // run could otherwise be adopted through simply by arriving first.
        currentRunIdRef.current = result.run_id;
        setLines((prev) => [...prev, { kind: "user", id: `user:${result.run_id}`, text: prompt }]);
        setTurn(result.run_id);
        setSending(false);
        // Last, so a terminal event that raced the response gets to clear the
        // turn state this line just set, rather than being overwritten by it.
        flushSendWindow();
      })
      .catch((err: unknown) => {
        // A synchronous refusal (provider_not_configured,
        // provider_consent_required, agent_busy, ...) — no run ever started,
        // so currentRunIdRef is untouched and replaying what the window held
        // judges it exactly as it would have been judged without the window.
        // Hand the draft back rather than discarding what the writer typed.
        if (!mountedRef.current) {
          discardSendWindow();
          return;
        }
        // Same as the resolved case: a refusal of a turn for the work the
        // writer has left is not news in the work they are now looking at,
        // and handing them back a prompt they wrote for a different book
        // would be worse than dropping it. No run id was ever minted, so
        // there is nothing to abandon.
        if (projectIdRef.current !== sentProjectId) return;
        setSending(false);
        setSendError(rpcErrorMessage(err, t));
        setDraft(prompt);
        flushSendWindow();
      });
  }

  // agent.cancel reaches the in-memory turn; the partial reply already in
  // `lines` is left exactly as it is — the engine keeps it in the
  // transcript, and the composer returns to "send" only once this run's own
  // agent-cancelled (or agent-done, if the cancel lost the race) arrives.
  function handleStop() {
    const runId = turnRunId;
    if (!runId || canceling) return;
    setCanceling(true);
    agentApi
      .cancel(runId)
      .catch((err: unknown) => {
        if (!mountedRef.current) return;
        // Silent only in the case that is genuinely not news: the turn ended
        // on its own while the cancel was in flight, and its terminal event
        // has already handed the composer back. If the turn is still running,
        // nothing else will ever say the stop failed — the writer clicked it
        // and the reply keeps arriving — so it gets the same translated,
        // never-raw treatment as a refused send.
        if (turnRunIdRef.current !== runId) return;
        setSendError(rpcErrorMessage(err, t));
      })
      .finally(() => {
        if (mountedRef.current) setCanceling(false);
      });
  }

  function handleComposerKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key !== "Enter" || e.shiftKey) return;
    // Never steal the Enter that belongs to an IME. In Japanese it is the key
    // that commits the kanji conversion candidate, and in Korean the one that
    // commits the trailing syllable of anything not ending in punctuation —
    // consume it as send and there is no keystroke left that finishes the
    // composition without also sending, so the unconverted reading is what
    // gets submitted. Both signals are checked because browsers disagree:
    // `isComposing` is the modern one, keyCode 229 the one Safari and older
    // WebKit report instead (and the only one a synthetic jsdom event carries
    // unless the test sets isComposing by hand).
    if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229) return;
    e.preventDefault();
    handleSend();
  }

  useEffect(() => {
    let cancelled = false;
    providersApi
      .list()
      .then((rows) => {
        if (cancelled) return;
        const active = rows.find((row) => row.active);
        setReady(Boolean(active?.configured && active?.consented));
      })
      .catch(() => {
        // An unreachable engine is not "ready" either — fail closed to the
        // notice rather than to a blank panel.
        if (!cancelled) setReady(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Restore (Task 7): the conversation that was here before the panel
  // closed. #93 made the engine's turn outlive the RPC call, so a turn can
  // still be running when the panel reopens with nothing on screen yet —
  // and there is no run-status RPC to ask; the plan deliberately does not
  // add one (see the "restored" notice line below). Fires once `ready` is
  // known: fetching history for a panel about to show the "configure a
  // provider" notice instead of a log is pointless. Prepended rather than
  // replacing `lines`, on the small chance the writer already sent
  // something before this resolves.
  useEffect(() => {
    if (ready !== true) return;
    let cancelled = false;
    agentApi
      .history(projectId)
      .then((rows) => {
        if (cancelled) return;
        const restored = linesFromHistory(rows);
        // The only signal available for "is a turn still running": whether
        // the conversation ends in a settled turn. See turnMayBeRunning for
        // what the rows can and cannot say about that.
        if (turnMayBeRunning(rows)) {
          // Keyed on the row it follows rather than a fixed string. With the
          // project-switch reset above, no two restores can put a line in
          // `lines` at once any more, so this is belt and braces and no test
          // pins it — but a fixed key is what turned that missing reset into
          // two interleaved conversations rather than a visibly stale one,
          // and an id scheme every other Line already follows costs nothing.
          const last = rows[rows.length - 1];
          restored.push({ kind: "notice", id: `history:${last.id}:maybe-running`, variant: "restored" });
        }
        if (restored.length > 0) setLines((prev) => [...restored, ...prev]);
      })
      .catch(() => {
        // A failed restore is not worse than an empty panel — start fresh
        // rather than blocking the log on it.
      });
    return () => {
      cancelled = true;
    };
  }, [ready, projectId]);

  // Grow the composer with what is in it, up to the same cap App.css sets on
  // .cmp-input textarea. The starter chips fill in a whole sentence, so a
  // fixed one-line box would show the feature's own happy path through a
  // peephole. Collapse-then-measure is the standard autosize; jsdom reports
  // scrollHeight as 0, and the guard leaves the box at its CSS min-height
  // there rather than collapsing it to nothing.
  const composerRef = useRef<HTMLTextAreaElement | null>(null);
  useEffect(() => {
    const el = composerRef.current;
    if (!el) return;
    el.style.height = "auto";
    if (el.scrollHeight > 0) el.style.height = `${Math.min(el.scrollHeight, COMPOSER_MAX_HEIGHT)}px`;
  }, [draft, ready]);

  // A mount can now hold more than one assistant line — Task 6 lets the
  // writer send a second turn once the first is done. Only the run
  // currentRunIdRef is currently pointed at (the newest one, whether still
  // streaming or just finished) needs the smooth reveal; every earlier
  // turn's line already has its full text sitting in `lines` and is
  // rendered verbatim. Reading the ref here rather than `turnRunId` state
  // means this stays correct even when a test drives raw agent-delta events
  // straight past the composer, with no run id ever assigned by hand.
  const activeAssistantId = currentRunIdRef.current ? `assistant:${currentRunIdRef.current}` : null;
  const activeAssistantText = activeAssistantId
    ? lines.find((l): l is Extract<Line, { kind: "assistant" }> => l.kind === "assistant" && l.id === activeAssistantId)
        ?.text ?? ""
    : "";
  const shownActiveText = useSmoothStream(activeAssistantText, running);
  const busy = sending || turnRunId !== null;

  return (
    <aside className="panel agent-panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl"><span className="ic"><Bot size={16} /></span> {t("agentPanel.title")}</span>
        <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
      </div>
      {ready === null ? null : ready ? (
        <>
        <div className="panel-scroll agent-log cmp-stream" data-testid="agent-log">
          {lines.map((line) => {
            if (line.kind === "user") {
              return (
                <div key={line.id} className="msg user">
                  <div className="msg-bubble">{line.text}</div>
                </div>
              );
            }
            if (line.kind === "assistant") {
              const text = line.id === activeAssistantId ? shownActiveText : line.text;
              return (
                <div key={line.id} className="msg bot">
                  <div className="msg-bubble">
                    <Markdown text={text} />
                  </div>
                  {line.usage ? (
                    // No cost figure: prices differ per provider and change
                    // often, and a wrong number is worse than none.
                    <div className="msg-usage" data-testid="agent-usage">
                      {t("agentPanel.usage", { input: line.usage.input, output: line.usage.output })}
                    </div>
                  ) : null}
                </div>
              );
            }
            if (line.kind === "notice") {
              // Text resolves at render time, from `variant`/`reason` — the
              // same rule tool lines follow (see toolVerbKey/toolLabelKey
              // below) — so a language switch never leaves stale copy
              // sitting in state. `message` is never read here or anywhere
              // else on this Line: agent_internal_error's carries a raw Go
              // panic value, and reasonMessage only ever sees `reason`.
              const text =
                line.variant === "restored"
                  ? t("agentPanel.restore.mayBeRunning")
                  : line.variant === "failed"
                    ? t("agentPanel.restore.failed")
                    : line.variant === "cancelled"
                      ? t("agentPanel.cancelled")
                      : reasonMessage(line.reason, t);
              return (
                <div
                  key={line.id}
                  className={`agent-notice agent-notice-${line.variant}`}
                  data-testid="agent-notice"
                  role={line.variant === "error" ? "alert" : undefined}
                >
                  {text}
                  {line.variant === "error" && line.reason === "provider_auth_failed" ? (
                    <>
                      {" "}
                      <Link to="/settings">{t("agentPanel.openSettings")}</Link>
                    </>
                  ) : null}
                </div>
              );
            }
            // Tool line: one row for the started+done/error pair Task 4
            // already collapsed. Built entirely from the tool NAME — a verb
            // plus a short translated label. The event's `summary` is never
            // rendered in any state: on "started" it is the raw call
            // arguments (a whole scene body, for a write), and on
            // "done"/"error" it is the JSON the go-sdk serialised from the
            // tool's output, or an English engine sentence. See
            // lib/agentTools.ts for the full trace.
            const kind = toolKind(line.name);
            const labelKey = toolLabelKey(line.name);
            const showUndo = Boolean(line.batchId) && !line.undone && !line.undoError;
            return (
              <div key={line.id} className={`tool-line tool-${line.state}`} data-testid="tool-line">
                <span className="tool-label">{t(toolVerbKey(kind, line.state))}</span>
                {labelKey ? <span className="tool-name"> · {t(labelKey)}</span> : null}
                {showUndo ? (
                  <button
                    type="button"
                    className="tool-undo"
                    disabled={line.undoing}
                    onClick={() => handleUndo(line.id, line.batchId as string)}
                  >
                    {t("agentPanel.tool.undo")}
                  </button>
                ) : null}
                {line.undone ? <span className="tool-undone">{t("agentPanel.tool.undone")}</span> : null}
                {line.undoError ? (
                  <span className="tool-undo-error" role="alert">
                    {" "}
                    {line.undoError}
                  </span>
                ) : null}
              </div>
            );
          })}
        </div>
        {sendError ? (
          <p className="agent-send-error" role="alert" data-testid="agent-send-error">
            {sendError}
          </p>
        ) : null}
        {lines.length === 0 && !busy ? (
          // Starting chips: three fixed prompts that fill the composer and
          // stop there — the writer gets to edit the sentence before it goes
          // anywhere. They only make sense before the first turn; once the
          // log has something in it (or a send is already in flight), the
          // writer is already composing their own message.
          <div className="ai-chiprow agent-starters" data-testid="agent-starters">
            <button
              type="button"
              className="chip"
              data-testid="agent-starter-draftScene"
              onClick={() => setDraft(t("agentPanel.starters.draftScene.prompt"))}
            >
              {t("agentPanel.starters.draftScene.label")}
            </button>
            <button
              type="button"
              className="chip"
              data-testid="agent-starter-continuity"
              onClick={() => setDraft(t("agentPanel.starters.continuity.prompt"))}
            >
              {t("agentPanel.starters.continuity.label")}
            </button>
            <button
              type="button"
              className="chip"
              data-testid="agent-starter-nextScene"
              onClick={() => setDraft(t("agentPanel.starters.nextScene.prompt"))}
            >
              {t("agentPanel.starters.nextScene.label")}
            </button>
          </div>
        ) : null}
        <div className="cmp-input-wrap">
          <div className="cmp-input">
            <textarea
              data-testid="agent-composer"
              ref={composerRef}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={handleComposerKeyDown}
              // A placeholder is not an accessible name — it is announced as
              // a hint at best and disappears the moment anything is typed.
              // Both buttons beside this box carry one; the box the panel
              // exists for should too.
              aria-label={t("agentPanel.composer.label")}
              placeholder={t("agentPanel.composer.placeholder")}
              rows={2}
            />
            {busy ? (
              <button
                type="button"
                className="cmp-send"
                data-testid="agent-stop"
                onClick={handleStop}
                disabled={!turnRunId || canceling}
                aria-label={t("agentPanel.composer.stop")}
              >
                <Square size={14} />
              </button>
            ) : (
              <button
                type="button"
                className="cmp-send"
                data-testid="agent-send"
                onClick={handleSend}
                disabled={!draft.trim()}
                aria-label={t("agentPanel.composer.send")}
              >
                <Send size={14} />
              </button>
            )}
          </div>
        </div>
        </>
      ) : (
        <p className="agent-empty" data-testid="agent-unconfigured">
          {t("agentPanel.unconfigured")}{" "}
          <Link to="/settings">{t("agentPanel.openSettings")}</Link>
        </p>
      )}
    </aside>
  );
}
