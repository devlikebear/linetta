import { useEffect, useRef, useState } from "react";
import { Bot, X } from "lucide-react";
import { Link } from "react-router-dom";
import { agent as agentApi, providers as providersApi } from "../../lib/rpc";
import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import { useEngineEvent } from "../../hooks/useEngineEvent";
import { useSmoothStream } from "../../hooks/useSmoothStream";
import { Markdown } from "./Markdown";
import "./AgentPanel.css";

/** The built-in agent's panel (#95).
 *
 *  Task 3 built the shell: it opens, it closes, and it says whether a
 *  provider is ready to talk to. Task 4 filled the log with the writer's
 *  turn, the agent's reply, and the reply arriving as it streams. This task
 *  (5) renders the tool line Task 4 kept but never showed, and adds undo for
 *  the lines that carry a batch id. No composer, no send button, no stop, no
 *  usage line, no history restore — those are Tasks 6-7.
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
 *  component's undo flow — they never arrive over the wire. */
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

/** Tools that mutate the manuscript, mirroring
 *  engine/internal/mcphost/tools_write.go's `WriteToolNames` — the only
 *  place that list is authoritative. This is what decides the line's verb
 *  (읽음/씀); it is a different question from whether the *button* shows,
 *  which is decided per-line by whether a `batch_id` actually came back (see
 *  the render loop below). A write tool can still render with no button —
 *  e.g. `linetta_write_scene`, which returns a snapshot id rather than a
 *  batch id, or any write cut off mid-flight by the writer pressing stop. */
const WRITE_TOOL_NAMES = new Set([
  "linetta_create_work",
  "linetta_write_scene",
  "linetta_write_summary",
  "linetta_revise_scene",
  "linetta_apply_story_ops",
  "linetta_create_checkpoint",
  "linetta_undo_last_change",
]);

function isWriteTool(name: string): boolean {
  return WRITE_TOOL_NAMES.has(name);
}

/** The line's verb, keyed by whether it is a write tool and by its resolved
 *  state. `"running"` reads as still-in-progress rather than reusing the
 *  past-tense done copy; `"error"` gets its own copy so it is legible
 *  without relying on color alone. */
function toolLabelKey(write: boolean, state: "running" | "ok" | "error"): MessageKey {
  if (state === "running") return write ? "agentPanel.tool.writing" : "agentPanel.tool.reading";
  if (state === "error") return write ? "agentPanel.tool.writeFailed" : "agentPanel.tool.readFailed";
  return write ? "agentPanel.tool.wrote" : "agentPanel.tool.read";
}

interface AgentDeltaPayload {
  run_id: string;
  text: string;
}

interface AgentToolPayload {
  run_id: string;
  name: string;
  state: "started" | "done" | "error";
  summary?: string;
  batch_id?: string;
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

interface Props {
  onClose: () => void;
}

export function AgentPanel({ onClose }: Props) {
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
  // call completes could still be adopted first. The composer, when it lands
  // and implements the send button, must set currentRunIdRef.current directly
  // from agent.run()'s returned run_id the moment it receives the response,
  // rather than leaving it to be decided by the first arriving event. Without
  // that step, a second turn in the same mount will silently drop all events
  // from the second run and render no output.
  function acceptRun(runId: string): boolean {
    if (currentRunIdRef.current === null) currentRunIdRef.current = runId;
    return currentRunIdRef.current === runId;
  }

  useEngineEvent<AgentDeltaPayload>("agent-delta", (payload) => {
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
  });

  useEngineEvent<AgentToolPayload>("agent-tool", (payload) => {
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
          // instead of dependent on the render loop hiding it correctly.
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
  });

  useEngineEvent<AgentDonePayload>("agent-done", (payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(false);
    const id = `assistant:${payload.run_id}`;
    setLines((prev) =>
      prev.map((l) => (l.kind === "assistant" && l.id === id ? { ...l, usage: payload.usage } : l)),
    );
  });

  // Translating `reason` into a message and showing it is Task 7's job.
  // This task only has to stop the streaming indicator honestly instead of
  // leaving the reply looking like it is still arriving.
  useEngineEvent<AgentErrorPayload>("agent-error", (payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(false);
  });

  useEngineEvent<AgentCancelledPayload>("agent-cancelled", (payload) => {
    if (!acceptRun(payload.run_id)) return;
    setRunning(false);
  });

  // Undo is a per-line action, not a turn-level one: mark only the clicked
  // line pending, then resolve it to undone or to a translated failure.
  // Failure — most commonly `agent_undo_unavailable`, since the service
  // keeps only the last 8 batches — is rendered beside the line and the
  // button is dropped; it never becomes a panel-level error.
  function handleUndo(id: string, batchId: string) {
    setLines((prev) =>
      prev.map((l) => (l.kind === "tool" && l.id === id ? { ...l, undoing: true, undoError: undefined } : l)),
    );
    agentApi
      .undo(batchId)
      .then(() => {
        setLines((prev) =>
          prev.map((l) => (l.kind === "tool" && l.id === id ? { ...l, undoing: false, undone: true } : l)),
        );
      })
      .catch((err: unknown) => {
        setLines((prev) =>
          prev.map((l) =>
            l.kind === "tool" && l.id === id ? { ...l, undoing: false, undoError: rpcErrorMessage(err, t) } : l,
          ),
        );
      });
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

  // At most one assistant line ever exists in a mount: acceptRun locks onto
  // a single run, and that run produces exactly one assistant reply. Feed
  // its raw accumulated text through useSmoothStream so the reveal is
  // decoupled from delta's chunky arrival rate.
  const assistantText = lines.find((l): l is Extract<Line, { kind: "assistant" }> => l.kind === "assistant")?.text ?? "";
  const shownAssistantText = useSmoothStream(assistantText, running);

  return (
    <aside className="panel agent-panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl"><span className="ic"><Bot size={16} /></span> {t("agentPanel.title")}</span>
        <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
      </div>
      {ready === null ? null : ready ? (
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
              return (
                <div key={line.id} className="msg bot">
                  <div className="msg-bubble">
                    <Markdown text={shownAssistantText} />
                  </div>
                </div>
              );
            }
            // Tool line: one row for the started+done/error pair Task 4
            // already collapsed. `summary` only renders once the call has
            // resolved — the "started" event's summary is the tool's raw
            // arguments (see runTool in engine/internal/agent/loop.go),
            // which can carry a full scene body, not a short phrase. Never
            // the full arguments, so a running line shows the verb alone.
            const write = isWriteTool(line.name);
            const showUndo = Boolean(line.batchId) && !line.undone && !line.undoError;
            return (
              <div key={line.id} className={`tool-line tool-${line.state}`} data-testid="tool-line">
                <span className="tool-label">{t(toolLabelKey(write, line.state))}</span>
                {line.state !== "running" && line.summary ? (
                  <span className="tool-summary"> · {line.summary}</span>
                ) : null}
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
      ) : (
        <p className="agent-empty" data-testid="agent-unconfigured">
          {t("agentPanel.unconfigured")}{" "}
          <Link to="/settings">{t("agentPanel.openSettings")}</Link>
        </p>
      )}
    </aside>
  );
}
