import { useEffect, useRef, useState } from "react";
import { Bot, X } from "lucide-react";
import { Link } from "react-router-dom";
import { providers as providersApi } from "../../lib/rpc";
import { useI18n } from "../../lib/i18n";
import { useEngineEvent } from "../../hooks/useEngineEvent";
import { useSmoothStream } from "../../hooks/useSmoothStream";
import { Markdown } from "./Markdown";
import "./AgentPanel.css";

/** The built-in agent's panel (#95).
 *
 *  Task 3 built the shell: it opens, it closes, and it says whether a
 *  provider is ready to talk to. This task (4) fills the log with the
 *  writer's turn, the agent's reply, and the reply arriving as it streams.
 *  No composer, no send button, no stop, no rendered tool lines, no undo, no
 *  usage line, no history restore — those are Tasks 5-7.
 */

/** One line in the transcript. Mirrors the shape in the task-4 brief.
 *
 *  A tool line collapses two wire events (`state: "started"` then `"done"`
 *  or `"error"`) into one row. There is no per-call id on `agent.tool`'s
 *  payload — only `run_id` and `name` — so `id` is built from the one
 *  correlator that exists: run id + tool name + how many times that name
 *  has started in this turn. Task 5 renders the collapsed row; this task
 *  only has to keep the pairing from getting lost. */
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
      undone?: boolean;
    };

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
          summary: payload.summary ?? "",
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
            // Tool lines: Task 5 collapses them into one visible row. This
            // task only keeps their state (see the useEngineEvent handlers
            // above) without rendering anything for them yet.
            return null;
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
