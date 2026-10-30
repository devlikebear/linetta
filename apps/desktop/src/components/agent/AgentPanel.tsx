import { Fragment, useEffect, useRef, useState } from "react";
import { Bot, Quote, Send, Square, X } from "lucide-react";
import { Link } from "react-router-dom";
import { agent as agentApi, providers as providersApi, settings as settingsApi } from "../../lib/rpc";
import type { AgentHistoryRow, AgentRunStatus, ProviderStatus } from "../../lib/types";
import { useI18n } from "../../lib/i18n";
import { toolKind, toolLabelKey, toolVerbKey } from "../../lib/agentTools";
import { reasonMessage, rpcErrorMessage } from "../../lib/rpcMessage";
import { useEngineEvent } from "../../hooks/useEngineEvent";
import { useSmoothStream } from "../../hooks/useSmoothStream";
import { isProviderReady } from "../../lib/providers";
import { Markdown } from "./Markdown";
import { ProviderPicker } from "./ProviderPicker";
import { IllustrationRequestCard } from "../illustration/IllustrationRequestCard";
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
  /** `selection` is the passage the writer sent along from the editor,
   *  drawn as a quote above what they asked about it. */
  | { kind: "user"; id: string; text: string; selection?: string }
  | {
      kind: "assistant";
      id: string;
      text: string;
      usage?: { input: number; output: number };
      /** Set on a restored line only: the run its row belongs to, so the
       *  usage of a turn this panel rejoined partway can find its reply. A
       *  live line carries the run in its id. */
      runId?: string;
    }
  | {
      kind: "tool";
      id: string;
      name: string;
      summary: string;
      state: "running" | "ok" | "error";
      batchId?: string;
      /** The pre-write snapshot a scene write or revise left behind — the
       *  same "something to undo" as batchId, just for prose instead of an
       *  outline batch (#112). A line carries at most one of the two. */
      snapshotId?: string;
      /** The illustration request linetta_request_illustration left; its
       *  approval card is drawn under the line (illustration Phase 4). */
      requestId?: string;
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
  /** The restored conversation stops mid-turn and agent.status says nothing is
   *  running: the turn was cut short without an ending — the app was closed
   *  on it. Unlike "restored" this is a statement, not a hedge. */
  | { kind: "notice"; id: string; variant: "interrupted" }
  | { kind: "notice"; id: string; variant: "cancelled" }
  | { kind: "notice"; id: string; variant: "failed" }
  | { kind: "notice"; id: string; variant: "error"; reason: string };

/** What every agent.* payload carries. `seq` numbers a turn's events from 1 in
 *  the order the engine emitted them; agent.status reports the last one, which
 *  is how a panel that rejoins a turn tells the events its snapshot already
 *  accounts for from the ones that come after it (see `rejoinedRef`). */
interface AgentEventBase {
  run_id: string;
  /** The work this event belongs to. See `forThisProject`. */
  project_id: string;
  seq?: number;
}

interface AgentDeltaPayload extends AgentEventBase {
  text: string;
}

interface AgentToolPayload extends AgentEventBase {
  name: string;
  state: "started" | "done" | "error";
  /** Never rendered. See the `summary` note on `Line` and lib/agentTools.ts:
   *  on every event, in both directions, this field is engine JSON or an
   *  English engine sentence. */
  summary?: string;
  batch_id?: string;
  /** The pre-write snapshot id a scene write or revise call resolves with —
   *  linetta_write_scene and linetta_revise_scene's counterpart to batch_id,
   *  which only linetta_apply_story_ops sets (#112). A call carries at most
   *  one of the two. */
  snapshot_id?: string;
  /** The scenes the call touched, as opaque node ids. The panel would like to
   *  name the scene on the line ("씀 · 4-2 씬"), but ids are not labels and
   *  this component has no tree to resolve them against — a lookup per tool
   *  event is a bigger change than this task should make. Rendering the id
   *  itself would be worse than rendering nothing, so the line carries the
   *  tool's label alone until a later task hands the panel the outline. */
  node_ids?: string[];
  /** linetta_request_illustration's request, for the approval card. */
  request_id?: string;
}

interface AgentDonePayload extends AgentEventBase {
  /** Never read, and deliberately kept — the same argument `summary` and
   *  `node_ids` above are kept on: the usage footer says what the turn cost,
   *  and which model produced it belongs on that line beside the token counts.
   *  Naming it needs copy in three catalogues and a decision about whether a
   *  writer wants a raw provider model id on screen at all, which is design
   *  this task does not have. Declaring the field is what makes the wire's
   *  shape readable here; the alternative is deleting it and rediscovering
   *  that agent.done carries it. */
  model?: string;
  usage: { input: number; output: number };
}

interface AgentErrorPayload extends AgentEventBase {
  reason: string;
  message: string;
}

type AgentCancelledPayload = AgentEventBase;

/** The composer's growth cap, in px. Must match `max-height` on
 *  `.cmp-input textarea` in App.css — the CSS enforces it for a box that has
 *  never been autosized, this constant for one that has. */
const COMPOSER_MAX_HEIGHT = 120;

/** What a `role === "tool"` history row's `content` decodes to — the exact
 *  wire shape agent.tool's own resolving event carries (see AgentToolPayload
 *  above), because the engine writes one straight from the other (see
 *  agent/transcript.go's toolEvent, appended by loop.go's runTool). Only the
 *  two fields the line cannot be drawn without are required here.
 *
 *  This is a hand copy of a Go struct's JSON tags, which is the hazard
 *  agentToolParity.test.ts already exists for one file over: rename `ok` to
 *  `success` in transcript.go and parseToolEvent returns null for every row,
 *  every restored tool line vanishes, and no panel test notices, because every
 *  fixture in them is hand-written JSON with the old names. agentToolParity's
 *  "restored tool row" block reads the Go source and feeds parseToolEvent a
 *  row built from the tags it actually finds there. */
interface RestoredToolEvent {
  name: string;
  ok: boolean;
  batch_id?: string;
  snapshot_id?: string;
  request_id?: string;
}

/** Parses one tool row's `content`. Null for anything that is not a usable
 *  toolEvent — malformed JSON, or JSON missing `name`/`ok` — so the caller
 *  can skip the row instead of crashing the whole restore over it.
 *
 *  Exported for the parity test only — nothing outside this file calls it. */
export function parseToolEvent(content: string): RestoredToolEvent | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(content);
  } catch {
    return null;
  }
  if (!parsed || typeof parsed !== "object") return null;
  const { name, ok, batch_id: batchId, snapshot_id: snapshotId, request_id: requestId } = parsed as Record<string, unknown>;
  if (typeof name !== "string" || typeof ok !== "boolean") return null;
  return {
    name,
    ok,
    batch_id: typeof batchId === "string" ? batchId : undefined,
    snapshot_id: typeof snapshotId === "string" ? snapshotId : undefined,
    request_id: typeof requestId === "string" ? requestId : undefined,
  };
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
/** The engine stores a passage sent with a prompt inside the prompt itself
 *  (agent/prompt.go withSelection), so later turns replay it too. Split it
 *  back out of a restored row so it reads as a quote, not as the writer's
 *  own sentence. Exported for the tests. */
