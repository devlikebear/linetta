import { useCallback, useEffect, useState } from "react";

import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { imageProviders as imageProvidersApi, settings as settingsApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { ImageProviderID, ImageProviderStatus, SettingsPatch } from "../../lib/types";

const IMAGE_PROVIDERS: ImageProviderID[] = ["openai-codex", "gemini-native"];

/* Settings → Image generation. Which provider draws illustrations, with what
 * model, and the writer's consent to send it the prompt and reference images.
 *
 * The credential is the agent provider's, so this pane never asks for a key:
 * a provider that is not connected yet sends the writer to that pane. The
 * consent is not the agent's: the agent's sentence promises what its tools
 * read, and drawing sends something else. */
export function ImageGenSection({ onOpenProviders }: { onOpenProviders: () => void }) {
  const { t } = useI18n();
  const [list, setList] = useState<ImageProviderStatus[] | null>(null);
  const [models, setModels] = useState<string[]>([]);
  const [modelsError, setModelsError] = useState<unknown>(null);
  const [modelDraft, setModelDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const refresh = useCallback(async () => {
    const next = await imageProvidersApi.list();
    setList(next);
    return next;
  }, []);

  useEffect(() => {
    refresh().catch(setError);
  }, [refresh]);

  const active = list?.find((s) => s.active) ?? null;
  const activeId = active?.id ?? null;
  const activeConfigured = Boolean(active?.configured);

  useEffect(() => {
    setModelDraft(active?.model ?? "");
  }, [activeId, active?.model]);

  const loadModels = useCallback(async () => {
    if (!activeId || !activeConfigured) return;
    setModelsError(null);
    try {
      const out = await imageProvidersApi.listModels(activeId);
      setModels(out.models);
    } catch (e) {
      setModels([]);
      setModelsError(e);
    }
  }, [activeId, activeConfigured]);

  useEffect(() => {
    setModels([]);
    void loadModels();
  }, [loadModels]);

  const apply = async (patch: SettingsPatch) => {
    setBusy(true);
    setError(null);
    try {
      await settingsApi.set(patch);
      await refresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const saveModel = () => {
    if (!active) return;
    const model = modelDraft.trim();
    if (model === (active.model ?? "")) return;
    void apply({ image_providers: { [active.id]: { model } } });
  };

  const saveConsent = (yes: boolean) => {
    if (!active) return;
    void apply({ image_providers: { [active.id]: { consented_at: yes ? Date.now() : 0 } } });
  };

  const providerName = (id: ImageProviderID) => t(`settings.providers.name.${id}` as MessageKey);

  return (
    <section className="settings-section" id="image-settings" data-testid="image-section">
      <h3>{t("settings.images.title")}</h3>
      <p className="sd">{t("settings.images.description")}</p>

      <div className="modal-field settings-segmented" data-testid="image-choices">
        <button
          type="button"
          className={!active ? "is-selected" : undefined}
          aria-pressed={!active}
          disabled={busy || !list}
          onClick={() => void apply({ image_provider: "" })}
          data-testid="image-choice-off"
        >
          {t("settings.images.off")}
        </button>
        {IMAGE_PROVIDERS.map((id) => (
          <button
            key={id}
            type="button"
            className={id === activeId ? "is-selected" : undefined}
            aria-pressed={id === activeId}
            disabled={busy || !list}
            onClick={() => void apply({ image_provider: id })}
            data-testid={`image-choice-${id}`}
          >
            {providerName(id)}
          </button>
        ))}
      </div>

      {active && !active.configured ? (
        <div className="modal-field" data-testid="image-not-configured">
          <p className="sd">{t("settings.images.notConfigured")}</p>
          <button type="button" className="btn ghost sm" onClick={onOpenProviders} data-testid="image-go-providers">
            {t("settings.images.goProviders")}
          </button>
        </div>
      ) : null}

      {active?.configured ? (
        <>
          <p className="sd" data-testid="image-state">
            {active.consented ? t("settings.images.state.ready") : t("settings.images.state.needsConsent")}
          </p>

          <div className="modal-field" data-testid="image-model">
            <label htmlFor="image-model-input">{t("settings.images.model")}</label>
            <input
              id="image-model-input"
              list="image-model-list"
              value={modelDraft}
              placeholder={active.id === "openai-codex" ? t("settings.images.model.auto") : t("settings.images.model.pick")}
              disabled={busy}
              onChange={(e) => setModelDraft(e.target.value)}
              onBlur={saveModel}
              data-testid="image-model-input"
            />
            <datalist id="image-model-list">
              {models.map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
            <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void loadModels()} data-testid="image-model-refresh">
              {t("settings.images.model.refresh")}
            </button>
            {modelsError ? (
              <p className="sd" data-testid="image-model-error">
                {t("settings.images.model.listFailed")} {rpcErrorMessage(modelsError, t)}
              </p>
            ) : null}
          </div>

          <label className="modal-field modal-check">
            <input
              type="checkbox"
              checked={active.consented}
              disabled={busy}
              onChange={(e) => saveConsent(e.target.checked)}
              data-testid="image-consent"
            />
            {t("settings.images.consent", { provider: providerName(active.id) })}
          </label>
          <p className="sd" data-testid="image-note">
            {t(`settings.images.note.${active.id}` as MessageKey)}
          </p>
        </>
      ) : null}

      {error ? (
        <p className="sd" role="alert" data-testid="image-error">
          {rpcErrorMessage(error, t)}
        </p>
      ) : null}
    </section>
  );
}
