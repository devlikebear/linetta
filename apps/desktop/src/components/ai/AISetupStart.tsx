import { ExternalLink, X } from "lucide-react";
import { useMemo } from "react";
import { useI18n } from "../../lib/i18n";
import {
  OPENROUTER_DEFAULT_MODEL_OPTIONS,
  OPENROUTER_RECOMMENDED_MODELS,
  OPENROUTER_SMART_DEFAULT_MODEL,
  organizeOpenRouterModelOptions,
} from "../../lib/openRouterDefaults";
import type { OpenRouterKeyInfo, ProviderID } from "../../lib/types";
import "./AISetupStart.css";

export type GuideID = "openrouter-safe" | "chatgpt-subscription" | "openai-api" | "claude-api" | "gemini-api";

export interface SetupGuide {
  id: GuideID;
  provider: ProviderID;
  title: string;
  badge: string;
  summary: string;
  policy: string;
  steps: string[];
  action: string;
  links: Array<{ label: string; href: string }>;
}

type Translate = ReturnType<typeof useI18n>["t"];

const SETUP_GUIDE_LINKS: Record<GuideID, Array<{ labelKey: string; href: string }>> = {
  "openrouter-safe": [
    { labelKey: "settings.setup.openrouter.linkKeys", href: "https://openrouter.ai/keys" },
    { labelKey: "settings.setup.openrouter.linkLimits", href: "https://openrouter.ai/docs/api/reference/limits" },
  ],
  "chatgpt-subscription": [
    { labelKey: "settings.setup.chatgpt.linkCodexCli", href: "https://developers.openai.com/codex/cli" },
    { labelKey: "settings.setup.chatgpt.linkBilling", href: "https://help.openai.com/en/articles/9039756-managing-billing-settings-on-chatgpt-web-and-platform" },
  ],
  "openai-api": [
    { labelKey: "settings.setup.openai.linkApiKey", href: "https://help.openai.com/en/articles/4936850-where-do-i-find-my-openai-api-key" },
    { labelKey: "settings.setup.openai.linkPricing", href: "https://openai.com/api/pricing/" },
  ],
  "claude-api": [
    { labelKey: "settings.setup.claude.linkAccess", href: "https://support.claude.com/en/articles/8114521-how-can-i-access-the-claude-api" },
    { labelKey: "settings.setup.claude.linkAuth", href: "https://platform.claude.com/docs/en/manage-claude/authentication" },
    { labelKey: "settings.setup.claude.linkPolicy", href: "https://code.claude.com/docs/en/legal-and-compliance" },
  ],
  "gemini-api": [
    { labelKey: "settings.setup.gemini.linkApiKey", href: "https://ai.google.dev/gemini-api/docs/api-key" },
    { labelKey: "settings.setup.gemini.linkBilling", href: "https://ai.google.dev/gemini-api/docs/billing" },
  ],
};

export function guideForProvider(provider: ProviderID): GuideID {
  switch (provider) {
    case "openrouter":
      return "openrouter-safe";
    case "openai":
      return "openai-api";
    case "anthropic":
      return "claude-api";
    case "gemini-native":
      return "gemini-api";
    case "openai-codex":
    case "claude-code-cli":
    default:
      return "chatgpt-subscription";
  }
}

