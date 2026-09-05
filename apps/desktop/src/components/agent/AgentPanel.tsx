import { useEffect, useRef, useState } from "react";
import { Bot, Send, Square, X } from "lucide-react";
import { Link } from "react-router-dom";
import { agent as agentApi, providers as providersApi } from "../../lib/rpc";
import { useI18n } from "../../lib/i18n";
import { toolKind, toolLabelKey, toolVerbKey } from "../../lib/agentTools";
import { rpcErrorMessage } from "../../lib/rpcMessage";
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
 *  the lines that carry a batch id. This task (6) adds the composer, the
 *  stop button, the three starting chips, and the usage line at the end of a
 *  turn — the panel can now actually send something. No history restore and
 *  no agent.error/agent.cancelled rendering — those are Task 7.
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
    };

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
  function acceptRun(runId: string): boolean {
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

  // Translating `reason` into a message and showing it is Task 7's job.
  // This task only has to stop the streaming indicator honestly instead of
  // leaving the reply looking like it is still arriving, and hand the
  // composer back to the writer.
  useEngineEvent<AgentErrorPayload>("agent-error", whileNotSending<AgentErrorPayload>((payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(false);
    setTurn(null);
    setCanceling(false);
  }));

  useEngineEvent<AgentCancelledPayload>("agent-cancelled", whileNotSending<AgentCancelledPayload>((payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(false);
    setTurn(null);
    setCanceling(false);
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
        // Set the authoritative run id directly from agent.run's response,
        // bypassing acceptRun's first-wins null check. See the comment
        // above acceptRun: an id assigned here, before any wire event has
        // been judged, closes the window a stale straggler from the previous
        // run could otherwise be adopted through simply by arriving first.
        currentRunIdRef.current = result.run_id;
        if (!mountedRef.current) {
          discardSendWindow();
          return;
        }
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
