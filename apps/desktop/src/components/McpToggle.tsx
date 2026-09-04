import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useEngineEvent } from "../hooks/useEngineEvent";
import { useI18n } from "../lib/i18n";
import { disableMcp, enableMcp } from "../lib/mcpControl";
import { mcp, projects as projectsApi, settings as settingsApi } from "../lib/rpc";
import { rpcErrorMessage } from "../lib/rpcMessage";
import type { McpStatus } from "../lib/types";
import "./McpToggle.css";

/** The agent door, on the work itself.
 *
 *  The writer must never be surprised that something other than them can edit
 *  the work — and must never need the settings pane just to open or close the
 *  door. Off is a quiet outline; on is a filled dot. Clicking opens a popover
 *  that says exactly WHICH work is open (the server may be scoped to another
 *  one), and turning on always goes through the popover so a stray click can
 *  never open the manuscript. Turning off is immediate: closing the door is
 *  the safe direction.
 *
 *  Enabling from here always scopes the server to the current work in full
 *  mode. Wider scopes and read-only live in Settings, where the trade-off can
 *  be explained.
 */

export type McpToggleProps = {
  projectId: string;
  projectTitle: string;
};

export function McpToggle({ projectId, projectTitle }: McpToggleProps) {
  const { t } = useI18n();
  const [status, setStatus] = useState<McpStatus | null>(null);
  // A build without MCP has no such method; staying hidden is correct.
  const [supported, setSupported] = useState(true);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  // Assume consent until settings say otherwise, so the common case (already
  // consented) never flashes the consent sentence.
  const [consented, setConsented] = useState(true);
  const [scopedTitle, setScopedTitle] = useState<string | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);

  const refresh = useCallback(async () => {
    try {
      setStatus(await mcp.status());
    } catch {
      setSupported(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // A change means SOME agent wrote to the manuscript — not necessarily the
  // HTTP host: the built-in panel fires mcp.changed with the external server
  // stopped. So this is a prompt to re-ask, not proof of anything; refresh
  // re-queries mcp.status, which is what actually decides the dot.
  useEngineEvent("mcp-changed", () => {
    void refresh();
  });

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  // The popover names the work the server is actually scoped to; fetch its
  // title only when it differs from the one on screen.
  const scopedId = status?.running ? (status.project_id ?? "") : "";
  useEffect(() => {
    if (!open || !scopedId || scopedId === projectId) {
      setScopedTitle(null);
      return;
    }
    let cancelled = false;
    projectsApi.get(scopedId).then(
      (p) => {
        if (!cancelled) setScopedTitle(p.title);
      },
      () => {
        if (!cancelled) setScopedTitle(null);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [open, scopedId, projectId]);

  const toggleOpen = async () => {
    setError(null);
    const next = !open;
    setOpen(next);
    if (!next) return;
    void refresh();
    try {
      const cfg = await settingsApi.get();
      setConsented((cfg.mcp_consent_version ?? 0) >= 1);
    } catch {
      // Unreadable settings keep the optimistic default; enabling still works
      // because the consent fields ride in the same patch when needed.
    }
  };

  const doEnable = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await enableMcp({ mode: "full", projectId, grantConsent: !consented });
      setStatus(res.status);
      setConsented(true);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const doDisable = async () => {
    setBusy(true);
    setError(null);
    try {
      setStatus(await disableMcp());
      setOpen(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  if (!supported) return null;

  const running = status?.running ?? false;
  const label = !running
    ? t("workspace.mcp.off")
    : status?.mode === "full"
      ? t("workspace.mcp.active.full")
      : t("workspace.mcp.active.readOnly");
  const scopeState = !running ? "off" : scopedId === "" ? "all" : scopedId === projectId ? "this" : "other";

  return (
    <div className="mcp-toggle" ref={rootRef}>
      <button
        type="button"
        className={`ws-tool icon-only${running ? " is-active" : ""}`}
        title={label}
        aria-label={label}
        aria-expanded={open}
        onClick={() => void toggleOpen()}
        data-testid="mcp-indicator"
      >
        <span aria-hidden="true" className={`mcp-dot${running ? " on" : ""}`}>
          {running ? "●" : "○"}
        </span>
      </button>

      {open && (
        <div className="mcp-popover" role="dialog" aria-label={t("settings.mcp.title")} data-testid="mcp-popover">
          {!running ? (
            <>
              <p>{t("workspace.mcp.enable.body", { title: projectTitle })}</p>
              {!consented && (
                <p className="sd" data-testid="mcp-popover-consent">
                  {t("settings.mcp.consent")}
                </p>
              )}
              <div className="mcp-popover-actions">
                <button
                  type="button"
                  className="btn"
                  disabled={busy}
                  onClick={() => void doEnable()}
                  data-testid="mcp-toggle-enable"
                >
                  {consented ? t("workspace.mcp.enable.confirm") : t("workspace.mcp.enable.consentConfirm")}
                </button>
              </div>
            </>
          ) : (
            <>
              <p data-testid="mcp-scope">
                {scopeState === "this" && t("workspace.mcp.running.thisWork")}
                {scopeState === "all" && t("workspace.mcp.running.allWorks")}
                {scopeState === "other" &&
                  t("workspace.mcp.running.otherWork", { title: scopedTitle ?? "…" })}
              </p>
              {status?.mode === "read_only" && <p className="sd">{t("settings.mcp.mode.read_only")}</p>}
              <div className="mcp-popover-actions">
                {scopeState === "other" && (
                  <button
                    type="button"
                    className="btn"
                    disabled={busy}
                    onClick={() => void doEnable()}
                    data-testid="mcp-rescope"
                  >
                    {t("workspace.mcp.rescope")}
                  </button>
                )}
                <button
                  type="button"
                  className="btn"
                  disabled={busy}
                  onClick={() => void doDisable()}
                  data-testid="mcp-toggle-disable"
                >
                  {t("settings.mcp.disable")}
                </button>
              </div>
            </>
          )}
          {error != null && (
            <p className="sd" role="alert" data-testid="mcp-toggle-error">
              {rpcErrorMessage(error, t)}
            </p>
          )}
          <Link to="/settings" className="mcp-popover-link" onClick={() => setOpen(false)}>
            {t("workspace.mcp.openSettings")}
          </Link>
        </div>
      )}
    </div>
  );
}