export function buildSetupGuides(t: Translate): SetupGuide[] {
  return [
    {
      id: "openrouter-safe",
      provider: "openrouter",
      title: t("settings.setup.openrouter.title"),
      badge: t("settings.setup.openrouter.badge"),
      summary: t("settings.setup.openrouter.summary"),
      policy: t("settings.setup.openrouter.policy"),
      action: t("settings.setup.openrouter.action"),
      steps: [
        t("settings.setup.openrouter.step1"),
        t("settings.setup.openrouter.step2"),
        t("settings.setup.openrouter.step3"),
        t("settings.setup.openrouter.step4"),
        t("settings.setup.openrouter.step5"),
      ],
      links: setupGuideLinks(t, "openrouter-safe"),
    },
    {
      id: "chatgpt-subscription",
      provider: "openai-codex",
      title: t("settings.setup.chatgpt.title"),
      badge: t("settings.setup.chatgpt.badge"),
      summary: t("settings.setup.chatgpt.summary"),
      policy: t("settings.setup.chatgpt.policy"),
      action: t("settings.setup.chatgpt.action"),
      steps: [
        t("settings.setup.chatgpt.step1"),
        t("settings.setup.chatgpt.step2"),
        t("settings.setup.chatgpt.step3"),
        t("settings.setup.chatgpt.step4"),
        t("settings.setup.chatgpt.step5"),
      ],
      links: setupGuideLinks(t, "chatgpt-subscription"),
    },
    {
      id: "openai-api",
      provider: "openai",
      title: t("settings.setup.openai.title"),
      badge: t("settings.setup.openai.badge"),
      summary: t("settings.setup.openai.summary"),
      policy: t("settings.setup.openai.policy"),
      action: t("settings.setup.openai.action"),
      steps: [
        t("settings.setup.openai.step1"),
        t("settings.setup.openai.step2"),
        t("settings.setup.openai.step3"),
        t("settings.setup.openai.step4"),
        t("settings.setup.openai.step5"),
      ],
      links: setupGuideLinks(t, "openai-api"),
    },
    {
      id: "claude-api",
      provider: "anthropic",
      title: t("settings.setup.claude.title"),
      badge: t("settings.setup.claude.badge"),
      summary: t("settings.setup.claude.summary"),
      policy: t("settings.setup.claude.policy"),
      action: t("settings.setup.claude.action"),
      steps: [
        t("settings.setup.claude.step1"),
        t("settings.setup.claude.step2"),
        t("settings.setup.claude.step3"),
        t("settings.setup.claude.step4"),
        t("settings.setup.claude.step5"),
      ],
      links: setupGuideLinks(t, "claude-api"),
    },
    {
      id: "gemini-api",
      provider: "gemini-native",
      title: t("settings.setup.gemini.title"),
      badge: t("settings.setup.gemini.badge"),
      summary: t("settings.setup.gemini.summary"),
      policy: t("settings.setup.gemini.policy"),
      action: t("settings.setup.gemini.action"),
      steps: [
        t("settings.setup.gemini.step1"),
        t("settings.setup.gemini.step2"),
        t("settings.setup.gemini.step3"),
        t("settings.setup.gemini.step4"),
        t("settings.setup.gemini.step5"),
      ],
      links: setupGuideLinks(t, "gemini-api"),
    },
  ];
}

function setupGuideLinks(t: Translate, id: GuideID): SetupGuide["links"] {
  return SETUP_GUIDE_LINKS[id].map((link) => ({
    label: t(link.labelKey),
    href: link.href,
  }));
}

interface AISetupStartProps {
  variant?: "settings" | "modal";
  currentProvider: ProviderID;
  currentProviderLabel: string;
  credentialState: string;
  unavailableProviders: string[];
  selectedGuideId: GuideID;
  onGuideIdChange: (id: GuideID) => void;
  onSelectProvider: (provider: ProviderID) => void;
  openRouterKeyInfo?: OpenRouterKeyInfo | null;
  openRouterKeyInfoLoading?: boolean;
  openRouterKeyInfoError?: string;
  onRefreshOpenRouterKeyInfo?: () => void;
  onConnectOpenRouterOAuth?: () => void;
  openRouterOAuthBusy?: boolean;
  openRouterOAuthURL?: string;
  openRouterOAuthError?: string;
  openRouterAPIKeyDraft?: string;
  openRouterAPIKeySaved?: boolean;
  openRouterModelDraft?: string;
  openRouterModelOptions?: string[];
  openRouterModelsLoading?: boolean;
  openRouterModelsError?: string;
  openRouterSetupBusy?: boolean;
  openRouterTestMessage?: { kind: "ok" | "error"; text: string } | null;
  onOpenRouterAPIKeyChange?: (value: string) => void;
  onOpenRouterModelChange?: (value: string) => void;
  onSaveOpenRouter?: () => void;
  onClearOpenRouterAPIKey?: () => void;
  onRefreshOpenRouterModels?: () => void;
  onTestOpenRouter?: () => void;
  saving?: boolean;
  onClose?: () => void;
}

