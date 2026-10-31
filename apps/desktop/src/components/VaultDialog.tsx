import { useCallback, useEffect, useRef, useState } from "react";

import { useEngineEvent } from "../hooks/useEngineEvent";
import { useI18n } from "../lib/i18n";
import type { MessageKey } from "../lib/i18n";
import { vaultApi } from "../lib/rpc";
import { rpcErrorMessage } from "../lib/rpcMessage";
import { formatVaultBytes, formatVaultDate, vaultReady } from "../lib/vault";
import type {
  VaultAccountMode,
  VaultProgress,
  VaultPullResult,
  VaultPushResult,
  VaultWorkState,
  VaultWorkStatus,
} from "../lib/types";
import "./VaultDialog.css";

/** Upload a work to the private vault, take a newer version, settle a conflict
 *  or go back to a restore point.
 *
 *  The caller saves the buffer before mounting this, and the dialog takes focus
 *  so nothing more is typed into the editor behind it. Nothing here runs by
 *  itself: every change to the manuscript is a button the writer pressed.
 *
 *  Once a pull lands, the editor behind the dialog is showing text that no
 *  longer exists, so the result screen has one way out and it reopens the work.
 */

const STATE_KEYS: Record<VaultWorkState, MessageKey> = {
  not_uploaded: "vault.state.notUploaded",
  synced: "vault.state.synced",
  local_changes: "vault.state.localChanges",
  remote_changes: "vault.state.remoteChanges",
  conflict: "vault.state.conflict",
};

const PROGRESS_KEYS: Record<VaultProgress["phase"], MessageKey> = {
  upload: "vault.progress.upload",
  download: "vault.progress.download",
  apply: "vault.progress.apply",
};

export interface VaultDialogProps {
  projectId: string;
  onClose: () => void;
  /** Takes the writer to Settings → Private Vault. */
  onOpenSettings: () => void;
  /** Called with the fresh state whenever it is known. */
  onStatus?: (state: VaultWorkState) => void;
  /** The manuscript was replaced: throw the open editor away and load it again. */
  onReopen: () => void;
}

