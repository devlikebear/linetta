import { useCallback, useEffect, useState } from "react";
import { Copy, ExternalLink } from "lucide-react";

import { useI18n } from "../lib/i18n";
import { openExternalUrl, publishApi } from "../lib/rpc";
import { rpcErrorMessage } from "../lib/rpcMessage";
import { PUBLISH_STATE_KEYS, findPublishEpisode } from "../lib/publish";
import { PublishSheetsSection } from "./PublishSheetsSection";
import type {
  PublishConnectStatus,
  PublishEpisodeView,
  PublishSerialStatus,
  PublishWorkStatus,
} from "../lib/types";
import "./PublishBadge.css";
import "./PublishDialog.css";

/** Publish one episode to Linetta Works.
 *
 *  The dialog never decides what an episode is: it asks the engine for the
 *  work's status and finds the episode holding the chosen node there. A work
 *  published for the first time needs a blurb before anything goes up;
 *  after that the work's details fold away under "edit work details".
 *
 *  Nothing the dialog does touches the manuscript. A failed publish leaves
 *  the text exactly as it was, says why, and keeps the button to try again.
 */

export interface PublishDialogProps {
  open: boolean;
  projectId: string;
  /** The episode, or any scene inside it. */
  nodeId: string;
  onClose: () => void;
  /** Takes the writer to Settings → Publishing. */
  onOpenSettings: () => void;
  /** Called with the fresh status after anything changed on the site. */
  onChanged?: (status: PublishWorkStatus) => void;
}

function reasonOf(error: unknown): string | null {
  const data = (error as { data?: { reason?: unknown } } | null)?.data;
  return typeof data?.reason === "string" ? data.reason : null;
}