export function AISetupStart({
  variant = "settings",
  currentProvider,
  currentProviderLabel,
  credentialState,
  unavailableProviders,
  selectedGuideId,
  onGuideIdChange,
  onSelectProvider,
  openRouterKeyInfo = null,
  openRouterKeyInfoLoading = false,
  openRouterKeyInfoError = "",
  onRefreshOpenRouterKeyInfo,
  onConnectOpenRouterOAuth,
  openRouterOAuthBusy = false,
  openRouterOAuthURL = "",
  openRouterOAuthError = "",
  openRouterAPIKeyDraft = "",
  openRouterAPIKeySaved = false,
  openRouterModelDraft = OPENROUTER_SMART_DEFAULT_MODEL,
  openRouterModelOptions = OPENROUTER_DEFAULT_MODEL_OPTIONS,
  openRouterModelsLoading = false,
  openRouterModelsError = "",
  openRouterSetupBusy = false,
  openRouterTestMessage = null,
  onOpenRouterAPIKeyChange,
  onOpenRouterModelChange,
  onSaveOpenRouter,
  onClearOpenRouterAPIKey,
  onRefreshOpenRouterModels,
  onTestOpenRouter,
  saving = false,
  onClose,
}: AISetupStartProps) {
  const { t } = useI18n();
  const isModal = variant === "modal";
  const setupGuides = useMemo(
    () => buildSetupGuides(t).filter((g) => !unavailableProviders.includes(g.provider)),
    [t, unavailableProviders],
  );
  const selectedGuide = setupGuides.find((g) => g.id === selectedGuideId) ?? setupGuides[0];
  if (!selectedGuide) return null;

  const steps = <SetupSteps guide={selectedGuide} collapsed={isModal} label={t("settings.aiWizard.stepsSummary")} />;
  const openRouterInlineSetup = selectedGuide.id === "openrouter-safe" ? (
    <OpenRouterInlineSetup
      variant={variant}
      apiKeyDraft={openRouterAPIKeyDraft}
      apiKeySaved={openRouterAPIKeySaved}
      modelDraft={openRouterModelDraft}
      modelOptions={openRouterModelOptions}
      modelsLoading={openRouterModelsLoading}
      modelsError={openRouterModelsError}
      busy={saving || openRouterSetupBusy}
      testMessage={openRouterTestMessage}
      onAPIKeyChange={onOpenRouterAPIKeyChange}
      onModelChange={onOpenRouterModelChange}
      onSave={onSaveOpenRouter}
      onClearAPIKey={onClearOpenRouterAPIKey}
      onRefreshModels={onRefreshOpenRouterModels}
      onTest={onTestOpenRouter}
    />
  ) : null;
  const showOpenRouterKeyInfo =
    selectedGuide.id === "openrouter-safe" &&
    (!isModal || openRouterKeyInfo || openRouterKeyInfoLoading || openRouterKeyInfoError);

  return (
    <section className={`ai-setup-start ${variant === "settings" ? "settings-section" : "is-modal"}`}>
      <div className="settings-section-head">
        <h3 id={variant === "modal" ? "ai-setup-start-title" : undefined}>{t("settings.aiWizard.title")}</h3>
        <div className="ai-setup-start-head-actions">
          <span className="setup-pill">{t("settings.aiWizard.badge")}</span>
          {onClose && (
            <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}>
              <X size={16} />
            </button>
          )}
        </div>
      </div>
      <p className="sd">{t("settings.aiWizard.description")}</p>
      <div className="setup-current">
        <span>
          {t("settings.aiWizard.current")}: <strong>{currentProviderLabel || currentProvider}</strong>
        </span>
        <span>{credentialState}</span>
      </div>
      <div className="setup-policy">{t("settings.aiWizard.policy")}</div>
      <div
        className={isModal ? "setup-choice-tabs" : "setup-choice-list"}
        role={isModal ? "tablist" : undefined}
        aria-label={isModal ? t("settings.aiWizard.choiceTabs") : undefined}
      >
        {setupGuides.map((guide) => (
          <button
            key={guide.id}
            type="button"
            role={isModal ? "tab" : undefined}
            aria-selected={isModal ? guide.id === selectedGuide.id : undefined}
            className={`${isModal ? "setup-tab" : "setup-choice"}${guide.id === selectedGuide.id ? " is-selected" : ""}`}
            onClick={() => onGuideIdChange(guide.id)}
          >
            <span className="setup-choice-main">
              <span className="setup-choice-title">{guide.title}</span>
              {!isModal && <span className="setup-choice-summary">{guide.summary}</span>}
            </span>
            {!isModal && <span className="setup-choice-badge">{guide.badge}</span>}
          </button>
        ))}
      </div>
      <div className="setup-guide">
        <div className="setup-guide-head">
          <div>
            <h4>{selectedGuide.title}</h4>
            <p>{selectedGuide.policy}</p>
          </div>
          <div className="setup-guide-actions">
            {selectedGuide.id === "openrouter-safe" && onConnectOpenRouterOAuth && (
              <button
                type="button"
                className="btn sm"
                onClick={() => !saving && !openRouterOAuthBusy && onConnectOpenRouterOAuth()}
                disabled={saving || openRouterOAuthBusy}
              >
                {openRouterOAuthBusy ? t("settings.setup.openrouter.oauthWaiting") : t("settings.setup.openrouter.oauthAction")}
              </button>
            )}
            <button
              type="button"
              className="btn ghost sm"
              onClick={() => !saving && onSelectProvider(selectedGuide.provider)}
              disabled={saving}
            >
              {currentProvider === selectedGuide.provider ? t("settings.aiWizard.selected") : selectedGuide.action}
            </button>
          </div>
        </div>
        {selectedGuide.id === "openrouter-safe" && (openRouterOAuthError || openRouterOAuthURL) && (
          <div className="setup-oauth-note">
            {openRouterOAuthError && <p className="sd">{openRouterOAuthError}</p>}
            {openRouterOAuthURL && (
              <a href={openRouterOAuthURL} target="_blank" rel="noreferrer">
                <ExternalLink size={13} /> {t("settings.setup.openrouter.oauthManualLink")}
              </a>
            )}
          </div>
        )}
        {isModal ? openRouterInlineSetup : steps}
        {isModal ? steps : openRouterInlineSetup}
        {showOpenRouterKeyInfo && (
          <>
            <OpenRouterKeyInfoPanel
              info={openRouterKeyInfo}
              loading={openRouterKeyInfoLoading}
              error={openRouterKeyInfoError}
              onRefresh={onRefreshOpenRouterKeyInfo}
            />
          </>
        )}
        <div className="setup-links" aria-label={`${selectedGuide.title} ${t("settings.aiWizard.officialGuide")}`}>
          {selectedGuide.links.map((link) => (
            <a key={link.href} href={link.href} target="_blank" rel="noreferrer">
              <ExternalLink size={13} /> {link.label}
            </a>
          ))}
        </div>
      </div>
    </section>
  );
}

