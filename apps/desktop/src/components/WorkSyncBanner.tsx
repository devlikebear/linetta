import { useState } from "react";

import { useI18n } from "../lib/i18n";
import { vaultApi } from "../lib/rpc";
import { rpcErrorMessage } from "../lib/rpcMessage";
import type { SyncWork } from "../lib/types";

export type WorkSyncBannerProps = {
  projectId: string;
  /** What automatic sync last found about this work, if it is out of step. */
  work: SyncWork | undefined;
  /** Saves the buffer: a pull must not race a save of the old text. */
  flush: () => Promise<void>;
  onReopen: () => void;
  onOpenDialog: () => void;
};

/** The open work's line when automatic sync cannot act on it. A newer version
 *  waits because it would replace the text on screen, so taking it is one press
 *  that the writer makes; a conflict needs their choice in the dialog. */
export function WorkSyncBanner({ projectId, work, flush, onReopen, onOpenDialog }: WorkSyncBannerProps) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  if (work?.state !== "remote_changes" && work?.state !== "conflict") return null;

  const take = async () => {
    setBusy(true);
    setError(null);
    try {
      await flush();
      await vaultApi.pull({ project_id: projectId });
      onReopen();
    } catch (e) {
      setError(e);
      setBusy(false);
    }
  };

  return (
    <div className="ws-sync-banner" role="status" data-testid="work-sync-banner">
      {work.state === "conflict" ? (
        <>
          <span>{t("sync.workConflict")}</span>
          <button type="button" className="btn ghost sm" onClick={onOpenDialog} data-testid="work-sync-choose">
            {t("sync.choose")}
          </button>
        </>
      ) : (
        <>
          <span>{t("sync.workNewVersion")}</span>
          <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void take()} data-testid="work-sync-take">
            {t("sync.take")}
          </button>
        </>
      )}
      {error ? <span role="alert" data-testid="work-sync-error">{rpcErrorMessage(error, t)}</span> : null}
    </div>
  );
}
