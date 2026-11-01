import { Link } from "react-router-dom";

import { useSyncStatus } from "../hooks/useSyncStatus";
import { useI18n } from "../lib/i18n";
import { reasonMessage } from "../lib/rpcMessage";

/** The library's line for automatic sync: only what the writer has to know. It
 *  says nothing while things are in step or merely offline. */
export function SyncStatusNotice({ available }: { available: boolean }) {
  const { t } = useI18n();
  const [status] = useSyncStatus(available);
  if (!status?.enabled) return null;

  const conflicts = status.works.filter((work) => work.state === "conflict");
  const deleted = status.works.filter((work) => status.archived.includes(work.project_id));
  return (
    <>
      {status.state === "stopped" && (
        <p className="lib-vault-notice" role="alert" data-testid="sync-stopped-notice">
          {t("sync.stopped", { reason: reasonMessage(status.reason ?? "", t) })}
        </p>
      )}
      {conflicts.length > 0 && (
        <p className="lib-vault-notice" data-testid="sync-conflict-notice">
          {t("sync.conflicts")}{" "}
          {conflicts.map((work, i) => (
            <span key={work.project_id}>
              {i > 0 && ", "}
              <Link to={`/workspace/${work.project_id}`}>{work.title}</Link>
            </span>
          ))}
        </p>
      )}
      {deleted.length > 0 && (
        <p className="lib-vault-notice" data-testid="sync-archived-notice">
          {t("sync.archived", { titles: deleted.map((work) => work.title).join(", ") })}
        </p>
      )}
    </>
  );
}