export function splitSelection(content: string): { text: string; selection?: string } {
  const m = /^<selection>\n([\s\S]*?)\n<\/selection>\n\n/.exec(content);
  if (!m) return { text: content };
  return { text: content.slice(m[0].length), selection: m[1] };
}

function linesFromHistory(rows: AgentHistoryRow[], liveRunId?: string): Line[] {
  const lines: Line[] = [];
  for (let i = 0; i < rows.length; i++) {
    const row = rows[i];
    if (row.role === "user") {
      lines.push({ kind: "user", id: `history:${row.id}`, ...splitSelection(row.content) });
    } else if (row.role === "assistant") {
      lines.push({ kind: "assistant", id: `history:${row.id}`, text: row.content, runId: row.run_id });
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
          snapshotId: ev.snapshot_id,
          requestId: ev.request_id,
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
    // A run this panel is following live gets its ending from the wire, with
    // the reason the transcript does not keep.
    if (liveRunId && row.run_id === liveRunId) continue;
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
 *  distinguishes it (the row carries no "and it also asked for tools").
 *
 *  Whether a turn IS running is no longer this function's to guess —
 *  agent.status says. What is left for it is the other half: when nothing is
 *  running, whether the conversation stops in a place a finished turn would
 *  not have left it. A missed notice is the safe direction of that error. */
function turnMayBeRunning(rows: AgentHistoryRow[]): boolean {
  const last = rows[rows.length - 1];
  if (!last) return false;
  if (endNoticeVariant(last.status)) return false;
  return last.role !== "assistant";
}

/** The id of one assistant bubble. A turn's reply is not one message: the
 *  model says something, calls tools, and says something else, and each of
 *  those belongs where it happened in the log. `segment` counts the tool
 *  calls' interruptions, so text that arrives after a tool line opens a new
 *  bubble under it instead of growing the one above it. */
function assistantLineId(runId: string, segment: number): string {
  return segment > 0 ? `assistant:${runId}:${segment}` : `assistant:${runId}`;
}

/** Whether `line` is a bubble of `runId`'s reply — live or restored. */
function isReplyOf(line: Line, runId: string): line is Extract<Line, { kind: "assistant" }> {
  if (line.kind !== "assistant") return false;
  const base = `assistant:${runId}`;
  return line.runId === runId || line.id === base || line.id.startsWith(`${base}:`);
}

/** How long the current turn has been going, ticking once a second. Its own
 *  component so the tick re-renders one line rather than the whole log. */
function Elapsed({ since }: { since: number }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  const total = Math.max(0, Math.floor((now - since) / 1000));
  const seconds = String(total % 60).padStart(2, "0");
  return <span className="agent-working-time">{`${Math.floor(total / 60)}:${seconds}`}</span>;
}

/** How close to the end of the log still counts as "reading the latest" — the
 *  log follows new lines only while the writer is within this of the bottom. */
const FOLLOW_THRESHOLD = 48;

interface Props {
  onClose: () => void;
  /** The active project — agent.run's first argument. */
  projectId: string;
  /** The currently open editor's node id — agent.run's scope argument and
   *  the only material the engine gets for "which scene do you mean". */
  nodeId: string;
  /** Pre-fills the composer on mount without sending it — the Library
   *  import dialog's "시작" button lands here with the extraction prompt
   *  already typed, same as a starter chip. The writer still presses send. */
  initialDraft?: string;
  /** A passage the writer sent from the editor's selection menu. Each new
   *  id attaches it above the composer, replacing any passage already
   *  there; it goes out with the next send as agent.run's `selection`. It is
   *  shown and sent only while `nodeId` is still the scene it came from —
   *  the engine tells the agent the passage is in the open scene. */
  selectionRequest?: { id: number; text: string; nodeId: string } | null;
  /** Replaces the composer's text on an already-mounted panel — the
   *  forecast sheet's [개요 반영 제안 요청] button (story-assist Phase 4),
   *  which fills the same sentence a starter chip would but from a panel
   *  that does not own AgentPanel's draft state. Paired with
   *  externalDraftNonce: this only re-applies when the nonce changes, so
   *  typing over it is never silently reverted on an unrelated re-render. */
  externalDraft?: string;
  externalDraftNonce?: number;
  /** Called once the panel has applied externalDraft/externalDraftNonce to
   *  the composer. The caller's job is to clear whatever it fed in (set its
   *  own request state back to null) — this panel unmounts on close, so the
   *  nonce it applied is forgotten the moment the writer reopens it, and the
   *  same stale request would overwrite a fresh initialDraft (or whatever
   *  the writer had started typing) all over again. Without this callback
   *  the caller has no event to clear on, since nothing else tells it the
   *  request was consumed. */
  onExternalDraftApplied?: () => void;
  /** Opens Settings → image generation, for an illustration request card
   *  whose provider is not set up yet. */
  onOpenImageSettings?: () => void;
}

export function AgentPanel({
  onClose,
  projectId,
  nodeId,
  initialDraft,
  selectionRequest,
  externalDraft,
  externalDraftNonce,
  onExternalDraftApplied,
  onOpenImageSettings,
}: Props) {
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
  // story-assist Phase 2 (2.C.3): the continuity starter chip calls
  // linetta_report_continuity, which is absent when the writer has switched
  // the Review tool group off in Settings. Defaults to true — the on-by-
  // default switch — so the chip does not flash hidden on every open while
  // settings.get is still in flight.
  const [reviewToolsEnabled, setReviewToolsEnabled] = useState(true);
  // The rows `ready` was read from, kept for the provider picker under the
  // composer. Only ever replaced by a whole providers.list response.
  const [providerRows, setProviderRows] = useState<ProviderStatus[]>([]);
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
  const [draft, setDraft] = useState(initialDraft ?? "");
  const [attached, setAttached] = useState<{ text: string; nodeId: string } | null>(
    selectionRequest ? { text: selectionRequest.text, nodeId: selectionRequest.nodeId } : null,
  );
  const attachment = attached && attached.nodeId === nodeId ? attached : null;
  // See externalDraft's own doc comment above. The nonce, not the text, is
  // the dependency: a caller sending the identical sentence twice (two
  // clicks on the same forecast branch) must still land it the second time,
  // which a text-only dependency would skip as "unchanged".
  useEffect(() => {
    if (externalDraftNonce === undefined || externalDraft === undefined) return;
    setDraft(externalDraft);
    onExternalDraftApplied?.();
  }, [externalDraftNonce]); // eslint-disable-line react-hooks/exhaustive-deps
  const [sending, setSending] = useState(false);
  const [turnRunId, setTurnRunId] = useState<string | null>(null);
  // When the turn in progress started — the working line's clock. From the
  // engine for a turn this panel rejoined, from this panel's own send
  // otherwise.
  const [turnStartedAt, setTurnStartedAt] = useState<number | null>(null);
  const [canceling, setCanceling] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);
  // The same value as `turnRunId`, readable from an async callback that
  // closed over an older render — handleStop's rejection has to know whether
  // the turn is *still* running, and its `turnRunId` closure was captured
  // before the round trip started. Written only through setTurn below, so the
  // two can never disagree.
  const turnRunIdRef = useRef<string | null>(null);
  function setTurn(runId: string | null, startedAt?: number) {
    turnRunIdRef.current = runId;
    setTurnRunId(runId);
    setTurnStartedAt(runId === null ? null : startedAt ?? Date.now());
  }

  // The run this mount follows. handleSend below assigns it authoritatively
  // from agent.run's own response (see the comment there), and this first-wins
  // lock is the fallback for every event that arrives with the ref still null
  // — before this mount has sent anything of its own. Once picked, it must not
  // be overridden by a stray late event from some other run. #94 spent three
  // fix rounds on exactly this class of bug (a late response for an abandoned
  // selection overwriting the screen); this guard is built in from the start
  // rather than discovered later.
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

  // WHAT IS LEFT FOR THIS REF AND THE FENCE BELOW, NOW THAT THE WIRE SAYS
  // WHOSE AN EVENT IS.
  //
  // This ref and orphanedSendRef were both added (review rounds 1 and 2) to
  // keep one work's conversation out of another work's panel, back when a
  // payload carried a run id and nothing else. `forThisProject` closes that
  // leak by construction — the emitter states the owner, so no inference from
  // this mount's bookkeeping is needed for it, on a mount five minutes old or
  // five milliseconds old.
  //
  // What these two still do is narrower, and it is not a privacy leak: the
  // same-project A → B → A case. The writer starts a turn in A, jumps to B and
  // back; the reset cleared `lines`, restore has redrawn that turn's rows from
  // history, and the run — which #93 made outlive its RPC call — is still
  // emitting. Its events name A truthfully and the panel is showing A, so the
  // project check passes and must. What is wrong is only that its remaining
  // output would append beside the copy of itself the restore just drew.
  // Cosmetic, in one work, for one writer.
  //
  // Say so plainly here so a later simplification pass starts from the truth
  // rather than re-deriving it: nothing is deleted (each still has a test that
  // fails when it is removed, and trading a closed race for a deletion-induced
  // one is how the last three rounds went), but the job is a duplicate row, not
  // a leaked conversation.
  //
  // This ref holds the runs already named — the ids this panel knew when the
  // writer left. They stay refused even after a jump back, for the reason
  // above; the "may still be running" notice under the restored rows is what
  // that writer gets instead, which is what it is for.
  const abandonedRunsRef = useRef<Set<string>>(new Set());

  // Which bubble of a run's reply the next chunk of text belongs to — see
  // assistantLineId. Bumped by every tool call the run starts.
  const segmentRef = useRef<Map<string, number>>(new Map());

  // REJOINING A TURN THAT IS ALREADY RUNNING.
  //
  // A turn outlives the panel (#93): the writer closes it, or leaves the
  // workspace, and the engine keeps working. A panel that mounts in the
  // middle of one used to have only the transcript, which cannot say whether a
  // turn is still running — so it drew the rows, guessed ("may still be in
  // progress"), and offered a send button the engine would refuse.
  //
  // agent.status answers the question. The restore effect below reads it with
  // the transcript, and when a turn is running this panel adopts it: the stop
  // button, the working line, and the rest of its events.
  //
  // `rejoinedRef` is that snapshot's mark. Events of the run numbered at or
  // below `seq` are already in what was read and are dropped; and because the
  // snapshot is two reads that a busy turn can slip between, the run's ending
  // re-reads the transcript once so the log settles on exactly what the engine
  // recorded.
  const rejoinedRef = useRef<{ runId: string; seq: number } | null>(null);
  // Wire events are held from mount until the restore has decided what this
  // panel is looking at, then replayed in order. Applying one early would
  // adopt its run before the snapshot says where that run had got to.
  const restoringRef = useRef(true);
  const restoreHeldRef = useRef<Array<() => void>>([]);
  // The work on screen, for callbacks that resolve after a jump to another.
  const projectIdRef = useRef(projectId);
  projectIdRef.current = projectId;

  function acceptRun(runId: string): boolean {
    if (abandonedRunsRef.current.has(runId)) return false;
    // A send from a work the writer left is still out (see orphanedSendRef),
    // and its run id has not come back yet, so abandonedRunsRef cannot name it.
    // `forThisProject` has already refused anything belonging to another work,
    // so what reaches here is the same-project A → B → A case described above:
    // the event names the work on screen and the first-wins null check below
    // would adopt it as a turn of this visit's, appending its remaining output
    // beside the copy restore already drew. No turn of this visit's can exist
    // anyway — the same fence holds the next send — so refusing costs nothing.
    if (orphanedSendRef.current && currentRunIdRef.current === null) return false;
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

  // The send this panel is currently waiting on, as an identity token minted
  // per send. This — not the project id — is what handleSend's round trip
  // checks when it lands (#95 Task 7 review round 2, N1): a writer who jumps
  // away and back (A → B → A) is looking at the same project id again by the
  // time a send from before the first jump resolves, so an id comparison
  // waves that stale response through and it appends A's old prompt line,
  // overwrites the run id and the turn the newer send installed, and flushes
  // a window that is not its own. The reset below clears the token instead,
  // and nothing can put it back.
  const sendTokenRef = useRef<object | null>(null);

  // A send whose work the writer left while its round trip was still out.
  // Closing that send's window is not enough on its own: until the response
  // arrives the abandoned run has no id here, so an event carrying it is
  // judged against a currentRunIdRef the reset just set to null and gets
  // adopted as the NEW work's turn — the same leak abandonedRunsRef closes
  // for runs that are already named, through the one door that stays open
  // while the name is still in the post.
  //
  // So the send itself is fenced rather than just its window. While this is
  // true acceptRun adopts nothing, and the composer holds the next send:
  // letting a second one out would leave two round trips outstanding whose
  // responses can land in either order, each overwriting the other's run id,
  // turn and window. It comes down in handleSend's own callbacks — the first
  // instant anything here can tell the two runs apart. Mirrored in state
  // because the send button's disabled-ness reads it.
  const orphanedSendRef = useRef(false);
  const [orphanedSend, setOrphanedSend] = useState(false);
  function clearOrphanedSend() {
    orphanedSendRef.current = false;
    if (mountedRef.current) setOrphanedSend(false);
  }

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
    // A turn may still be running for the work being left. Refuse its events
    // from here on rather than letting the cleared currentRunIdRef adopt them.
    if (currentRunIdRef.current) abandonedRunsRef.current.add(currentRunIdRef.current);
    currentRunIdRef.current = null;
    pendingToolRef.current.clear();
    toolOccurrenceRef.current.clear();
    segmentRef.current.clear();
    // The next work's restore decides afresh; until it has, hold its events.
    rejoinedRef.current = null;
    restoringRef.current = true;
    restoreHeldRef.current = [];
    // A send for the work being left is still out. Its run id cannot be named
    // yet, so the send is fenced instead — read before discardSendWindow,
    // which is what clears the flag this asks. (A fence raised by an earlier
    // jump must also stay up until that send actually lands, which is what
    // the second half of the condition is for.)
    //
    // Reading `orphanedSendRef` as well as `pendingSendRef` is what makes the
    // raise idempotent, and that is load-bearing under StrictMode (on in
    // main.tsx). React double-invokes the render body and discards the
    // queued state updates of the first pass — but NOT its ref mutations, and
    // discardSendWindow() below clears pendingSendRef in that same body. So a
    // pendingSendRef-only condition is true on the first pass and false on the
    // second: setOrphanedSend(true) is queued, then thrown away, while
    // orphanedSendRef stays true. The fence still refuses (nothing leaks) but
    // the send button no longer says so — the writer sees a live control,
    // presses it, and nothing happens, which is the exact failure the mirror
    // exists to prevent.
    if (pendingSendRef.current || orphanedSendRef.current) {
      orphanedSendRef.current = true;
      setOrphanedSend(true);
    }
    // Nothing in flight belongs to this panel any more; handleSend's round
    // trip disowns itself by finding its token gone.
    sendTokenRef.current = null;
    // Held events belong to the run being abandoned, and the send that opened
    // the window (if any) is fenced above and disowned below in handleSend.
    discardSendWindow();
    setLines([]);
    setRunning(false);
    setTurn(null);
    setCanceling(false);
    setSending(false);
    setSendError(null);
  }

  /** True when a wire event belongs to the work this panel is showing.
   *
   *  Every agent.* notification names its project (engine/internal/agent/
   *  loop.go), because a run id alone does not say whose conversation an
   *  event is. Everything else in this file that answers that question —
   *  `abandonedRunsRef`, `orphanedSendRef`, `sendTokenRef`, the reset — is
   *  reconstructing it from what this mount happens to remember doing, and
   *  each of those covers a different interval: the ones whose run is already
   *  named, the ones whose name is still in the post. A fresh mount remembers
   *  none of it, which is why closing the panel during a turn, jumping to
   *  another work and reopening streamed the first work's prose into the
   *  second's log even with all three in place.
   *
   *  This is the fact itself rather than an inference from it, so it holds on
   *  a mount that has just been created as readily as on one that has been
   *  open all along. */
  function forThisProject(payload: { project_id: string }): boolean {
    return payload.project_id === projectId;
  }

  /** Wraps a wire handler so anything arriving inside the send window waits
   *  for a run id to judge it against, instead of being judged against the
   *  previous turn's.
   *
   *  The project check comes first, ahead of the window: an event for another
   *  work must not even be held, or the flush would replay it into this
   *  work's log the moment this panel's own send lands. */
  function whileNotSending<T extends AgentEventBase>(apply: (payload: T) => void) {
    const handle = (payload: T) => {
      if (!forThisProject(payload)) return;
      if (restoringRef.current) {
        restoreHeldRef.current.push(() => handle(payload));
        return;
      }
      // Already in the snapshot this panel rejoined the run from.
      const rejoined = rejoinedRef.current;
      if (rejoined && payload.run_id === rejoined.runId && payload.seq !== undefined && payload.seq <= rejoined.seq) {
        return;
      }
      if (deferDuringSend(() => apply(payload))) return;
      apply(payload);
    };
    return handle;
  }

  /** Ends the restore hold and replays what it held, in arrival order. */
  function releaseRestoreHold() {
    restoringRef.current = false;
    const held = restoreHeldRef.current;
    restoreHeldRef.current = [];
    for (const replay of held) replay();
  }

  /** Settles the log on the transcript once a rejoined run has ended.
   *
   *  Only while that run is still the last thing this panel did: once the
   *  writer has sent another turn, the log holds lines the re-read would draw
   *  a second time, and a cosmetic gap in an older turn is the better trade. */
  function settleRejoinedRun(runId: string, usage?: { input: number; output: number }) {
    if (rejoinedRef.current?.runId !== runId) return;
    rejoinedRef.current = null;
    const forProject = projectId;
    agentApi
      .history(forProject)
      .then((rows) => {
        if (!mountedRef.current || projectIdRef.current !== forProject) return;
        if (currentRunIdRef.current !== runId || pendingSendRef.current) return;
        const settled = linesFromHistory(rows, runId);
        if (usage) {
          for (let i = settled.length - 1; i >= 0; i--) {
            const line = settled[i];
            if (isReplyOf(line, runId)) {
              settled[i] = { ...line, usage };
              break;
            }
          }
        }
        setLines((prev) => [
          ...settled,
          // How the run ended, as the wire said it — the transcript keeps a
          // status, not the reason.
          ...prev.filter((l) => l.kind === "notice" && l.id.startsWith(`notice:${runId}:`)),
        ]);
      })
      .catch(() => {
        // The log as it stands is still a true account, just not a settled one.
      });
  }

  useEngineEvent<AgentDeltaPayload>("agent-delta", whileNotSending<AgentDeltaPayload>((payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(true);
    const id = assistantLineId(payload.run_id, segmentRef.current.get(payload.run_id) ?? 0);
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
      // Whatever the model says next comes after this line, in a bubble of its
      // own; and the text before it has finished arriving.
      segmentRef.current.set(payload.run_id, (segmentRef.current.get(payload.run_id) ?? 0) + 1);
      setRunning(false);
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
          snapshotId: payload.snapshot_id,
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
              snapshotId: payload.snapshot_id ?? l.snapshotId,
              requestId: payload.request_id ?? l.requestId,
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
    // The turn's cost goes under the last thing it said.
    setLines((prev) => {
      for (let i = prev.length - 1; i >= 0; i--) {
        if (!isReplyOf(prev[i], payload.run_id)) continue;
        const next = [...prev];
        next[i] = { ...(prev[i] as Extract<Line, { kind: "assistant" }>), usage: payload.usage };
        return next;
      }
      return prev;
    });
    settleRejoinedRun(payload.run_id, payload.usage);
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
    settleRejoinedRun(payload.run_id);
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
    settleRejoinedRun(payload.run_id);
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

  // batchId/snapshotId: exactly one is set, mirroring the two sources a tool
  // line's `showUndo` reads below — a structural batch from
  // linetta_apply_story_ops, or a scene write/revise's pre-write snapshot.
  function handleUndo(id: string, batchId: string | undefined, snapshotId: string | undefined) {
    // No `undoError: undefined` reset here: `showUndo` in the render loop
    // requires `!line.undoError`, so a line that already has one has no
    // button to click and can never reach this function.
    setLines((prev) => prev.map((l) => (l.kind === "tool" && l.id === id ? { ...l, undoing: true } : l)));
    agentApi
      .undo(batchId, snapshotId)
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
    // The fence: a send for a work the writer has left is still out, and its
    // response can still overwrite this one's run id, turn and window when it
    // lands. Held, not dropped — the draft stays in the composer, the send
    // button is disabled meanwhile, and agent.run resolves as soon as the
    // engine has minted the run id, so this is the same wait the composer
    // already does for a send of its own.
    if (!prompt || sending || turnRunId !== null || orphanedSendRef.current) return;
    // This send's identity, checked when the round trip lands. A project
    // switch clears the ref, and no later send can put this token back — so a
    // jump away and back cannot make a stale response look current again, the
    // way comparing project ids could.
    const token = {};
    sendTokenRef.current = token;
    const sentAttachment = attachment;
    setDraft("");
    setAttached(null);
    setSendError(null);
    setSending(true);
    followRef.current = true;
    // Open the send window before the call, not after: the engine has already
    // minted this turn's run id by the time run() returns, and an event for
    // it can reach the listener before the response reaches us.
    pendingSendRef.current = true;
    agentApi
      .run(projectId, prompt, nodeId, sentAttachment?.text)
      .then((result) => {
        if (sendTokenRef.current !== token) {
          // The writer jumped to another work while this send was in flight
          // — and may since have jumped back, which is why the token, not the
          // project id, is what knows. The turn is real and still running in
          // the engine, but it belongs to a conversation this panel is no
          // longer showing, so nothing of it is installed here: no prompt
          // line, no run id, no turn for the stop button. From here its
          // events are refused by name, so the fence that stood in for that
          // name while it was unknown comes down.
          //
          // The send window is deliberately NOT touched: the project-switch
          // reset already emptied and closed it, and if the writer has since
          // sent something in the new work, that window is theirs — closing
          // it here would drop their turn's opening events instead.
          abandonedRunsRef.current.add(result.run_id);
          clearOrphanedSend();
          return;
        }
        if (!mountedRef.current) {
          discardSendWindow();
          return;
        }
        // Set the authoritative run id directly from agent.run's response,
        // bypassing acceptRun's first-wins null check. See the comment
        // above acceptRun: an id assigned here, before any wire event has
        // been judged, closes the window a stale straggler from the previous
        // run could otherwise be adopted through simply by arriving first.
        currentRunIdRef.current = result.run_id;
        setLines((prev) => [
          ...prev,
          { kind: "user", id: `user:${result.run_id}`, text: prompt, selection: sentAttachment?.text },
        ]);
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
        // Same as the resolved case: a refusal of a turn for the work the
        // writer has left is not news in the work they are now looking at,
        // and handing them back a prompt they wrote for a different book
        // would be worse than dropping it. No run id was ever minted, so
        // there is nothing to abandon — only the fence to take down.
        if (sendTokenRef.current !== token) {
          clearOrphanedSend();
          return;
        }
        if (!mountedRef.current) {
          discardSendWindow();
          return;
        }
        setSending(false);
        setSendError(rpcErrorMessage(err, t));
        setDraft(prompt);
        // A passage attached since takes precedence over the one this send
        // carried, the same as a new selection replaces an old one.
        if (sentAttachment) setAttached((cur) => cur ?? sentAttachment);
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
        setProviderRows(rows);
        setReady(isProviderReady(rows.find((row) => row.active)));
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

  useEffect(() => {
    let cancelled = false;
    settingsApi
      .get()
      .then((s) => {
        if (cancelled) return;
        setReviewToolsEnabled(s.review_tools_enabled !== false);
      })
      .catch(() => {
        // Fail open: the chip's own tool call would refuse if the group
        // really is off, which is a clearer failure than hiding a chip the
        // writer expects to see over a transient settings.get error.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Restore (Task 7): the conversation that was here before the panel
  // closed. #93 made the engine's turn outlive the RPC call, so a turn can
  // still be running when the panel reopens with nothing on screen yet —
  // agent.status says whether one is, and the panel rejoins it (see
  // rejoinedRef). Fires once `ready` is
  // known: fetching history for a panel about to show the "configure a
  // provider" notice instead of a log is pointless. Prepended rather than
  // replacing `lines`, on the small chance the writer already sent
  // something before this resolves.
  useEffect(() => {
    if (ready !== true) return;
    let cancelled = false;
    // Null when the engine cannot say (the call failed): the restore then
    // falls back to what the rows alone suggest, the way it always did.
    const readStatus = (): Promise<AgentRunStatus | null> =>
      Promise.resolve()
        .then(() => agentApi.status(projectId))
        .catch(() => null);
    const sameMoment = (a: AgentRunStatus, b: AgentRunStatus) =>
      a.running === b.running && a.run_id === b.run_id && a.seq === b.seq;

    (async () => {
      let status = await readStatus();
      let rows = await agentApi.history(projectId);
      // Two reads are not one: a turn can write a row between them. Read the
      // status again and, if the turn moved, the transcript with it — a turn
      // busy enough to move every time is settled when it ends instead.
      for (let attempt = 0; attempt < 2 && status?.running; attempt++) {
        const again = await readStatus();
        if (!again || sameMoment(status, again)) break;
        status = again;
        rows = await agentApi.history(projectId);
      }
      if (cancelled) return;

      // A turn is running and nothing of this visit's is in the way: rejoin
      // it. (A send still out for a work the writer left keeps its fence —
      // see orphanedSendRef — and that turn stays the old hedge.)
      const runId = status?.running ? status.run_id : undefined;
      const rejoin =
        status && runId && !orphanedSendRef.current && !pendingSendRef.current && currentRunIdRef.current === null;
      const restored = linesFromHistory(rows, rejoin ? runId : undefined);
      if (status && runId && rejoin) {
        abandonedRunsRef.current.delete(runId);
        currentRunIdRef.current = runId;
        rejoinedRef.current = { runId, seq: status.seq };
        // The reply streamed so far that is not a transcript row yet.
        if (status.text) {
          restored.push({ kind: "assistant", id: assistantLineId(runId, 0), text: status.text });
        }
        // The tool it is inside, drawn as running so the event that resolves
        // it has a line to land on.
        if (status.tool) {
          const key = `${runId}:${status.tool}`;
          const id = `${key}:1`;
          toolOccurrenceRef.current.set(key, 1);
          pendingToolRef.current.set(key, id);
          segmentRef.current.set(runId, 1);
          restored.push({ kind: "tool", id, name: status.tool, summary: "", state: "running" });
        }
        setTurn(runId, status.started_at);
        setRunning(Boolean(status.text) && !status.tool);
      } else if (turnMayBeRunning(rows)) {
        // Keyed on the row it follows rather than a fixed string, the id
        // scheme every other Line follows.
        const last = rows[rows.length - 1];
        // The engine says nothing is running, so a conversation that stops
        // mid-turn was cut short. Without that answer — or with a turn this
        // panel could not rejoin — the old hedge is the honest line.
        const variant = status && !status.running ? "interrupted" : "restored";
        restored.push({ kind: "notice", id: `history:${last.id}:${variant}`, variant });
      }
      if (restored.length > 0) setLines((prev) => [...restored, ...prev]);
      // A reopened conversation opens on its latest line, not its first.
      followRef.current = true;
      releaseRestoreHold();
    })().catch(() => {
      // A failed restore is not worse than an empty panel — start fresh
      // rather than blocking the log on it.
      if (!cancelled) releaseRestoreHold();
    });
    return () => {
      cancelled = true;
    };
  }, [ready, projectId]);

  // Follow the conversation as it grows, unless the writer has scrolled up to
  // read something — then stay put until they come back down.
  const logRef = useRef<HTMLDivElement | null>(null);
  const followRef = useRef(true);
  function handleLogScroll() {
    const el = logRef.current;
    if (!el) return;
    followRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < FOLLOW_THRESHOLD;
  }

  // Grow the composer with what is in it, up to the same cap App.css sets on
  // .cmp-input textarea. The starter chips fill in a whole sentence, so a
  // fixed one-line box would show the feature's own happy path through a
  // peephole. Collapse-then-measure is the standard autosize; jsdom reports
  // scrollHeight as 0, and the guard leaves the box at its CSS min-height
  // there rather than collapsing it to nothing.
  const composerRef = useRef<HTMLTextAreaElement | null>(null);

  // A passage sent while the panel is already open. The one the panel
  // mounted with is already in `attached`; the ref is what tells the two
  // apart, since the request object is the same on every later render.
  const seenSelectionIdRef = useRef(selectionRequest?.id);
  useEffect(() => {
    if (!selectionRequest || selectionRequest.id === seenSelectionIdRef.current) return;
    seenSelectionIdRef.current = selectionRequest.id;
    setAttached({ text: selectionRequest.text, nodeId: selectionRequest.nodeId });
  }, [selectionRequest]);
  // The writer came here to type a question about the passage. `ready`
  // because the composer does not exist until the provider check lands.
  useEffect(() => {
    if (selectionRequest && ready) composerRef.current?.focus();
  }, [selectionRequest, ready]);
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
  const activeAssistantId = currentRunIdRef.current
    ? assistantLineId(currentRunIdRef.current, segmentRef.current.get(currentRunIdRef.current) ?? 0)
    : null;
  const activeAssistantText = activeAssistantId
    ? lines.find((l): l is Extract<Line, { kind: "assistant" }> => l.kind === "assistant" && l.id === activeAssistantId)
        ?.text ?? ""
    : "";
  const shownActiveText = useSmoothStream(activeAssistantText, running);
  const busy = sending || turnRunId !== null;
  // What the turn is doing, for the working line: inside a tool, writing its
  // reply, or neither — which is the model thinking, or composing a tool call
  // that can take minutes for a whole scene and streams nothing meanwhile.
  const toolRunning = lines.some((l) => l.kind === "tool" && l.state === "running");
  const workingKey = toolRunning
    ? "agentPanel.working.tool"
    : running
      ? "agentPanel.working.writing"
      : "agentPanel.working.thinking";

  useEffect(() => {
    const el = logRef.current;
    if (el && followRef.current) el.scrollTop = el.scrollHeight;
  }, [lines, shownActiveText, busy]);

  return (
    <aside className="panel agent-panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl"><span className="ic"><Bot size={16} /></span> {t("agentPanel.title")}</span>
        <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
      </div>
      {ready === null ? null : ready ? (
        <>
        <div
          className="panel-scroll agent-log cmp-stream"
          data-testid="agent-log"
          ref={logRef}
          onScroll={handleLogScroll}
        >
          {lines.map((line) => {
            if (line.kind === "user") {
              return (
                <div key={line.id} className="msg user">
                  {line.selection ? (
                    <blockquote className="agent-selection-quote" data-testid="agent-user-selection">
                      {line.selection}
                    </blockquote>
                  ) : null}
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
                  : line.variant === "interrupted"
                    ? t("agentPanel.restore.interrupted")
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
            // Revertible whenever the line carries something to undo — a
            // structural batch id, or (#112) a scene write/revise's pre-write
            // snapshot id. Which one is present decides how handleUndo calls
            // agent.undo; the writer sees the same button either way.
            const showUndo = Boolean(line.batchId || line.snapshotId) && !line.undone && !line.undoError;
            return (
              <Fragment key={line.id}>
                <div className={`tool-line tool-${line.state}`} data-testid="tool-line">
                  <span className="tool-label">{t(toolVerbKey(kind, line.state))}</span>
                  {labelKey ? <span className="tool-name"> · {t(labelKey)}</span> : null}
                  {showUndo ? (
                    <button
                      type="button"
                      className="tool-undo"
                      disabled={line.undoing}
                      onClick={() => handleUndo(line.id, line.batchId, line.snapshotId)}
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
                {line.requestId ? (
                  <IllustrationRequestCard requestId={line.requestId} onOpenSettings={onOpenImageSettings} />
                ) : null}
              </Fragment>
            );
          })}
          {busy ? (
            // The one line that says a turn is alive. Without it a turn that
            // is thinking, or writing a scene into a tool call, looks exactly
            // like one that has stopped: nothing streams for minutes.
            <div className="agent-working" data-testid="agent-working" role="status">
              <span className="agent-working-dots" aria-hidden="true">
                <i />
                <i />
                <i />
              </span>
              <span>{t(workingKey)}</span>
              {turnStartedAt !== null ? <Elapsed since={turnStartedAt} /> : null}
            </div>
          ) : null}
        </div>
        {sendError ? (
          <p className="agent-send-error" role="alert" data-testid="agent-send-error">
            {sendError}
          </p>
        ) : null}
        {attachment && !busy && !draft.trim() ? (
          // With a passage attached, the useful starts are about that
          // passage. Same contract as the chips below: they fill the
          // composer, and the writer still presses send.
          <div className="ai-chiprow agent-starters" data-testid="agent-selection-starters">
            {(["polish", "feedback", "expand"] as const).map((key) => (
              <button
                key={key}
                type="button"
                className="chip"
                data-testid={`agent-selection-starter-${key}`}
                onClick={() => {
                  setDraft(t(`agentPanel.selectionStarters.${key}.prompt`));
                  composerRef.current?.focus();
                }}
              >
                {t(`agentPanel.selectionStarters.${key}.label`)}
              </button>
            ))}
          </div>
        ) : lines.length === 0 && !busy ? (
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
            {reviewToolsEnabled ? (
              <button
                type="button"
                className="chip"
                data-testid="agent-starter-checkContinuity"
                onClick={() => setDraft(t("agentPanel.starters.checkContinuity.prompt"))}
              >
                {t("agentPanel.starters.checkContinuity.label")}
              </button>
            ) : null}
            {reviewToolsEnabled ? (
              <button
                type="button"
                className="chip"
                data-testid="agent-starter-fillStoryWorld"
                onClick={() => setDraft(t("agentPanel.starters.fillStoryWorld.prompt"))}
              >
                {t("agentPanel.starters.fillStoryWorld.label")}
              </button>
            ) : null}
            <button
              type="button"
              className="chip"
              data-testid="agent-starter-nextScene"
              onClick={() => setDraft(t("agentPanel.starters.nextScene.prompt"))}
            >
              {t("agentPanel.starters.nextScene.label")}
            </button>
            {reviewToolsEnabled ? (
              <button
                type="button"
                className="chip"
                data-testid="agent-starter-forecastNext"
                onClick={() => setDraft(t("agentPanel.starters.forecastNext.prompt"))}
              >
                {t("agentPanel.starters.forecastNext.label")}
              </button>
            ) : null}
          </div>
        ) : null}
        <div className="cmp-input-wrap">
          {attachment ? (
            <div className="agent-attachment" data-testid="agent-attachment">
              <Quote size={12} aria-hidden="true" />
              <span className="agent-attachment-text" title={attachment.text}>
                {attachment.text}
              </span>
              <button
                type="button"
                className="agent-attachment-remove"
                data-testid="agent-attachment-remove"
                onClick={() => setAttached(null)}
                aria-label={t("agentPanel.attachment.remove")}
              >
                <X size={12} />
              </button>
            </div>
          ) : null}
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
              placeholder={t(attachment ? "agentPanel.composer.placeholderSelection" : "agentPanel.composer.placeholder")}
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
                // Also while a send for a work the writer left is still out
                // (orphanedSendRef): handleSend refuses one anyway, and a
                // control that silently does nothing is worse than one that
                // says it cannot yet.
                disabled={!draft.trim() || orphanedSend}
                aria-label={t("agentPanel.composer.send")}
              >
                <Send size={14} />
              </button>
            )}
          </div>
          <ProviderPicker
            rows={providerRows}
            disabled={busy}
            // The picker only offers providers that are ready, so a change
            // here keeps `ready` true and the conversation on screen; the
            // rows are adopted for what the picker shows next.
            onRows={setProviderRows}
            onError={(err) => setSendError(rpcErrorMessage(err, t))}
          />
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