function SetupSteps({ guide, collapsed, label }: { guide: SetupGuide; collapsed: boolean; label: string }) {
  const list = (
    <ol className="setup-steps">
      {guide.steps.map((step) => (
        <li key={step}>{step}</li>
      ))}
    </ol>
  );
  if (!collapsed) return list;
  return (
    <details className="setup-steps-disclosure">
      <summary>{label}</summary>
      {list}
    </details>
  );
}

function OpenRouterInlineSetup({
  variant,
  apiKeyDraft,
  apiKeySaved,
  modelDraft,
  modelOptions,
  modelsLoading,
  modelsError,
  busy,
  testMessage,
  onAPIKeyChange,
  onModelChange,
  onSave,
  onClearAPIKey,
  onRefreshModels,
  onTest,
}: {
  variant: "settings" | "modal";
  apiKeyDraft: string;
  apiKeySaved: boolean;
  modelDraft: string;
  modelOptions: string[];
  modelsLoading: boolean;
  modelsError: string;
  busy: boolean;
  testMessage: { kind: "ok" | "error"; text: string } | null;
  onAPIKeyChange?: (value: string) => void;
  onModelChange?: (value: string) => void;
  onSave?: () => void;
  onClearAPIKey?: () => void;
  onRefreshModels?: () => void;
  onTest?: () => void;
}) {
  const { t } = useI18n();
  if (!onAPIKeyChange && !onSave && !onRefreshModels && !onTest) return null;
  const modelListId = `openrouter-model-options-${variant}`;
  const apiKeyId = `openrouter-api-key-${variant}`;
  const modelId = `openrouter-model-${variant}`;
  const organizedModelOptions = organizeOpenRouterModelOptions(modelOptions);
  return (
    <div className="setup-inline-config">
      <div className="modal-field">
        <label htmlFor={apiKeyId}>{t("settings.setup.openrouter.apiKeyLabel")}</label>
        <div className="set-field-row">
          <input
            id={apiKeyId}
            type="password"
            value={apiKeyDraft}
            onChange={(e) => onAPIKeyChange?.(e.currentTarget.value)}
            placeholder={
              apiKeySaved
                ? t("settings.setup.openrouter.apiKeySavedPlaceholder")
                : t("settings.setup.openrouter.apiKeyPlaceholder")
            }
            autoComplete="off"
            disabled={busy}
          />
          {apiKeySaved && onClearAPIKey && (
            <button type="button" className="btn ghost sm" onClick={onClearAPIKey} disabled={busy}>
              {t("common.deleteKey")}
            </button>
          )}
          {onSave && (
            <button type="button" className="btn sm" onClick={onSave} disabled={busy || !apiKeyDraft.trim()}>
              {t("settings.setup.openrouter.saveKey")}
            </button>
          )}
        </div>
        <p className="sd">{t("settings.setup.openrouter.inlineHelp")}</p>
      </div>
      <div className="modal-field">
        <label htmlFor={modelId}>{t("settings.setup.openrouter.modelLabel")}</label>
        <div className="openrouter-model-presets" aria-label={t("settings.setup.openrouter.recommendedModels")}>
          <span className="openrouter-model-presets-title">{t("settings.setup.openrouter.recommendedModels")}</span>
          <div className="openrouter-model-preset-grid">
            {OPENROUTER_RECOMMENDED_MODELS.map((preset) => (
              <button
                key={preset.id}
                type="button"
                className={modelDraft.trim() === preset.id ? "openrouter-model-preset is-selected" : "openrouter-model-preset"}
                onClick={() => onModelChange?.(preset.id)}
                disabled={busy}
              >
                <span className="openrouter-model-preset-title">{t(preset.titleKey)}</span>
                <span className="openrouter-model-preset-summary">{t(preset.summaryKey)}</span>
                <code>{preset.id}</code>
              </button>
            ))}
          </div>
        </div>
        <div className="set-field-row">
          <input
            id={modelId}
            type="text"
            list={modelListId}
            value={modelDraft}
            onChange={(e) => onModelChange?.(e.currentTarget.value)}
            placeholder={OPENROUTER_SMART_DEFAULT_MODEL}
            disabled={busy}
          />
          <datalist id={modelListId}>
            {organizedModelOptions.map((model) => (
              <option key={model} value={model} />
            ))}
          </datalist>
          {onRefreshModels && (
            <button type="button" className="btn ghost sm" onClick={onRefreshModels} disabled={busy || modelsLoading}>
              {modelsLoading ? t("settings.setup.openrouter.modelsLoading") : t("settings.setup.openrouter.refreshModels")}
            </button>
          )}
          {onTest && (
            <button type="button" className="btn ghost sm" onClick={onTest} disabled={busy}>
              {t("settings.provider.test")}
            </button>
          )}
        </div>
        <p className="sd">{t("settings.setup.openrouter.modelHelp")}</p>
        {modelsError ? <p className="error">{modelsError}</p> : null}
        {testMessage ? (
          <p className={testMessage.kind === "ok" ? "provider-test-ok" : "provider-test-error"}>{testMessage.text}</p>
        ) : null}
      </div>
    </div>
  );
}

