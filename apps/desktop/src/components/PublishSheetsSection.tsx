import { useEffect, useState } from "react";

import { entityKindLabel, useI18n } from "../lib/i18n";
import { illustrations, publishApi } from "../lib/rpc";
import { PUBLISH_STATE_KEYS } from "../lib/publish";
import type { IllustrationMeta, PublishSheetsStatus } from "../lib/types";

/** Sheets and the cover in the publish dialog (Works Phase 8).
 *
 *  The writer turns sheets on in each sheet's panel and picks the cover here,
 *  from the gallery; "upload sheets and cover" then makes the site hold
 *  exactly that. Errors go to the dialog through run, like its other
 *  actions. */
export function PublishSheetsSection({
  projectId,
  sheets,
  busy,
  run,
  onSynced,
}: {
  projectId: string;
  sheets: PublishSheetsStatus;
  busy: boolean;
  /** The dialog's guard: shows busy and errors. */
  run: (fn: () => Promise<void>) => Promise<void>;
  /** Called after the site took the sheets, to refresh the status. */
  onSynced: () => Promise<unknown>;
}) {
  const { language, t } = useI18n();
  const [gallery, setGallery] = useState<IllustrationMeta[]>([]);
  const [coverId, setCoverId] = useState(sheets.cover_id ?? "");
  const [preview, setPreview] = useState<string | null>(null);
  const [synced, setSynced] = useState(false);

  useEffect(() => {
    illustrations
      .list(projectId)
      .then((res) => setGallery(res.items))
      .catch(() => setGallery([]));
  }, [projectId]);

  useEffect(() => setCoverId(sheets.cover_id ?? ""), [sheets.cover_id]);

  useEffect(() => {
    let cancelled = false;
    if (!coverId) {
      setPreview(null);
      return;
    }
    illustrations
      .get(coverId)
      .then((img) => {
        if (!cancelled) setPreview(`data:${img.mime};base64,${img.data_base64}`);
      })
      .catch(() => {
        if (!cancelled) setPreview(null);
      });
    return () => {
      cancelled = true;
    };
  }, [coverId]);

  const chooseCover = (id: string) =>
    run(async () => {
      await publishApi.setCover(projectId, id);
      setCoverId(id);
      setSynced(false);
    });

  const sync = () =>
    run(async () => {
      await publishApi.sheetsSync(projectId);
      await onSynced();
      setSynced(true);
    });

  const label = (meta: IllustrationMeta, index: number) => meta.caption.trim() || t("publish.sheets.picture", { number: String(index + 1) });

  return (
    <div className="publish-sheets" data-testid="publish-sheets">
      <p className="sk">{t("publish.sheets.title")}</p>
      <p className="sd">
        {sheets.chosen > 0 ? t("publish.sheets.notice", { count: sheets.chosen }) : t("publish.sheets.none")}
      </p>

      <div className="modal-field">
        <label htmlFor="publish-cover">{t("publish.sheets.cover")}</label>
        <select
          id="publish-cover"
          value={coverId}
          disabled={busy}
          onChange={(e) => void chooseCover(e.target.value)}
          data-testid="publish-cover"
        >
          <option value="">{t("publish.sheets.coverNone")}</option>
          {gallery.map((meta, index) => (
            <option key={meta.id} value={meta.id}>
              {label(meta, index)}
            </option>
          ))}
        </select>
        {preview && <img className="publish-cover-preview" src={preview} alt="" data-testid="publish-cover-preview" />}
        {sheets.cover && (
          <p className="sd">
            {t("publish.sheets.coverOnSite")}{" "}
            <span className={`publish-badge is-${sheets.cover.state}`}>{t(PUBLISH_STATE_KEYS[sheets.cover.state])}</span>
            {sheets.cover.moderation_reason && ` ${t("publish.review.reason", { reason: sheets.cover.moderation_reason })}`}
          </p>
        )}
      </div>

      {sheets.remote && sheets.sheets.length > 0 && (
        <ul className="publish-sheet-list" data-testid="publish-sheet-list">
          {sheets.sheets.map((sheet) => (
            <li key={sheet.entity_id}>
              <span>
                {sheet.name} <span className="sd">{entityKindLabel(language, sheet.kind)}</span>
              </span>
              <span className={`publish-badge is-${sheet.state}`}>{t(PUBLISH_STATE_KEYS[sheet.state])}</span>
              {sheet.moderation_reason && (
                <span className="sd publish-sheet-reason">{t("publish.review.reason", { reason: sheet.moderation_reason })}</span>
              )}
            </li>
          ))}
        </ul>
      )}

      <div className="field-actions">
        <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void sync()} data-testid="publish-sheets-sync">
          {synced ? t("publish.sheets.synced") : t("publish.sheets.sync")}
        </button>
      </div>
    </div>
  );
}
