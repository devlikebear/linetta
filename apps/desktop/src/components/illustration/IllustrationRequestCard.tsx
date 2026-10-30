import { useCallback, useEffect, useState } from "react";

import { useEngineEvent } from "../../hooks/useEngineEvent";
import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import {
  illustrationRequests as requestsApi,
  imageProviders as imageProvidersApi,
  nodes as nodesApi,
} from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type {
  IllustrationAspect,
  IllustrationRequest,
  IllustrationsChangedPayload,
  ImageProviderStatus,
} from "../../lib/types";
import { useIllustrationImage } from "./useIllustrationImage";
import "./Illustration.css";

interface Props {
  requestId: string;
  /** A row the caller already has (the gallery's list), to skip one fetch. */
  initial?: IllustrationRequest;
  onOpenSettings?: () => void;
}

const ASPECTS: IllustrationAspect[] = ["portrait", "square", "landscape"];

/* A picture an agent asked for, waiting for the writer (illustration Phase 4).
 * Nothing reaches an image provider until "Generate": the writer reads the
 * prompt that will be sent, may change it, the aspect and the caption, and
 * draws it — or rejects it. The same card shows how it ended. */
export function IllustrationRequestCard({ requestId, initial, onOpenSettings }: Props) {
  const { t } = useI18n();
  const [request, setRequest] = useState<IllustrationRequest | null>(initial ?? null);
  const [target, setTarget] = useState("");
  const [provider, setProvider] = useState<ImageProviderStatus | null | undefined>(undefined);
  const [prompt, setPrompt] = useState(initial?.prompt ?? "");
  const [aspect, setAspect] = useState<IllustrationAspect>(initial?.aspect ?? "portrait");
  const [caption, setCaption] = useState(initial?.caption ?? "");
  const [includeReferences, setIncludeReferences] = useState(initial?.include_references ?? false);
  const [busy, setBusy] = useState<"approve" | "reject" | null>(null);
  const [error, setError] = useState<unknown>(null);
  const pictureUrl = useIllustrationImage(request?.status === "done" ? request.illustration_id ?? null : null);

  const adopt = useCallback((r: IllustrationRequest) => {
    setRequest(r);
    setPrompt(r.prompt);
    setAspect(r.aspect);
    setCaption(r.caption);
    setIncludeReferences(r.include_references);
  }, []);

  const reload = useCallback(async () => {
    try {
      adopt(await requestsApi.get(requestId));
    } catch (e) {
      setError(e);
    }
  }, [adopt, requestId]);

  useEffect(() => {
    if (!initial) void reload();
  }, [initial, reload]);

  useEffect(() => {
    imageProvidersApi
      .list()
      .then((rows) => setProvider(rows.find((r) => r.active && r.configured && r.consented) ?? null))
      .catch(() => setProvider(null));
  }, []);

  const targetId = request?.target_node_id;
  useEffect(() => {
    if (!targetId) {
      setTarget("");
      return;
    }
    let cancelled = false;
    nodesApi
      .get(targetId)
      .then((n) => {
        if (!cancelled) setTarget([n.label, n.title].filter(Boolean).join(" · "));
      })
      .catch(() => {
        if (!cancelled) setTarget("");
      });
    return () => {
      cancelled = true;
    };
  }, [targetId]);

  useEngineEvent<IllustrationsChangedPayload>("illustrations-changed", (payload) => {
    if (payload.request_id === requestId && busy === null) void reload();
  });

  if (!request) {
    return error ? (
      <div className="illustration-request" data-testid="illustration-request">
        <p className="sd" role="alert">{rpcErrorMessage(error, t)}</p>
      </div>
    ) : null;
  }

  const waiting = request.status === "pending" || request.status === "failed";
  const providerName = provider ? t(`settings.providers.name.${provider.id}` as MessageKey) : "";

  const approve = async () => {
    setBusy("approve");
    setError(null);
    setRequest({ ...request, status: "generating" });
    try {
      adopt(
        await requestsApi.approve(request.id, {
          prompt: prompt.trim(),
          aspect,
          caption: caption.trim(),
          include_references: includeReferences,
        }),
      );
    } catch (e) {
      setError(e);
      await reload();
    } finally {
      setBusy(null);
    }
  };

  const reject = async () => {
    setBusy("reject");
    setError(null);
    try {
      adopt(await requestsApi.reject(request.id));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className={`illustration-request is-${request.status}`} data-testid="illustration-request">
      <div className="illustration-request-head">
        <strong>{t(`illustrationRequest.purpose.${request.purpose}` as MessageKey)}</strong>
        {target ? <span className="sd"> · {target}</span> : null}
        <span className={`illustration-request-status is-${request.status}`} data-testid="illustration-request-status">
          {t(`illustrationRequest.status.${request.status}` as MessageKey)}
        </span>
      </div>
      {request.note ? <p className="illustration-request-note">{request.note}</p> : null}
      {request.source === "external" ? <p className="sd">{t("illustrationRequest.fromExternal")}</p> : null}

      {waiting ? (
        <>
          <div className="modal-field illustration-prompt">
            <label htmlFor={`request-prompt-${request.id}`}>{t("illustration.prompt")}</label>
            <textarea
              id={`request-prompt-${request.id}`}
              rows={5}
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              disabled={busy !== null}
              data-testid="illustration-request-prompt"
            />
            {provider ? (
              <p className="sd">{t("illustration.prompt.hint", { provider: providerName })}</p>
            ) : null}
          </div>
          <div className="illustration-options">
            <div className="illustration-seg" role="group" aria-label={t("illustration.aspect")}>
              {ASPECTS.map((a) => (
                <button
                  key={a}
                  type="button"
                  className={aspect === a ? "is-selected" : ""}
                  onClick={() => setAspect(a)}
                  disabled={busy !== null}
                >
                  {t(`illustration.aspect.${a}` as MessageKey)}
                </button>
              ))}
            </div>
            {request.character_ids.length > 0 ? (
              <label className="sd">
                <input
                  type="checkbox"
                  checked={includeReferences}
                  onChange={(e) => setIncludeReferences(e.target.checked)}
                  disabled={busy !== null}
                />{" "}
                {t("illustration.includeReferences")}
              </label>
            ) : null}
          </div>
          <div className="modal-field">
            <label htmlFor={`request-caption-${request.id}`}>{t("illustration.caption")}</label>
            <input
              id={`request-caption-${request.id}`}
              value={caption}
              onChange={(e) => setCaption(e.target.value)}
              disabled={busy !== null}
            />
          </div>
          {request.status === "failed" && request.error ? (
            <p className="sd" role="alert">{t("illustrationRequest.failed", { reason: request.error })}</p>
          ) : null}
          {provider === null ? <p className="sd">{t("illustration.off")}</p> : null}
          <div className="illustration-request-actions">
            {provider === null ? (
              <button type="button" className="btn accent sm" onClick={onOpenSettings} disabled={!onOpenSettings}>
                {t("illustration.openSettings")}
              </button>
            ) : (
              <button
                type="button"
                className="btn accent sm"
                onClick={() => void approve()}
                disabled={!provider || prompt.trim() === "" || busy !== null}
                data-testid="illustration-request-approve"
              >
                {request.status === "failed" ? t("illustration.again") : t("illustration.generate")}
              </button>
            )}
            <button
              type="button"
              className="btn ghost sm"
              onClick={() => void reject()}
              disabled={busy !== null}
              data-testid="illustration-request-reject"
            >
              {t("illustrationRequest.reject")}
            </button>
          </div>
        </>
      ) : null}

      {request.status === "generating" ? <p className="sd">{t("illustration.generating")}</p> : null}

      {request.status === "done" ? (
        <div className="illustration-request-done" data-testid="illustration-request-done">
          {pictureUrl ? <img src={pictureUrl} alt={request.caption} /> : null}
          <p className="sd">
            {request.placed_at
              ? t(`illustrationRequest.placed.${request.purpose}` as MessageKey)
              : t("illustrationRequest.notPlaced", { reason: request.error ?? "" })}
          </p>
        </div>
      ) : null}

      {request.status === "cancelled" ? <p className="sd">{t("illustrationRequest.cancelled")}</p> : null}
      {error ? <p className="sd" role="alert">{rpcErrorMessage(error, t)}</p> : null}
    </div>
  );
}
