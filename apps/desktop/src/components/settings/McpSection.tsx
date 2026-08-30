import { useCallback, useEffect, useState } from "react";

import { useI18n } from "../../lib/i18n";
import { disableMcp, enableMcp } from "../../lib/mcpControl";
import { mcp, mcpBridgePath, projects as projectsApi, settings as settingsApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { McpActivityEntry, McpStatus, Project } from "../../lib/types";

/** Connect an external agent (MCP).
 *
 *  The pane's job is to make one decision legible: an agent outside Linetta is
 *  about to be able to read — and, in full mode, rewrite — the manuscript. So
 *  consent is an explicit checkbox rather than something a toggle implies, and
 *  the token only ever reaches the copyable command, never the .mcp.json
 *  snippet, because that file is one people commit.
 */

export type McpSectionProps = {
  /** Absolute path to the bundled bridge. Resolved from the shell when not
   *  supplied; tests pass it directly. */
  bridgePath?: string;
};

type Saved = {
  mode: string;
  port: number;
  projectId: string;
  consented: boolean;
};

export function McpSection({ bridgePath }: McpSectionProps) {
  const { t } = useI18n();
  // undefined until the shell answers, then a path or null. The difference
  // matters: null means this build ships no bridge (Mac App Store), which the
  // writer has to be told, and "not asked yet" must not look like that.
  const [resolvedBridge, setResolvedBridge] = useState<string | null | undefined>(bridgePath);
  const [status, setStatus] = useState<McpStatus | null>(null);
  const [saved, setSaved] = useState<Saved>({ mode: "full", port: 7391, projectId: "", consented: false });
  const [works, setWorks] = useState<Project[]>([]);
  const [token, setToken] = useState<string | null>(null);
  const [activity, setActivity] = useState<McpActivityEntry[]>([]);
  // The raw failure is kept and translated at render time: a reason code has
  // no language of its own, and switching language should redraw the message
  // rather than leave a stale sentence on screen.
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    const [st, cfg] = await Promise.all([mcp.status(), settingsApi.get()]);
    setStatus(st);
    setSaved({
      // Off carries no mode preference, so the pane defaults to full: writing
      // together is what connecting an agent is for. Read-only survives a
      // round trip only while the writer keeps it chosen under Advanced.
      mode: cfg.mcp_mode === "read_only" ? "read_only" : "full",
      port: cfg.mcp_port || 7391,
      projectId: cfg.mcp_project_id ?? "",
      consented: (cfg.mcp_consent_version ?? 0) >= 1,
    });
    if (st.running) {
      setActivity(await mcp.activity(20));
    }
  }, []);

  // The scope picker names works instead of asking for a UUID. A load failure
  // leaves the list empty; the current scope still renders as a raw option
  // below so an unknown id is never silently dropped.
  useEffect(() => {
    projectsApi.list({}).then(setWorks, () => setWorks([]));
  }, []);

  useEffect(() => {
    void refresh().catch(setError);
  }, [refresh]);

  useEffect(() => {
    if (bridgePath) return;
    void mcpBridgePath()
      .then((path) => setResolvedBridge(path ?? null))
      .catch(() => setResolvedBridge(null));
  }, [bridgePath]);

  const guard = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const saveConsent = (consented: boolean) =>
    guard(async () => {
      await settingsApi.set({
        mcp_consent_version: consented ? 1 : 0,
        mcp_consented_at: consented ? Date.now() : 0,
      });
      setSaved((s) => ({ ...s, consented }));
    });

  const enable = () =>
    guard(async () => {
      const res = await enableMcp({
        mode: saved.mode,
        port: saved.port,
        projectId: saved.projectId,
      });
      setToken(res.token);
      setStatus(res.status);
      setActivity(await mcp.activity(20));
    });

  const disable = () =>
    guard(async () => {
      // disableMcp persists "off" before stopping, so the kill switch
      // survives an app restart (#74).
      setStatus(await disableMcp());
      setToken(null);
    });

  const regenerate = () =>
    guard(async () => {
      const res = await mcp.regenerateToken();
      setToken(res.token);
      setStatus(res.status);
    });

  const running = status?.running ?? false;
  const port = status?.port || saved.port;
  const endpoint = `http://127.0.0.1:${port}/mcp`;

  // The literal token goes only in the command the writer runs on their own
  // machine. .mcp.json gets headersHelper instead — it is a file people commit.
  const addCommand = token
    ? `claude mcp add --transport http linetta ${endpoint} --header "Authorization: Bearer ${token}"`
    : null;
  const projectConfig = JSON.stringify(
    {
      mcpServers: {
        linetta: {
          type: "http",
          url: endpoint,
          headersHelper: `${resolvedBridge ?? "linetta-mcp"} --print-headers`,
        },
      },
    },
    null,
    2,
  );
  const desktopConfig = JSON.stringify(
    { mcpServers: { linetta: { command: resolvedBridge ?? "linetta-mcp", args: [] } } },
    null,
    2,
  );

  return (
    <section className="settings-section" id="mcp-settings" data-testid="mcp-section">
      <h3>{t("settings.mcp.title")}</h3>
      <p className="sd">{t("settings.mcp.description")}</p>

      <label className="modal-field">
        <input
          type="checkbox"
          checked={saved.consented}
          disabled={busy || running}
          onChange={(e) => void saveConsent(e.target.checked)}
          data-testid="mcp-consent"
        />
        {t("settings.mcp.consent")}
      </label>

      <div className="modal-field">
        <label htmlFor="mcp-project">{t("settings.mcp.projectLimit")}</label>
        <select
          id="mcp-project"
          value={saved.projectId}
          disabled={busy || running}
          onChange={(e) => setSaved((s) => ({ ...s, projectId: e.target.value }))}
        >
          <option value="">{t("settings.mcp.projectLimit.all")}</option>
          {works.map((p) => (
            <option key={p.id} value={p.id}>
              {p.title}
            </option>
          ))}
          {saved.projectId !== "" && !works.some((p) => p.id === saved.projectId) && (
            <option value={saved.projectId}>{saved.projectId}</option>
          )}
        </select>
      </div>

      {/* One decision turns MCP on; the trade-offs live behind Advanced.
          Turning on uses full access — the collaboration this exists for —
          unless read-only is deliberately chosen here. */}
      <details className="modal-field" data-testid="mcp-advanced">
        <summary>{t("settings.mcp.advanced")}</summary>
        <label className="modal-field">
          <input
            type="checkbox"
            checked={saved.mode === "read_only"}
            disabled={busy || running}
            onChange={(e) => setSaved((s) => ({ ...s, mode: e.target.checked ? "read_only" : "full" }))}
            data-testid="mcp-readonly"
          />
          {t("settings.mcp.mode.read_only")}
        </label>
        <div className="modal-field">
          <label htmlFor="mcp-port">{t("settings.mcp.port")}</label>
          <input
            id="mcp-port"
            type="number"
            value={saved.port}
            disabled={busy || running}
            onChange={(e) => setSaved((s) => ({ ...s, port: Number(e.target.value) }))}
          />
        </div>
      </details>

      {!running && (
        <button
          type="button"
          disabled={busy || !saved.consented}
          onClick={() => void enable()}
          data-testid="mcp-enable"
        >
          {t("settings.mcp.enable")}
        </button>
      )}
      {running && (
        <>
          <p data-testid="mcp-running">{t("settings.mcp.running", { port: String(port) })}</p>
          <button type="button" disabled={busy} onClick={() => void disable()} data-testid="mcp-disable">
            {t("settings.mcp.disable")}
          </button>
          <button type="button" disabled={busy} onClick={() => void regenerate()} data-testid="mcp-regenerate">
            {t("settings.mcp.regenerate")}
          </button>
        </>
      )}

      {error != null && (
        <p className="sd" role="alert" data-testid="mcp-error">
          {/* A taken port is the one failure a writer can fix from this pane,
              so the engine's reason code becomes a sentence naming the knob. */}
          {rpcErrorMessage(error, t)}
        </p>
      )}

      {running && (
        <div data-testid="mcp-snippets">
          <h4>{t("settings.mcp.snippets.title")}</h4>
          {addCommand ? (
            <>
              <p className="sd">{t("settings.mcp.snippets.claudeCode")}</p>
              <pre data-testid="mcp-snippet-command">{addCommand}</pre>
            </>
          ) : (
            <p className="sd" data-testid="mcp-token-hidden">
              {t("settings.mcp.snippets.tokenHidden")}
            </p>
          )}
          {resolvedBridge === null && (
            <p className="sd" data-testid="mcp-bridge-missing">
              {t("settings.mcp.snippets.bridgeMissing")}
            </p>
          )}
          <p className="sd">{t("settings.mcp.snippets.projectConfig")}</p>
          <pre data-testid="mcp-snippet-project">{projectConfig}</pre>
          <p className="sd">{t("settings.mcp.snippets.desktop")}</p>
          <pre data-testid="mcp-snippet-desktop">{desktopConfig}</pre>
        </div>
      )}

      {running && activity.length > 0 && (
        <div data-testid="mcp-activity">
          <h4>{t("settings.mcp.activity.title")}</h4>
          <ul>
            {activity.map((entry) => (
              <li key={entry.id}>
                {entry.ok ? "✓" : "✕"} {entry.tool}
                {entry.detail ? ` — ${entry.detail}` : ""}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
