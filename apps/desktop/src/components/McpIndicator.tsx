import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { useEngineEvent } from "../hooks/useEngineEvent";
import { useI18n } from "../lib/i18n";
import { mcp } from "../lib/rpc";

/** A quiet marker that an external agent can reach this manuscript.
 *
 *  The writer must never be surprised that something other than them can edit
 *  the work. It shows only while the server is actually listening, and clicking
 *  it goes to Settings, where the activity log and the kill switch are.
 */
export function McpIndicator() {
  const { t } = useI18n();
  const [running, setRunning] = useState(false);
  const [mode, setMode] = useState("");

  const refresh = useCallback(async () => {
    try {
      const st = await mcp.status();
      setRunning(st.running);
      setMode(st.mode);
    } catch {
      // A build without MCP has no such method; staying hidden is correct.
      setRunning(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // An agent change is proof the server is live, so the dot appears without
  // waiting for a poll.
  useEngineEvent("mcp-changed", () => {
    setRunning(true);
  });

  if (!running) return null;

  const label = mode === "full" ? t("workspace.mcp.active.full") : t("workspace.mcp.active.readOnly");
  return (
    <Link
      to="/settings"
      className="ws-tool icon-only"
      title={label}
      aria-label={label}
      data-testid="mcp-indicator"
    >
      <span aria-hidden="true">●</span>
    </Link>
  );
}
