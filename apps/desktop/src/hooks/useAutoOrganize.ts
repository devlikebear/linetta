import { useCallback, useEffect, useRef } from "react";
import { useIdleTimer } from "./useIdleTimer";

/** How long without typing counts as a long rest (#387). */
export const ORGANIZE_IDLE_MS = 10 * 60 * 1000;

export type OrganizeTrigger = "leave" | "idle";

interface Options {
  projectId: string | null;
  nodeId: string | null;
  /** The episode the open scene is in, as the outline shows it. */
  episodeId: string | null;
  /** agent.organize with a trigger: the engine runs it only when the writer
   *  has switched organizing on for that trigger. */
  organize: (projectId: string, nodeId: string, trigger: OrganizeTrigger) => Promise<unknown>;
  /** Saves the open scene first: the pass reads what is stored. */
  flush: () => Promise<void>;
}

/** Tells the engine about the two moments organizing may run on its own
 *  (#387): the writer leaving an episode for another, and a long rest after
 *  typing. It never interrupts: nothing is shown, and a refusal (a turn
 *  running, no provider) is let go — the scenes wait for the next moment.
 *  Returns markActivity, for every edit. */
export function useAutoOrganize({ projectId, nodeId, episodeId, organize, flush }: Options) {
  const fire = useCallback((pid: string, nid: string, trigger: OrganizeTrigger) => {
    void flush()
      .then(() => organize(pid, nid, trigger))
      .catch(() => {
        // Not the writer's request: nothing to report.
      });
  }, [organize, flush]);

  const last = useRef<{ projectId: string; nodeId: string; episodeId: string | null } | null>(null);
  useEffect(() => {
    if (!projectId || !nodeId) return;
    const prev = last.current;
    last.current = { projectId, nodeId, episodeId };
    if (prev && prev.projectId === projectId && prev.episodeId !== episodeId) {
      fire(prev.projectId, prev.nodeId, "leave");
    }
  }, [projectId, nodeId, episodeId, fire]);

  const where = useRef({ projectId, nodeId });
  where.current = { projectId, nodeId };
  const typed = useRef(false);
  const { markActivity: restart, cancel } = useIdleTimer(ORGANIZE_IDLE_MS, () => {
    const { projectId: pid, nodeId: nid } = where.current;
    if (!typed.current || !pid || !nid) return;
    typed.current = false;
    fire(pid, nid, "idle");
  });
  // A rest is counted in the scene it began in.
  useEffect(() => {
    typed.current = false;
    cancel();
  }, [nodeId, cancel]);

  return useCallback(() => {
    typed.current = true;
    restart();
  }, [restart]);
}