export function PublishDialog({ open, projectId, nodeId, onClose, onOpenSettings, onChanged }: PublishDialogProps) {
  const { language, t } = useI18n();
  const [connection, setConnection] = useState<PublishConnectStatus | null>(null);
  const [status, setStatus] = useState<PublishWorkStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [published, setPublished] = useState<PublishEpisodeView | null>(null);
  const [copied, setCopied] = useState(false);
  const [blurb, setBlurb] = useState("");
  const [serial, setSerial] = useState<PublishSerialStatus>("ongoing");
  const [slug, setSlug] = useState("");
  const [workSaved, setWorkSaved] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const conn = await publishApi.connectStatus();
      setConnection(conn);
      if (conn.state !== "connected") return;
      const ws = await publishApi.workStatus(projectId);
      setStatus(ws);
      setBlurb(ws.work?.blurb ?? "");
      setSerial(ws.work?.serial_status ?? "ongoing");
      // A revoked token turns the connection off underneath us.
      if (!ws.connected) setConnection(await publishApi.connectStatus());
    } catch (e) {
      setError(e);
      if (reasonOf(e) === "publish_unauthorized") {
        setConnection(await publishApi.connectStatus().catch(() => null));
      }
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    if (!open) return;
    setPublished(null);
    setCopied(false);
    setSlug("");
    setWorkSaved(false);
    void load();
  }, [open, load]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  const episode = findPublishEpisode(status, nodeId);
  const registered = Boolean(status?.registered);
  const shown = published ?? episode;
  const connected = connection?.state === "connected";

  const refresh = async () => {
    const ws = await publishApi.workStatus(projectId);
    setStatus(ws);
    onChanged?.(ws);
    return ws;
  };

  const guard = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
      if (reasonOf(e) === "publish_unauthorized") {
        setConnection(await publishApi.connectStatus().catch(() => null));
      }
    } finally {
      setBusy(false);
    }
  };

  const saveWork = async () => {
    await publishApi.upsertWork({
      project_id: projectId,
      blurb: blurb.trim(),
      serial_status: serial,
      ...(registered || !slug.trim() ? {} : { slug: slug.trim() }),
    });
  };

  const publish = () =>
    guard(async () => {
      if (!registered) await saveWork();
      const res = await publishApi.publishEpisode(nodeId);
      setPublished(res.episode);
      await refresh().catch(() => {});
    });

  const updateWork = () =>
    guard(async () => {
      await saveWork();
      await refresh();
      setWorkSaved(true);
    });

  const setVisibility = (target: PublishEpisodeView, visibility: "public" | "private") =>
    guard(async () => {
      const view = await publishApi.setEpisodeVisibility(projectId, target.node_id, visibility);
      if (published && published.node_id === view.node_id) setPublished({ ...published, ...view });
      await refresh();
    });

  const copyUrl = (url: string) =>
    guard(async () => {
      if (!navigator.clipboard?.writeText) throw new Error("clipboard unavailable");
      await navigator.clipboard.writeText(url);
      setCopied(true);
    });

  const blurbMissing = !blurb.trim();
  const canPublish = Boolean(episode) && !episode?.empty && (registered || !blurbMissing);
  const onSite = Boolean(shown?.url);
  const errorReason = reasonOf(error);

  const workFields = (
    <>
      <div className="modal-field">
        <label htmlFor="publish-blurb">{t("publish.blurb")}</label>
        <textarea
          id="publish-blurb"
          value={blurb}
          rows={4}
          maxLength={2000}
          disabled={busy}
          placeholder={t("publish.blurbPlaceholder")}
          onChange={(e) => {
            setBlurb(e.target.value);
            setWorkSaved(false);
          }}
          data-testid="publish-blurb"
        />
      </div>
      <div className="modal-field">
        <label htmlFor="publish-serial">{t("publish.serialStatus")}</label>
        <select
          id="publish-serial"
          value={serial}
          disabled={busy}
          onChange={(e) => {
            setSerial(e.target.value as PublishSerialStatus);
            setWorkSaved(false);
          }}
          data-testid="publish-serial"
        >
          <option value="ongoing">{t("publish.serial.ongoing")}</option>
          <option value="completed">{t("publish.serial.completed")}</option>
        </select>
      </div>
    </>
  );

  return (
    <div className="backdrop center" onClick={onClose}>
      <div
        className="modal publish-modal"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={t("publish.title")}
        data-testid="publish-dialog"
      >
        <h2>{t("publish.title")}</h2>

        {loading ? (
          <p className="modal-sub">{t("common.loading")}</p>
        ) : !connected ? (
          <div data-testid="publish-not-connected">
            <p className="modal-sub">
              {connection?.failure_reason === "revoked" || errorReason === "publish_unauthorized"
                ? t("publish.revoked")
                : t("publish.notConnected")}
            </p>
            <div className="modal-actions">
              <button type="button" className="btn ghost" onClick={onClose}>{t("common.close")}</button>
              <button type="button" className="btn accent" onClick={onOpenSettings} data-testid="publish-open-settings">
                {t("publish.openSettings")}
              </button>
            </div>
          </div>
        ) : (
          <>
            {status?.offline && (
              <p className="sd publish-offline" role="status" data-testid="publish-offline">
                {t("publish.offline")}
              </p>
            )}

            {!episode ? (
              <p className="modal-sub" data-testid="publish-no-episode">{t("errors.publishNotEpisode")}</p>
            ) : (
              <div className="publish-episode" data-testid="publish-episode">
                <div className="publish-episode-head">
                  <span className="sk">
                    {[episode.label, episode.title].filter(Boolean).join(" · ") || t("publish.untitled")}
                  </span>
                  <span className={`publish-badge is-${shown?.state ?? episode.state}`} data-testid="publish-state">
                    {t(PUBLISH_STATE_KEYS[shown?.state ?? episode.state])}
                  </span>
                </div>
                <p className="sd">
                  {t("publish.charCount", { count: episode.char_count.toLocaleString(language) })}
                  {shown?.number ? ` · ${t("publish.number", { number: String(shown.number) })}` : ""}
                </p>
                {episode.empty && <p className="sd" role="alert">{t("errors.publishEmptyEpisode")}</p>}
                {episode.illustrations ? (
                  <p className="sd" data-testid="publish-illustrations">
                    {t("publish.illustrations", { count: episode.illustrations })}
                  </p>
                ) : null}
                {episode.modified && episode.state !== "modified" && onSite && (
                  <p className="sd">{t("publish.differsFromSite")}</p>
                )}
                {(shown?.state === "rejected" || shown?.state === "needs_review" || shown?.state === "pending") && (
                  <div className="publish-review" data-testid="publish-review">
                    <p className="sd">{t(`publish.review.${shown.state === "needs_review" ? "needsReview" : shown.state}`)}</p>
                    {shown.moderation_reason && (
                      <p className="sd" data-testid="publish-review-reason">
                        {t("publish.review.reason", { reason: shown.moderation_reason })}
                      </p>
                    )}
                    {shown.state !== "pending" && <p className="sd">{t("publish.review.appeal")}</p>}
                  </div>
                )}
                {onSite && shown?.url && (
                  <div className="publish-url" data-testid="publish-url">
                    <code>{shown.url}</code>
                    <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void copyUrl(shown.url ?? "")} data-testid="publish-copy">
                      <Copy size={13} /> {copied ? t("publish.copied") : t("publish.copy")}
                    </button>
                    <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void guard(() => openExternalUrl(shown.url ?? ""))} data-testid="publish-open">
                      <ExternalLink size={13} /> {t("publish.open")}
                    </button>
                    {shown.state === "private" ? (
                      <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void setVisibility(shown, "public")} data-testid="publish-make-public">
                        {t("publish.makePublic")}
                      </button>
                    ) : shown.state !== "rejected" ? (
                      <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void setVisibility(shown, "private")} data-testid="publish-make-private">
                        {t("publish.makePrivate")}
                      </button>
                    ) : null}
                  </div>
                )}
              </div>
            )}

            {!registered ? (
              <div className="publish-work" data-testid="publish-work-new">
                <p className="sd">{t("publish.firstTime")}</p>
                {workFields}
                <div className="modal-field">
                  <label htmlFor="publish-slug">{t("publish.slug")}</label>
                  <input
                    id="publish-slug"
                    value={slug}
                    disabled={busy}
                    placeholder={t("publish.slugPlaceholder")}
                    onChange={(e) => setSlug(e.target.value.toLowerCase())}
                    data-testid="publish-slug"
                  />
                </div>
              </div>
            ) : (
              <details className="publish-work" data-testid="publish-work-edit">
                <summary>{t("publish.editWork")}</summary>
                {status?.work?.url && <p className="sd"><code>{status.work.url}</code></p>}
                {workFields}
                <div className="field-actions">
                  <button type="button" className="btn ghost sm" disabled={busy || blurbMissing} onClick={() => void updateWork()} data-testid="publish-save-work">
                    {workSaved ? t("publish.workSaved") : t("publish.saveWork")}
                  </button>
                </div>
              </details>
            )}

            {registered && status?.sheets && (
              <PublishSheetsSection
                projectId={projectId}
                sheets={status.sheets}
                busy={busy}
                run={guard}
                onSynced={refresh}
              />
            )}

            {status && status.site_only.length > 0 && (
              <div className="publish-site-only" data-testid="publish-site-only">
                <p className="sk">{t("publish.siteOnly.title")}</p>
                <p className="sd">{t("publish.siteOnly.description")}</p>
                <ul>
                  {status.site_only.map((ep) => (
                    <li key={ep.node_id}>
                      <span>
                        {t("publish.number", { number: String(ep.number ?? "") })} {ep.title}
                      </span>
                      <span className={`publish-badge is-${ep.remote_status ?? "public"}`}>
                        {ep.remote_status === "private" ? t("publish.state.private") : t("publish.state.public")}
                      </span>
                      {ep.remote_status !== "private" && (
                        <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void setVisibility(ep, "private")} data-testid={`publish-site-only-private-${ep.node_id}`}>
                          {t("publish.makePrivate")}
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {error ? (
              <p className="sd" role="alert" data-testid="publish-error">
                {rpcErrorMessage(error, t)}
              </p>
            ) : null}

            <div className="modal-actions">
              <button type="button" className="btn ghost" onClick={onClose}>{t("common.close")}</button>
              {errorReason === "publish_unauthorized" ? (
                <button type="button" className="btn accent" onClick={onOpenSettings} data-testid="publish-open-settings">
                  {t("publish.openSettings")}
                </button>
              ) : (
                <button
                  type="button"
                  className="btn accent"
                  disabled={busy || !canPublish}
                  onClick={() => void publish()}
                  data-testid="publish-submit"
                >
                  {error
                    ? t("publish.retry")
                    : busy
                      ? t("publish.publishing")
                      : onSite
                        ? t("publish.republish")
                        : t("publish.publish")}
                </button>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
