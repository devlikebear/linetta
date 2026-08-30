import { mcp, settings as settingsApi } from "./rpc";
import type { McpStatus, McpTokenResult, SettingsPatch } from "./types";

/** The one enable/disable sequence for MCP, shared by the workspace toggle and
 *  the settings pane so the two entrances cannot drift apart.
 *
 *  Order matters in both directions:
 *  - enable persists the mode/scope FIRST, because `mcp.enable` restarts the
 *    host from whatever the settings store holds;
 *  - disable persists `off` BEFORE stopping the listener (#74). If the stop
 *    fails after the write, a restart converges on "off"; the reverse order
 *    leaves a stopped listener that quietly resurrects on the next launch.
 */

export type EnableMcpOptions = {
  /** Access mode to persist; the workspace toggle always passes "full". */
  mode?: string;
  port?: number;
  /** Work to scope the server to; empty string opens every work. */
  projectId?: string;
  /** Record the data-sharing consent in the same patch (first enable from the
   *  workspace toggle, where the consent sentence is shown inline). */
  grantConsent?: boolean;
};

export async function enableMcp(opts: EnableMcpOptions = {}): Promise<McpTokenResult> {
  const patch: SettingsPatch = { mcp_mode: opts.mode ?? "full" };
  if (opts.port != null) patch.mcp_port = opts.port;
  if (opts.projectId != null) patch.mcp_project_id = opts.projectId;
  if (opts.grantConsent) {
    patch.mcp_consent_version = 1;
    patch.mcp_consented_at = Date.now();
  }
  await settingsApi.set(patch);
  return mcp.enable();
}

export async function disableMcp(): Promise<McpStatus> {
  await settingsApi.set({ mcp_mode: "off" });
  return mcp.disable();
}
