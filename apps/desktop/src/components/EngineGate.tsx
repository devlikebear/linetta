import { useEffect, useState, type ReactNode } from "react";
import { engineStatus, openRecoveryFolder, restoreLatestBackup } from "../lib/rpc";
import type { EngineStatus } from "../lib/types";
import { useI18n } from "../lib/i18n";

interface Props {
  children: ReactNode;
}

export function EngineGate({ children }: Props) {
  const { t } = useI18n();
  const [status, setStatus] = useState<EngineStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [recoveryMessage, setRecoveryMessage] = useState<string | null>(null);
  const [recovering, setRecovering] = useState(false);

  const refresh = () => {
    setLoading(true);
    engineStatus()
      .then(setStatus)
      .catch((e) => setStatus({
        ok: false,
        error: String(e),
        version: null,
        home: null,
        db_path: null,
        migration_version: null,
        migration_count: null,
      }))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    refresh();
  }, []);

  if (loading) {
    return (
      <main className="shell engine-gate">
        <h1>Linetta</h1>
        <p className="hint">{t("engine.checking")}</p>
      </main>
    );
  }

  if (!status?.ok) {
    const diagnostics = buildDiagnostics(status);
    return (
      <main className="shell engine-gate">
        <h1>{t("engine.failedTitle")}</h1>
        <p className="hint">
          {t("engine.failedDescription")}
        </p>
        {status?.error && <p className="error">{status.error}</p>}
        {status?.home && <p className="hint">{t("engine.recoveryDescription")}</p>}
        {recoveryMessage && <p className="hint">{recoveryMessage}</p>}
        <pre className="engine-diagnostics">{diagnostics}</pre>
        <div className="engine-gate-actions">
          <button type="button" onClick={refresh}>{t("engine.retry")}</button>
          <button
            type="button"
            onClick={() => navigator.clipboard?.writeText(diagnostics)}
          >
            {t("engine.copyDiagnostics")}
          </button>
          {status?.home && (
            <>
              <button
                type="button"
                onClick={() => {
                  openRecoveryFolder().catch((e) => {
                    setRecoveryMessage(t("engine.recoveryFailed", { error: String(e) }));
                  });
                }}
              >
                {t("engine.openBackupFolder")}
              </button>
              <button
                type="button"
                disabled={recovering}
                onClick={async () => {
                  if (!window.confirm(t("engine.restoreConfirm"))) return;
                  setRecovering(true);
                  try {
                    const restored = await restoreLatestBackup();
                    setRecoveryMessage(t("engine.restoreComplete", { path: restored.backup_path }));
                  } catch (e) {
                    setRecoveryMessage(t("engine.recoveryFailed", { error: String(e) }));
                  } finally {
                    setRecovering(false);
                  }
                }}
              >
                {t("engine.restoreLatest")}
              </button>
            </>
          )}
        </div>
      </main>
    );
  }

  return <>{children}</>;
}

function buildDiagnostics(status: EngineStatus | null): string {
  return JSON.stringify({
    ok: status?.ok ?? false,
    error: status?.error ?? null,
    version: status?.version ?? null,
    home: status?.home ?? null,
    db_path: status?.db_path ?? null,
    migration_version: status?.migration_version ?? null,
    migration_count: status?.migration_count ?? null,
  }, null, 2);
}
