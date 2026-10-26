import { useCallback, useEffect, useState } from "react";

import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { useEngineEvent } from "../../hooks/useEngineEvent";
import { illustrationRequests as requestsApi, illustrations as illustrationsApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { IllustrationMeta, IllustrationRequest, IllustrationsChangedPayload } from "../../lib/types";
import { IllustrationRequestCard } from "./IllustrationRequestCard";
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
  /** The nodes a picture can be made to stand for from here: the open
   *  scene and the episode (or chapter) it belongs to. */
  targets?: RepresentativeTarget[];
  /** A node's label, for "represents …". */
  nodeLabel?: (nodeId: string) => string;
  /** Called after the writer changes which picture stands for a node. */
  onRepresentativesChanged?: () => void;
  onOpenSettings?: () => void;
}

export interface RepresentativeTarget {
  id: string;
  label: string;
}

/** Requests still waiting for the writer, or drawing, or failed and retryable. */
const OPEN_REQUESTS = ["pending", "generating", "failed"] as const;

function formatBytes(n: number): string {
  if (n === 0) return "0 KB";
  if (n < 1024 * 1024) return `${Math.max(1, Math.round(n / 1024))} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

function Thumb({ meta, representative, onOpen }: { meta: IllustrationMeta; representative: boolean; onOpen: () => void }) {
  const { t } = useI18n();
  const url = useIllustrationImage(meta.id);
  return (
    <button type="button" className="illustration-thumb" onClick={onOpen} data-testid={`illustration-thumb-${meta.id}`}>
      {url ? <img src={url} alt={meta.caption} /> : <span className="illustration-thumb-empty" />}
      {meta.caption ? <span className="illustration-thumb-caption">{meta.caption}</span> : null}
      {meta.used_in?.length ? <ThumbBadge /> : null}
      {representative ? (
        <span className="illustration-thumb-badge is-representative">{t("illustration.representative")}</span>
      ) : null}
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
export function IllustrationGallery({
  projectId,
  refreshKey,
  onClose,
  onNew,
  onInsert,
  targets = [],
  nodeLabel,
  onRepresentativesChanged,
  onOpenSettings,
}: Props) {
  const { t, language } = useI18n();
  const [items, setItems] = useState<IllustrationMeta[] | null>(null);
  const [requests, setRequests] = useState<IllustrationRequest[]>([]);
  const [representatives, setRepresentatives] = useState<Record<string, string>>({});
  const [usage, setUsage] = useState({ count: 0, bytes: 0 });
  const [openId, setOpenId] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async () => {
    try {
      const [out, open, reps] = await Promise.all([
        illustrationsApi.list(projectId),
        requestsApi.list(projectId, [...OPEN_REQUESTS]),
        illustrationsApi.representatives(projectId),
      ]);
      setItems(out.items);
      setUsage({ count: out.count, bytes: out.bytes });
      // Oldest first: the order the agent asked in.
      setRequests([...open].reverse());
      setRepresentatives(reps);
    } catch (e) {
      setError(e);
    }
  }, [projectId]);

  useEffect(() => {
    void load();
  }, [load, refreshKey]);

  // A request drawn, rejected or left (by the panel's card, an external
  // client, or this list) changes the list and maybe the gallery. A card
  // that finished stays visible until the next reload so its result shows.
  useEngineEvent<IllustrationsChangedPayload>("illustrations-changed", (payload) => {
    if (payload.project_id !== projectId) return;
    if (payload.status === "pending") {
      void load();
      return;
    }
    if (payload.status === "done") {
      void illustrationsApi
        .list(projectId)
        .then((out) => {
          setItems(out.items);
          setUsage({ count: out.count, bytes: out.bytes });
        })
        .catch(setError);
      void illustrationsApi.representatives(projectId).then(setRepresentatives).catch(setError);
    }
  });

  const representing = (id: string) => Object.keys(representatives).filter((nodeId) => representatives[nodeId] === id);

  const setRepresentative = async (nodeId: string, illustrationId: string) => {
    try {
      await illustrationsApi.setRepresentative(nodeId, illustrationId);
      setRepresentatives(await illustrationsApi.representatives(projectId));
      onRepresentativesChanged?.();
    } catch (e) {
      setError(e);
    }
  };

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

        {!open && requests.length > 0 ? (
          <section className="illustration-requests" data-testid="illustration-requests">
            <h3>{t("illustrationRequest.section")}</h3>
            {requests.map((r) => (
              <IllustrationRequestCard key={r.id} requestId={r.id} initial={r} onOpenSettings={onOpenSettings} />
            ))}
          </section>
        ) : null}

        {open ? (
          <Detail
            meta={open}
            locale={language}
            onInsert={onInsert}
            targets={targets}
            representing={representing(open.id)}
            representatives={representatives}
            nodeLabel={nodeLabel}
            onSetRepresentative={(nodeId, id) => void setRepresentative(nodeId, id)}
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
              <Thumb key={m.id} meta={m} representative={representing(m.id).length > 0} onOpen={() => setOpenId(m.id)} />
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
  targets,
  representing,
  representatives,
  nodeLabel,
  onSetRepresentative,
  onBack,
  onChanged,
  onDeleted,
}: {
  meta: IllustrationMeta;
  locale?: string;
  onInsert?: (meta: IllustrationMeta) => void;
  targets: RepresentativeTarget[];
  /** The nodes this picture stands for. */
  representing: string[];
  representatives: Record<string, string>;
  nodeLabel?: (nodeId: string) => string;
  onSetRepresentative: (nodeId: string, illustrationId: string) => void;
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
          {representing.length ? (
            <div className="modal-field" data-testid="illustration-representative-of">
              <label>{t("illustration.representativeOf")}</label>
              <p className="sd">{representing.map((id) => nodeLabel?.(id) || id).join(", ")}</p>
            </div>
          ) : null}
          {error ? (
            <p className="sd" role="alert">
              {rpcErrorMessage(error, t)}
            </p>
          ) : null}
          {targets.length ? (
            <div className="illustration-detail-actions" data-testid="illustration-representative-actions">
              {targets.map((target) =>
                representatives[target.id] === meta.id ? (
                  <button
                    key={target.id}
                    type="button"
                    className="btn ghost sm"
                    onClick={() => onSetRepresentative(target.id, "")}
                    data-testid={`illustration-clear-representative-${target.id}`}
                  >
                    {t("illustration.clearRepresentative", { target: target.label })}
                  </button>
                ) : (
                  <button
                    key={target.id}
                    type="button"
                    className="btn ghost sm"
                    onClick={() => onSetRepresentative(target.id, meta.id)}
                    data-testid={`illustration-set-representative-${target.id}`}
                  >
                    {t("illustration.setRepresentative", { target: target.label })}
                  </button>
                ),
              )}
            </div>
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
