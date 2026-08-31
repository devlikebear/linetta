import { useCallback, useState } from "react";

import { useI18n, localeForLanguage } from "../../lib/i18n";
import { backupApi, type BackupEntry, type BackupProject } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";

/** Restore from a backup — the merge half of #83.
 *
 *  The startup recovery screen replaces the whole database and only exists
 *  when the engine cannot boot. This pane covers the everyday case: the writer
 *  picks a backup, picks a work inside it, and gets that work back as a NEW
 *  project. The live library is never overwritten (the engine also snapshots
 *  it right before the merge), so this flow cannot damage what they are
 *  writing now — which is why no scary confirmation dialog is needed.
 */
export function RestoreSection() {
  const { language, t } = useI18n();
  const [backups, setBackups] = useState<BackupEntry[] | null>(null);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [works, setWorks] = useState<BackupProject[] | null>(null);
  const [busyWork, setBusyWork] = useState<string | null>(null);
  const [restoredTitle, setRestoredTitle] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const loadBackups = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await backupApi.list();
      setBackups(res.backups);
      setOpen(true);
    } catch (err) {
      setError(rpcErrorMessage(err, t));
    } finally {
      setLoading(false);
    }
  }, [t]);

  const peek = useCallback(async (path: string) => {
    setSelected(path);
    setWorks(null);
    setError(null);
    try {
      const res = await backupApi.peek(path);
      setWorks(res.projects);
    } catch (err) {
      setError(rpcErrorMessage(err, t));
    }
  }, [t]);

  const restore = useCallback(
    async (path: string, work: BackupProject) => {
      setBusyWork(work.id);
      setRestoredTitle(null);
      setError(null);
      try {
        const res = await backupApi.restoreProject(path, work.id, t("settings.restore.suffix"));
        setRestoredTitle(res.title);
      } catch (err) {
        setError(rpcErrorMessage(err, t));
      } finally {
        setBusyWork(null);
      }
    },
    [t],
  );

  const kindLabel = (kind: BackupEntry["kind"]) =>
    kind === "daily"
      ? t("settings.restore.kind.daily")
      : kind === "pre_migration"
        ? t("settings.restore.kind.preMigration")
        : t("settings.restore.kind.recovery");

  const locale = localeForLanguage(language);
  const formatWhen = (millis: number) =>
    new Date(millis).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" });

  return (
    <div className="restore-block" data-testid="restore-section">
      <p className="sd">{t("settings.restore.description")}</p>
      {!open ? (
        <button
          type="button"
          className="btn ghost sm"
          onClick={() => void loadBackups()}
          disabled={loading}
          data-testid="restore-show-backups"
        >
          {loading ? t("settings.restore.loading") : t("settings.restore.showBackups")}
        </button>
      ) : (
        <>
          {backups && backups.length === 0 && <p className="sd">{t("settings.restore.empty")}</p>}
          {backups && backups.length > 0 && (
            <ul className="restore-backups" data-testid="restore-backup-list">
              {backups.map((b) => (
                <li key={b.path}>
                  <button
                    type="button"
                    className={"btn ghost sm" + (selected === b.path ? " active" : "")}
                    onClick={() => void peek(b.path)}
                  >
                    {formatWhen(b.created_at)} · {kindLabel(b.kind)} · {formatSize(b.size_bytes)}
                  </button>
                  {selected === b.path && (
                    <div className="restore-works">
                      {works === null && <p className="sd">{t("settings.restore.loading")}</p>}
                      {works && works.length === 0 && (
                        <p className="sd">{t("settings.restore.noWorks")}</p>
                      )}
                      {works && works.length > 0 && (
                        <ul>
                          {works.map((w) => (
                            <li key={w.id} className="restore-work-row">
                              <span className="restore-work-title">{w.title}</span>
                              <button
                                type="button"
                                className="btn ghost sm"
                                onClick={() => void restore(b.path, w)}
                                disabled={busyWork !== null}
                                data-testid={`restore-work-${w.id}`}
                              >
                                {busyWork === w.id
                                  ? t("settings.restore.restoring")
                                  : t("settings.restore.restore")}
                              </button>
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
          <button
            type="button"
            className="btn ghost sm"
            onClick={() => {
              setOpen(false);
              setSelected(null);
              setWorks(null);
            }}
          >
            {t("settings.restore.hideBackups")}
          </button>
        </>
      )}
      {restoredTitle && (
        <p className="sd" data-testid="restore-done">
          {t("settings.restore.done", { value: restoredTitle })}
        </p>
      )}
      {error && <p className="settings-error">{error}</p>}
    </div>
  );
}

function formatSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${bytes} B`;
}
