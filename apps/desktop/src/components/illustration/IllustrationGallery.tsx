import { useCallback, useEffect, useState } from "react";

import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { illustrations as illustrationsApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { IllustrationMeta } from "../../lib/types";
import { useIllustrationImage } from "./useIllustrationImage";
import "./Illustration.css";

interface Props {
  projectId: string;
  /** Bumped by the caller after a drawing is saved, to reload the list. */
  refreshKey?: number;
  onClose: () => void;
  onNew: () => void;
  /** Puts a picture into the open scene. Absent when no scene is open. */
  onInsert?: (meta: IllustrationMeta) => void;
}

function formatBytes(n: number): string {
  if (n === 0) return "0 KB";
  if (n < 1024 * 1024) return `${Math.max(1, Math.round(n / 1024))} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

function Thumb({ meta, onOpen }: { meta: IllustrationMeta; onOpen: () => void }) {
  const url = useIllustrationImage(meta.id);
  return (
    <button type="button" className="illustration-thumb" onClick={onOpen} data-testid={`illustration-thumb-${meta.id}`}>
      {url ? <img src={url} alt={meta.caption} /> : <span className="illustration-thumb-empty" />}
      {meta.caption ? <span className="illustration-thumb-caption">{meta.caption}</span> : null}
      {meta.used_in?.length ? <ThumbBadge /> : null}
    </button>
  );
}

function ThumbBadge() {
  const { t } = useI18n();
  return <span className="illustration-thumb-badge">{t("illustration.inUse")}</span>;
}

/* A work's illustrations: everything drawn for it, newest first. Each one
 * keeps the prompt that was sent, so a picture can be understood (and drawn
 * again) later. The footer says what they cost in library.db, because every
 * daily backup carries them. */
export function IllustrationGallery({ projectId, refreshKey, onClose, onNew, onInsert }: Props) {
  const { t, language } = useI18n();
  const [items, setItems] = useState<IllustrationMeta[] | null>(null);
  const [usage, setUsage] = useState({ count: 0, bytes: 0 });
  const [openId, setOpenId] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async () => {
    try {
      const out = await illustrationsApi.list(projectId);
      setItems(out.items);
      setUsage({ count: out.count, bytes: out.bytes });
    } catch (e) {
      setError(e);
    }
  }, [projectId]);

  useEffect(() => {
    void load();
  }, [load, refreshKey]);

  const open = items?.find((m) => m.id === openId) ?? null;

  return (
    <div className="backdrop center" onMouseDown={onClose}>
      <div
        className="modal illustration-gallery"
        role="dialog"
        aria-label={t("illustration.gallery")}
        onMouseDown={(e) => e.stopPropagation()}
        data-testid="illustration-gallery"
      >
        <div className="illustration-gallery-head">
          <h2>{t("illustration.gallery")}</h2>
          <button type="button" className="btn accent sm" onClick={onNew} data-testid="illustration-new">
            {t("illustration.new")}
          </button>
        </div>

        {error ? (
          <p className="sd" role="alert">
            {rpcErrorMessage(error, t)}
          </p>
        ) : null}

        {open ? (
          <Detail
            meta={open}
            locale={language}
            onInsert={onInsert}
            onBack={() => setOpenId(null)}
            onChanged={(next) => setItems((cur) => cur?.map((m) => (m.id === next.id ? next : m)) ?? cur)}
            onDeleted={() => {
              setOpenId(null);
              void load();
            }}
          />
        ) : items && items.length === 0 ? (
          <p className="sd" data-testid="illustration-empty">
            {t("illustration.empty")}
          </p>
        ) : (
          <div className="illustration-grid">
            {(items ?? []).map((m) => (
              <Thumb key={m.id} meta={m} onOpen={() => setOpenId(m.id)} />
            ))}
          </div>
        )}

        <div className="modal-actions illustration-gallery-foot">
          <span className="sd" data-testid="illustration-usage">
            {t("illustration.usage", { count: usage.count, size: formatBytes(usage.bytes) })}
          </span>
          <button type="button" className="btn ghost" onClick={onClose}>
            {t("common.close")}
          </button>
        </div>
      </div>
    </div>
  );
}

function Detail({
  meta,
  locale,
  onInsert,
  onBack,
  onChanged,
  onDeleted,
}: {
  meta: IllustrationMeta;
  locale?: string;
  onInsert?: (meta: IllustrationMeta) => void;
  onBack: () => void;
  onChanged: (meta: IllustrationMeta) => void;
  onDeleted: () => void;
}) {
  const { t } = useI18n();
  const url = useIllustrationImage(meta.id);
  const [caption, setCaption] = useState(meta.caption);
  const [armed, setArmed] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const saveCaption = async () => {
    if (caption.trim() === meta.caption) return;
    try {
      onChanged(await illustrationsApi.update(meta.id, caption));
    } catch (e) {
      setError(e);
    }
  };

  const remove = async () => {
    if (!armed) {
      setArmed(true);
      return;
    }
    try {
      await illustrationsApi.delete(meta.id);
      onDeleted();
    } catch (e) {
      setArmed(false);
      setError(e);
    }
  };

  const providerName = t(`settings.providers.name.${meta.provider}` as MessageKey);

  return (
    <div className="illustration-detail" data-testid="illustration-detail">
      <button type="button" className="btn ghost sm" onClick={onBack}>
        {t("illustration.back")}
      </button>
      <div className="illustration-detail-body">
        {url ? <img src={url} alt={meta.caption} /> : <span className="illustration-thumb-empty" />}
        <div className="illustration-detail-side">
          <div className="modal-field">
            <label htmlFor="illustration-caption">{t("illustration.caption")}</label>
            <input
              id="illustration-caption"
              value={caption}
              onChange={(e) => setCaption(e.target.value)}
              onBlur={() => void saveCaption()}
              data-testid="illustration-caption"
            />
          </div>
          <p className="sd">
            {t("illustration.detail.provider", { provider: providerName, model: meta.model })}
            {" · "}
            {new Date(meta.created_at).toLocaleString(locale)}
            {meta.width && meta.height ? ` · ${meta.width}×${meta.height}` : ""}
          </p>
          <div className="modal-field">
            <label>{t("illustration.detail.prompt")}</label>
            <pre className="illustration-detail-prompt">{meta.prompt}</pre>
          </div>
          {meta.used_in?.length ? (
            <div className="modal-field" data-testid="illustration-used-in">
              <label>{t("illustration.usedIn")}</label>
              <p className="sd">{meta.used_in.map((s) => [s.label, s.title].filter(Boolean).join(" · ")).join(", ")}</p>
            </div>
          ) : null}
          {error ? (
            <p className="sd" role="alert">
              {rpcErrorMessage(error, t)}
            </p>
          ) : null}
          <div className="illustration-detail-actions">
            <button
              type="button"
              className="btn accent sm"
              disabled={!onInsert}
              title={onInsert ? undefined : t("illustration.noScene")}
              onClick={() => onInsert?.({ ...meta, caption: caption.trim() })}
              data-testid="illustration-insert"
            >
              {t("illustration.insert")}
            </button>
            <button type="button" className="btn ghost sm" onClick={() => void remove()} data-testid="illustration-delete">
              {armed ? t("illustration.deleteConfirm") : t("illustration.delete")}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
