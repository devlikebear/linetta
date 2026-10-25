import { useEffect, useState } from "react";

import { useI18n } from "../lib/i18n";
import { publishApi } from "../lib/rpc";
import { rpcErrorMessage } from "../lib/rpcMessage";

/** "Show on Linetta Works" for one sheet (Works Phase 8).
 *
 *  Shown only while this device is connected to the site; a build without
 *  publishing (mobile) answers the status call with an error and the toggle
 *  stays hidden. The choice is saved at once and goes to the site the next
 *  time the writer uploads sheets from the publish dialog. */
export function SheetPublishToggle({ entityId }: { entityId: string }) {
  const { t } = useI18n();
  const [available, setAvailable] = useState(false);
  const [published, setPublished] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let cancelled = false;
    publishApi
      .connectStatus()
      .then(async (conn) => {
        if (conn.state !== "connected") return;
        const { published } = await publishApi.sheetGet(entityId);
        if (cancelled) return;
        setPublished(published);
        setAvailable(true);
      })
      .catch(() => {
        // Not available in this build, or not reachable: show nothing.
      });
    return () => {
      cancelled = true;
    };
  }, [entityId]);

  if (!available) return null;

  const toggle = async (next: boolean) => {
    setBusy(true);
    setError(null);
    try {
      const res = await publishApi.sheetSet(entityId, next);
      setPublished(res.published);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="sec es-publish" data-testid="sheet-publish">
      <label className="es-publish-row">
        <input
          type="checkbox"
          checked={published}
          disabled={busy}
          onChange={(e) => void toggle(e.target.checked)}
          data-testid="sheet-publish-toggle"
        />
        <span>{t("entity.publish.label")}</span>
      </label>
      <p className="sd">{t(published ? "entity.publish.on" : "entity.publish.hint")}</p>
      {error ? <p className="sd es-error" role="alert">{rpcErrorMessage(error, t)}</p> : null}
    </div>
  );
}