function OpenRouterKeyInfoPanel({
  info,
  loading,
  error,
  onRefresh,
}: {
  info: OpenRouterKeyInfo | null;
  loading: boolean;
  error: string;
  onRefresh?: () => void;
}) {
  const { t } = useI18n();
  return (
    <div className="setup-key-info">
      <div className="setup-key-info-head">
        <strong>{t("settings.setup.openrouter.keyInfoTitle")}</strong>
        {onRefresh && (
          <button type="button" className="btn ghost sm" onClick={onRefresh} disabled={loading}>
            {loading ? t("settings.setup.openrouter.keyInfoLoading") : t("settings.setup.openrouter.keyInfoRefresh")}
          </button>
        )}
      </div>
      {error ? <p className="sd">{error}</p> : null}
      {!error && !loading && !info ? <p className="sd">{t("settings.setup.openrouter.keyInfoUnavailable")}</p> : null}
      {loading && !info ? <p className="sd">{t("settings.setup.openrouter.keyInfoLoading")}</p> : null}
      {info && (
        <dl className="setup-key-info-grid">
          <div>
            <dt>{t("settings.setup.openrouter.keyInfoRemaining")}</dt>
            <dd>{formatCredit(info.limit_remaining)}</dd>
          </div>
          <div>
            <dt>{t("settings.setup.openrouter.keyInfoLimit")}</dt>
            <dd>{formatCredit(info.limit)}</dd>
          </div>
          <div>
            <dt>{t("settings.setup.openrouter.keyInfoMonthlyUsage")}</dt>
            <dd>{formatCredit(info.usage_monthly)}</dd>
          </div>
        </dl>
      )}
    </div>
  );
}

function formatCredit(value: number | null | undefined): string {
  if (value == null) return "--";
  return `$${value.toFixed(2)}`;
}