export function VaultDialog({ projectId, onClose, onOpenSettings, onStatus, onReopen }: VaultDialogProps) {
  const { language, t } = useI18n();
  const [loading, setLoading] = useState(true);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [mode, setMode] = useState<VaultAccountMode>("full");
  const [retrieveUntil, setRetrieveUntil] = useState("");
  const [status, setStatus] = useState<VaultWorkStatus | null>(null);
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState<VaultProgress | null>(null);
  const [pushed, setPushed] = useState<VaultPushResult | null>(null);
  const [pulled, setPulled] = useState<VaultPullResult | null>(null);
  const [confirmRevision, setConfirmRevision] = useState<number | null>(null);
  const [error, setError] = useState<unknown>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  const refresh = useCallback(async () => {
    const next = await vaultApi.workStatus(projectId);
    setStatus(next);
    onStatus?.(next.state);
  }, [projectId, onStatus]);

  useEffect(() => {
    dialogRef.current?.focus();
    let cancelled = false;
    (async () => {
      const overview = await vaultApi.status();
      if (cancelled) return;
      if (!vaultReady(overview)) {
        setNeedsSetup(true);
        return;
      }
      setMode(overview.account?.mode ?? "full");
      setRetrieveUntil(overview.account?.retrieve_until ?? "");
      await refresh();
    })()
      .catch((e: unknown) => {
        if (!cancelled) setError(e);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [refresh]);

  useEngineEvent<VaultProgress>("vault-progress", (next) => {
    if (next.project_id === projectId) setProgress(next);
  });

  const leave = pulled ? onReopen : onClose;
  const closable = !running;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      if (closable) leave();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [closable, leave]);

  const run = async (fn: () => Promise<void>) => {
    setRunning(true);
    setProgress(null);
    setError(null);
    setPushed(null);
    setConfirmRevision(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
      // A conflict or a local change found on the way says the screen is out of
      // date; the reason stays up beside the choices that now apply.
      await refresh().catch(() => {});
    } finally {
      setRunning(false);
    }
  };

  const push = () =>
    run(async () => {
      setPushed(await vaultApi.push(projectId));
      await refresh();
    });

  const pull = (revision?: number) =>
    run(async () => {
      setPulled(await vaultApi.pull({ project_id: projectId, revision }));
    });

  const keepLocal = () =>
    run(async () => {
      const result = await vaultApi.resolveConflict({ project_id: projectId, choice: "keep_local" });
      setPushed(result.push ?? null);
      await refresh();
    });

  const takeRemote = () =>
    run(async () => {
      const result = await vaultApi.resolveConflict({
        project_id: projectId,
        choice: "take_remote",
        title_suffix: t("vault.copySuffix"),
      });
      setPulled(result.pull ?? null);
    });

  const canUpload = mode === "full";
  const state = status?.state;
  // A restore over unsent work would lose it, and the engine refuses.
  const canRestore = state === "synced" || state === "remote_changes";
  // Taking the head again is how files a pull could not write get put back.
  const canPullAgain = state === "synced";

  const renderResult = (result: VaultPullResult) => (
    <div data-testid="vault-pulled">
      <p className="modal-sub">{t("vault.pulled")}</p>
      {result.copy_project_id && <p className="modal-sub" data-testid="vault-copy-kept">{t("vault.copyKept")}</p>}
      {!result.files_restored && (
        <p className="modal-sub" role="alert" data-testid="vault-files-missing">{t("vault.filesNotRestored")}</p>
      )}
      {result.snapshot_path && (
        <p className="modal-sub" data-testid="vault-snapshot">
          {t("vault.snapshot")} <code>{result.snapshot_path}</code>
        </p>
      )}
      <div className="modal-actions">
        <button type="button" className="btn sm" onClick={onReopen} data-testid="vault-reopen">
          {t("vault.reopen")}
        </button>
      </div>
    </div>
  );

  const renderActions = () => {
    if (state === "not_uploaded" || state === "local_changes") {
      return (
        <div className="modal-actions">
          <button
            type="button"
            className="btn sm"
            disabled={running || !canUpload}
            onClick={() => void push()}
            data-testid="vault-push"
          >
            {t("vault.push")}
          </button>
        </div>
      );
    }
    if (state === "remote_changes") {
      return (
        <>
          <p className="modal-sub">{t("vault.pullNote")}</p>
          <div className="modal-actions">
            <button type="button" className="btn sm" disabled={running} onClick={() => void pull()} data-testid="vault-pull">
              {t("vault.pull")}
            </button>
          </div>
        </>
      );
    }
    if (state === "conflict") {
      return (
        <div className="vault-choices" data-testid="vault-conflict">
          <p className="modal-sub">{t("vault.conflictNote")}</p>
          <button
            type="button"
            className="btn ghost sm"
            disabled={running || !canUpload}
            onClick={() => void keepLocal()}
            data-testid="vault-keep-local"
          >
            {t("vault.keepLocal")}
          </button>
          <p className="modal-sub">{t("vault.keepLocalNote")}</p>
          <button
            type="button"
            className="btn ghost sm"
            disabled={running}
            onClick={() => void takeRemote()}
            data-testid="vault-take-remote"
          >
            {t("vault.takeRemote")}
          </button>
          <p className="modal-sub">{t("vault.takeRemoteNote")}</p>
        </div>
      );
    }
    return null;
  };

  const renderRestorePoints = () => {
    if (!status || status.restore_points.length === 0) return null;
    return (
      <div className="vault-points" data-testid="vault-restore-points">
        <h3>{t("vault.restorePoints")}</h3>
        {!canRestore && <p className="modal-sub">{t("vault.restoreBlocked")}</p>}
        <ul>
          {status.restore_points.map((point) => {
            const head = point.revision === status.head_revision;
            const confirming = confirmRevision === point.revision;
            const label = head ? "vault.pullAgain" : "vault.restore";
            return (
              <li key={point.revision}>
                <span>
                  {formatVaultDate(point.created, language)} · {formatVaultBytes(point.bytes)}
                  {head && ` · ${t("vault.latest")}`}
                </span>
                {(head ? canPullAgain : canRestore) && (
                  <button
                    type="button"
                    className="btn ghost sm"
                    disabled={running}
                    onClick={() => (confirming ? void pull(point.revision) : setConfirmRevision(point.revision))}
                    data-testid={`vault-restore-${point.revision}`}
                  >
                    {t(confirming ? "vault.restoreConfirm" : label)}
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      </div>
    );
  };

  const renderBody = () => {
    if (pulled) return renderResult(pulled);
    if (loading) return <p className="modal-sub">{t("common.loading")}</p>;
    if (needsSetup) {
      return (
        <>
          <p className="modal-sub" data-testid="vault-needs-setup">{t("vault.needsSetup")}</p>
          <div className="modal-actions">
            <button type="button" className="btn sm" onClick={onOpenSettings} data-testid="vault-open-settings">
              {t("vault.openSettings")}
            </button>
          </div>
        </>
      );
    }
    if (!status) return null;
    return (
      <>
        <p className="vault-state" data-testid="vault-state">{t(STATE_KEYS[status.state])}</p>
        {mode === "terms_outdated" && <p className="modal-sub" role="alert">{t("settings.vault.termsOutdated")}</p>}
        {mode === "read_only" && (
          <p className="modal-sub" role="alert">
            {t("settings.vault.readOnly", { date: formatVaultDate(retrieveUntil, language) })}
          </p>
        )}
        {pushed && (
          <p className="modal-sub" role="status" data-testid="vault-pushed">
            {t(pushed.unchanged ? "vault.unchanged" : "vault.pushed")}
          </p>
        )}
        {renderActions()}
        {renderRestorePoints()}
      </>
    );
  };

  const percent = progress && progress.total > 0 ? Math.round((progress.done / progress.total) * 100) : 0;

  return (
    <div className="backdrop center" onClick={closable ? leave : undefined}>
      <div
        ref={dialogRef}
        tabIndex={-1}
        className="modal vault-modal"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={t("vault.title")}
        data-testid="vault-dialog"
      >
        <h2>{t("vault.title")}</h2>
        {renderBody()}
        {running && (
          <div className="vault-progress" data-testid="vault-progress">
            <p className="modal-sub" role="status">
              {t(progress ? PROGRESS_KEYS[progress.phase] : "vault.progress.start", {
                done: progress?.done ?? 0,
                total: progress?.total ?? 0,
              })}
            </p>
            <div role="progressbar" aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100}>
              <div style={{ width: `${percent}%` }} />
            </div>
          </div>
        )}
        {error ? (
          <p className="modal-sub" role="alert" data-testid="vault-dialog-error">
            {rpcErrorMessage(error, t)}
          </p>
        ) : null}
        {!pulled && (
          <div className="modal-actions">
            <button type="button" className="btn ghost sm" disabled={!closable} onClick={onClose} data-testid="vault-close">
              {t("common.close")}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
