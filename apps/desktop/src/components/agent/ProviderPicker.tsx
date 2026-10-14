import { useEffect, useRef, useState } from "react";
import { useI18n } from "../../lib/i18n";
import { providers as providersApi, settings as settingsApi } from "../../lib/rpc";
import { dispatchAppEvent } from "../../lib/appEvents";
import { isProviderReady, offeredProviders } from "../../lib/providers";
import type { ProviderID, ProviderStatus } from "../../lib/types";

/** Which provider and model the next turn goes to, and a way to change both
 *  without leaving the conversation.
 *
 *  The engine resolves the provider at the start of every turn (agent.run →
 *  Providers.Client("")), so a change here applies from the next send on and
 *  never to a turn already running — which is why the picker is disabled
 *  while one is. Only a provider the agent could actually start a turn on is
 *  selectable: switching to one without a key or without consent would swap
 *  the conversation for the "set up a provider" notice, a worse place to
 *  find out than the settings pane, which is where setting one up happens.
 *
 *  The model list is fetched on first focus rather than on mount. It is a
 *  network call to the provider, and most turns are sent without anyone
 *  looking at the model — opening the panel should not cost one. */
export function ProviderPicker({
  rows,
  disabled,
  onRows,
  onError,
}: {
  rows: ProviderStatus[];
  disabled: boolean;
  /** A fresh providers.list after a change, for the panel to adopt. */
  onRows: (rows: ProviderStatus[]) => void;
  onError: (err: unknown) => void;
}) {
  const { t } = useI18n();
  const active = rows.find((row) => row.active);
  const [saving, setSaving] = useState(false);
  /** A model list belongs to the provider it was fetched for — Anthropic's
   *  ids mean nothing to Gemini — so it carries that provider's id, and a
   *  list for any other provider is simply not shown. */
  const [models, setModels] = useState<{ provider: ProviderID; ids: string[] } | null>(null);
  const [modelsState, setModelsState] = useState<"idle" | "loading" | "failed">("idle");
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  if (!active) return null;
  const activeId = active.id;
  const storedModel = active.model ?? "";

  async function apply(change: () => Promise<unknown>) {
    setSaving(true);
    try {
      await change();
      const next = await providersApi.list();
      if (mountedRef.current) onRows(next);
    } catch (err) {
      if (mountedRef.current) onError(err);
    } finally {
      if (mountedRef.current) setSaving(false);
    }
  }

  function chooseProvider(id: ProviderID) {
    if (id === activeId) return;
    setModelsState("idle");
    void apply(async () => {
      const settings = await settingsApi.set({ provider: id });
      // The rest of the app reads settings from this event, the way the
      // settings route announces its own saves.
      dispatchAppEvent("linetta:settings-updated", settings);
    });
  }

  function chooseModel(model: string) {
    if (model === storedModel) return;
    void apply(async () => {
      const settings = await settingsApi.set({ providers: { [activeId]: { model } } });
      dispatchAppEvent("linetta:settings-updated", settings);
    });
  }

  function loadModels() {
    if (models?.provider === activeId || modelsState === "loading") return;
    const provider = activeId;
    setModelsState("loading");
    providersApi
      .listModels(provider)
      .then(({ models: ids }) => {
        if (!mountedRef.current) return;
        setModels({ provider, ids });
        setModelsState("idle");
      })
      .catch(() => {
        // Not news worth an alert: the stored model and the provider default
        // are still offered, and any other id can be typed in the settings
        // pane. The quiet line below says so.
        if (mountedRef.current) setModelsState("failed");
      });
  }

  const fetched = models?.provider === activeId ? models.ids : [];
  // The stored model first, so a writer whose model the provider no longer
  // lists (or whose list failed to load) still sees what is in use.
  const modelIds = [...new Set([storedModel, ...fetched].filter(Boolean))];
  const locked = disabled || saving;

  return (
    <div className="agent-model-bar" data-testid="agent-provider-picker">
      <label className="chip agent-model-chip">
        <select
          aria-label={t("agentPanel.provider.label")}
          data-testid="agent-provider-select"
          value={activeId}
          disabled={locked}
          onChange={(e) => chooseProvider(e.target.value as ProviderID)}
        >
          {offeredProviders(rows).map(({ id, labelKey }) => {
            const row = rows.find((r) => r.id === id);
            // The active one stays enabled even if it somehow is not ready —
            // a <select> whose current value is a disabled option renders
            // blank in some webviews.
            const usable = id === activeId || isProviderReady(row);
            return (
              <option key={id} value={id} disabled={!usable}>
                {usable
                  ? t(labelKey)
                  : t("agentPanel.provider.needsSetup", { provider: t(labelKey) })}
              </option>
            );
          })}
        </select>
      </label>
      <label className="chip agent-model-chip">
        <select
          aria-label={t("agentPanel.model.label")}
          data-testid="agent-model-select"
          value={storedModel}
          disabled={locked}
          onFocus={loadModels}
          onPointerDown={loadModels}
          onChange={(e) => chooseModel(e.target.value)}
        >
          <option value="">{t("settings.providers.model.default")}</option>
          {modelIds.map((id) => (
            <option key={id} value={id}>
              {id}
            </option>
          ))}
          {modelsState === "loading" ? (
            <option value="__loading" disabled>
              {t("agentPanel.model.loading")}
            </option>
          ) : null}
        </select>
      </label>
      {modelsState === "failed" ? (
        <p className="agent-model-note" data-testid="agent-model-failed">
          {t("agentPanel.model.listFailed")}
        </p>
      ) : null}
    </div>
  );
}
