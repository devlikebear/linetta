import { useEffect, useRef, useState } from "react";
import { agent as agentApi } from "../lib/rpc";
import { useEngineEvent } from "./useEngineEvent";

interface AgentEvent {
  project_id: string;
}

/** Whether the built-in agent has a turn running for `projectId`.
 *
 *  The panel knows this about itself, but the panel is not always there: a
 *  turn outlives it (#93), so a writer who closed it — or left the workspace
 *  and came back to it closed — has nothing on screen saying the agent is
 *  still rewriting their scenes. This is the toolbar's answer.
 *
 *  agent.status seeds it, since a turn may have started before this mounted;
 *  the wire keeps it current after that. */
export function useAgentRunning(projectId: string | undefined, enabled: boolean): boolean {
  const [runningFor, setRunningFor] = useState<string | null>(null);
  // Bumped by every event. A status answer that an event has overtaken is
  // older than what the event said, and must not overwrite it.
  const eventsRef = useRef(0);

  useEffect(() => {
    if (!enabled || !projectId) return;
    let cancelled = false;
    const seenAtCall = eventsRef.current;
    Promise.resolve()
      .then(() => agentApi.status(projectId))
      .then((status) => {
        if (cancelled || eventsRef.current !== seenAtCall) return;
        setRunningFor(status.running ? projectId : null);
      })
      .catch(() => {
        // No answer is not "running".
      });
    return () => {
      cancelled = true;
    };
  }, [projectId, enabled]);

  const alive = (payload: AgentEvent) => {
    if (payload.project_id !== projectId) return;
    eventsRef.current += 1;
    setRunningFor(payload.project_id);
  };
  const ended = (payload: AgentEvent) => {
    if (payload.project_id !== projectId) return;
    eventsRef.current += 1;
    setRunningFor(null);
  };
  useEngineEvent<AgentEvent>("agent-delta", alive);
  useEngineEvent<AgentEvent>("agent-tool", alive);
  useEngineEvent<AgentEvent>("agent-done", ended);
  useEngineEvent<AgentEvent>("agent-error", ended);
  useEngineEvent<AgentEvent>("agent-cancelled", ended);

  return enabled && Boolean(projectId) && runningFor === projectId;
}
