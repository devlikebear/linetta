import { useState } from "react";

import { useSyncStatus } from "../../hooks/useSyncStatus";
import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { syncApi } from "../../lib/rpc";
import { reasonMessage, rpcErrorMessage } from "../../lib/rpcMessage";
import { formatVaultBytes } from "../../lib/vault";
import type { SyncPreview, SyncStatus, SyncWork } from "../../lib/types";

const WORK_STATE_KEYS: Record<string, MessageKey> = {
  synced: "settings.sync.work.synced",
  not_uploaded: "settings.sync.work.notUploaded",
  local_changes: "settings.sync.work.localChanges",
  remote_changes: "settings.sync.work.remoteChanges",
  conflict: "settings.sync.work.conflict",
  deleted: "settings.sync.work.deleted",
  removed: "settings.sync.work.removed",
};

function stateLine(status: SyncStatus, t: ReturnType<typeof useI18n>["t"]): string {
  switch (status.state) {
    case "running":
      return t("settings.sync.running");
    case "waiting":
      return t("settings.sync.waiting");
    case "stopped":
      return t("settings.sync.stopped", { reason: reasonMessage(status.reason ?? "", t) });
    default:
      return t("settings.sync.idle");
  }
}

function formatWhen(ms: number, language: string): string {
  return new Date(ms).toLocaleString(language);
}

/** Turning automatic sync on is the moment works start leaving without being
 *  asked one by one, so it says how much first and sends nothing until the
 *  writer agrees. The switch is per device. */
export function SyncAutoPanel({ editable }: { editable: boolean }) {
  const { t, language } = useI18n();
  const [status, setStatus] = useSyncStatus(true);
  const [preview, setPreview] = useState<SyncPreview | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

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

  const askToEnable = () => guard(async () => setPreview(await syncApi.preview()));
  const confirmEnable = () =>
    guard(async () => {
      setStatus(await syncApi.setAuto(true));
      setPreview(null);
    });
  const disable = () => guard(async () => setStatus(await syncApi.setAuto(false)));
  const runNow = () => guard(async () => setStatus(await syncApi.runNow()));

  if (!status) return null;
  const enabled = status.enabled;
  const works = status.works.filter((work) => work.state !== "synced" || work.problem);

  return (
    <div className="sync-auto" data-testid="sync-auto">
      <h4>{t("settings.sync.autoTitle")}</h4>
      <p className="sd">{t("settings.sync.autoNote")}</p>
      {enabled ? (
        <>
          <p
            className={status.state === "stopped" ? "sd vault-warning" : "sd"}
            role={status.state === "stopped" ? "alert" : "status"}
            data-testid="sync-state"
          >
            {stateLine(status, t)}
          </p>
          <p className="sd" data-testid="sync-last">
            {status.last_success_at
              ? t("settings.sync.lastSuccess", { when: formatWhen(status.last_success_at, language) })
              : t("settings.sync.neverSynced")}
          </p>
          {works.length > 0 && (
            <ul className="vault-items" data-testid="sync-works">
              {works.map((work: SyncWork) => (
                <li key={work.project_id || work.item_id}>
                  <span className="vault-item-title">{work.title}</span>
                  <span className="sd" data-testid={`sync-work-${work.project_id}`}>
                    {work.problem ? reasonMessage(work.problem, t) : t(WORK_STATE_KEYS[work.state] ?? "settings.sync.work.synced")}
                  </span>
                </li>
              ))}
            </ul>
          )}
          <div className="field-actions">
            <button type="button" className="btn sm" disabled={busy} onClick={() => void runNow()} data-testid="sync-run-now">
              {t("settings.sync.runNow")}
            </button>
            <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void disable()} data-testid="sync-disable">
              {t("settings.sync.disable")}
            </button>
          </div>
        </>
      ) : preview ? (
        <div className="vault-delete-confirm" data-testid="sync-enable-confirm">
          <p className="sd">
            {t("settings.sync.enableConfirm", { works: preview.works, size: formatVaultBytes(preview.bytes) })}
          </p>
          <div className="field-actions">
            <button type="button" className="btn sm" disabled={busy} onClick={() => void confirmEnable()} data-testid="sync-enable-yes">
              {t("settings.sync.enableYes")}
            </button>
            <button type="button" className="btn ghost sm" disabled={busy} onClick={() => setPreview(null)} data-testid="sync-enable-no">
              {t("common.cancel")}
            </button>
          </div>
        </div>
      ) : (
        <div className="field-actions">
          <button type="button" className="btn sm" disabled={busy || !editable} onClick={() => void askToEnable()} data-testid="sync-enable">
            {t("settings.sync.enable")}
          </button>
        </div>
      )}
      {error ? <p className="sd" role="alert" data-testid="sync-error">{rpcErrorMessage(error, t)}</p> : null}
    </div>
  );
}
