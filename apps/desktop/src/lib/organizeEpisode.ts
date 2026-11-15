import type { OrganizeResult } from "./types";

export interface OrganizeDeps {
  /** agent.organize */
  organize: (projectId: string, nodeId: string, force?: boolean) => Promise<OrganizeResult>;
  /** Saves the open scene first: the pass reads what is stored. */
  flush: () => Promise<void>;
  confirm: (title: string) => Promise<boolean>;
  toast: (message: string) => void;
  t: (key: "workspace.organize.started" | "workspace.organize.unchanged" | "workspace.organize.nothing", vars: Record<string, string>) => string;
  errorMessage: (e: unknown) => string;
}

/** Asks for the episode nodeId is in to be read for what the work's record
 *  lacks (#387). When nothing changed since the last read, the writer chooses
 *  whether to pay for another. Reports how it went in a toast. */
export async function organizeEpisode(projectId: string, nodeId: string, d: OrganizeDeps): Promise<void> {
  try {
    await d.flush();
    let result = await d.organize(projectId, nodeId);
    if (!result.started && result.unchanged > 0) {
      if (!(await d.confirm(d.t("workspace.organize.unchanged", { episode: result.episode_label })))) return;
      result = await d.organize(projectId, nodeId, true);
    }
    d.toast(result.started
      ? d.t("workspace.organize.started", { episode: result.episode_label, count: String(result.scenes) })
      : d.t("workspace.organize.nothing", { episode: result.episode_label }));
  } catch (e) {
    d.toast(d.errorMessage(e));
  }
}
