import { useCallback, useEffect, useRef, useState } from "react";

import { useEngineEvent } from "../hooks/useEngineEvent";
import { useI18n, type MessageKey } from "../lib/i18n";
import { publishApi } from "../lib/rpc";
import { rpcErrorMessage } from "../lib/rpcMessage";
import type { PublishApprovalRequest } from "../lib/types";
import { useIllustrationImage } from "./illustration/useIllustrationImage";
import "./PublishApprovalDialog.css";

/** Asks the writer before an agent puts anything on Linetta Works (#238).
 *  The engine holds the agent's tool call until this dialog answers, the
 *  agent gives up, or the request expires, so there is no way to publish
 *  without the writer pressing the button here. Mounted once for the whole
 *  app: an agent may ask while the writer is on any screen. */
export function PublishApprovalDialog() {
  const { t } = useI18n();
  const [pending, setPending] = useState<PublishApprovalRequest[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Set once a notice arrives: the opening fetch may answer after it, with
  // a list older than the one the notice brought.
  const heard = useRef(false);

  useEffect(() => {
    let cancelled = false;
    publishApi
      .approvalsPending()
      .then((res) => {
        if (!cancelled && !heard.current) setPending(res.pending ?? []);
      })
      .catch(() => {
        // An engine that cannot publish has nothing waiting.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEngineEvent<{ pending: PublishApprovalRequest[] | null }>("publish-approvals-changed", (event) => {
    heard.current = true;
    setPending(event.pending ?? []);
  });

  const current = pending[0];

  const answer = useCallback(
    async (approved: boolean) => {
      if (!current || busy) return;
      setBusy(true);
      setError(null);
      try {
        await publishApi.answerApproval(current.id, approved);
        // The engine announces the new queue too; dropping it here keeps an
        // answered request from flashing back if that notice is slow.
        setPending((list) => list.filter((item) => item.id !== current.id));
      } catch (err) {
        setError(rpcErrorMessage(err, t));
        // A request that ended on its own (timed out, withdrawn) is gone
        // from the engine's list; show what is really still waiting.
        publishApi.approvalsPending().then((res) => setPending(res.pending ?? [])).catch(() => {});
      } finally {
        setBusy(false);
      }
    },
    [busy, current, t],
  );

  if (!current) return null;
  return <ApprovalCard request={current} more={pending.length - 1} busy={busy} error={error} onAnswer={answer} />;
}

function titleKey(request: PublishApprovalRequest): MessageKey {
  switch (request.kind) {
    case "register_work":
      return request.republish ? "publishApproval.titleUpdateWork" : "publishApproval.titleRegister";
    case "publish_episode":
      return request.republish ? "publishApproval.titleRepublish" : "publishApproval.titleEpisode";
    case "publish_cover":
      return "publishApproval.titleCover";
  }
}

function useSecondsLeft(expiresAt: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  return Math.max(0, Math.ceil((expiresAt - now) / 1000));
}

interface CardProps {
  request: PublishApprovalRequest;
  more: number;
  busy: boolean;
  error: string | null;
  onAnswer: (approved: boolean) => void;
}

function ApprovalCard({ request, more, busy, error, onAnswer }: Readonly<CardProps>) {
  const { t } = useI18n();
  const seconds = useSecondsLeft(request.expires_at);
  const cover = useIllustrationImage(request.kind === "publish_cover" ? request.illustration_id ?? null : null);
  const declineRef = useRef<HTMLButtonElement>(null);

  // Declining is the safe default: Enter on a dialog that appeared mid-typing
  // must not publish.
  useEffect(() => {
    declineRef.current?.focus();
  }, [request.id]);

  let from = t("publishApproval.fromAgent");
  if (request.source !== "agent") {
    from = request.client
      ? t("publishApproval.fromClient", { client: request.client })
      : t("publishApproval.fromExternal");
  }

  return (
    <div className="backdrop center">
      <div
        className="modal publish-approval"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="publish-approval-title"
        data-testid="publish-approval"
        onKeyDown={(event) => {
          if (event.key === "Escape" && !busy) onAnswer(false);
        }}
      >
        <h2 id="publish-approval-title">{t(titleKey(request))}</h2>
        <p className="modal-sub">{from}</p>
        <p className="modal-sub">{t("publishApproval.work", { title: request.work_title })}</p>
        {request.episode_title && (
          <p className="modal-sub">{t("publishApproval.episode", { title: request.episode_title })}</p>
        )}
        {request.kind === "register_work" && (
          <>
            <p className="modal-sub">
              {t(request.serial_status === "completed" ? "publishApproval.serialCompleted" : "publishApproval.serialOngoing")}
            </p>
            <p className="modal-sub"><strong>{t("publishApproval.blurb")}</strong></p>
            <p className="modal-sub publish-approval-blurb">{request.blurb}</p>
          </>
        )}
        {request.kind === "publish_cover" && cover && (
          <img className="publish-approval-cover" src={cover} alt={t("publishApproval.coverAlt")} />
        )}
        {request.kind === "publish_episode" && request.republish && (
          <p className="modal-sub" role="note">{t("publishApproval.republish")}</p>
        )}
        {request.kind === "publish_cover" && request.republish && (
          <p className="modal-sub" role="note">{t("publishApproval.replaceCover")}</p>
        )}
        <p className="modal-sub">{t("publishApproval.public")}</p>
        <p className="modal-sub">{t("publishApproval.expires", { seconds })}</p>
        {more > 0 && <p className="modal-sub">{t("publishApproval.more", { count: more })}</p>}
        {error && <p className="modal-sub publish-approval-error" role="alert">{error}</p>}
        <div className="modal-actions">
          <button ref={declineRef} type="button" className="btn ghost" disabled={busy} onClick={() => onAnswer(false)}>
            {t("publishApproval.decline")}
          </button>
          <button type="button" className="btn accent" disabled={busy} onClick={() => onAnswer(true)}>
            {t("publishApproval.approve")}
          </button>
        </div>
      </div>
    </div>
  );
}
