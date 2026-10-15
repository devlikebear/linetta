import type { MessageKey } from "./i18n";
import type { ProviderID, ProviderStatus } from "./types";

/** The order the engine's providers.list returns, kept explicit so a control
 *  listing them does not silently reorder when a response is cached.
 *  labelKey is written as a literal (not a template string) so a typo fails
 *  type-checking instead of silently falling through at render time. Shared
 *  by the settings pane and the agent panel's picker, so the two can never
 *  name the same provider differently. */
export const PROVIDER_ORDER: { id: ProviderID; labelKey: MessageKey }[] = [
  { id: "openai-codex", labelKey: "settings.providers.name.openai-codex" },
  { id: "anthropic", labelKey: "settings.providers.name.anthropic" },
  { id: "gemini-native", labelKey: "settings.providers.name.gemini-native" },
  { id: "openai", labelKey: "settings.providers.name.openai" },
  { id: "claude-code", labelKey: "settings.providers.name.claude-code" },
];

/** The providers this build actually offers, in PROVIDER_ORDER. The App Store
 *  and mobile builds cannot spawn the Claude Code CLI and leave it out of
 *  providers.list, so a control listing providers follows the rows rather
 *  than the full list. Before any rows have arrived, the CLI is held back
 *  rather than shown and then taken away. */
export const offeredProviders = (rows: ProviderStatus[]) =>
  rows.length > 0
    ? PROVIDER_ORDER.filter((p) => rows.some((r) => r.id === p.id))
    : PROVIDER_ORDER.filter((p) => p.id !== "claude-code");

/** Whether a provider takes an API key typed into the settings pane. Codex
 *  signs in through the browser and Claude Code through its own CLI. */
export const takesApiKey = (id: ProviderID) => id !== "openai-codex" && id !== "claude-code";

/** The i18n key naming one provider, for interpolation into a sentence that
 *  has to say which company it means. */
export const nameKeyFor = (id: ProviderID): MessageKey =>
  PROVIDER_ORDER.find((p) => p.id === id)!.labelKey;

/** Whether the agent can start a turn on this provider: a credential and the
 *  writer's consent to send it manuscript text. The engine refuses a turn
 *  without either (provider_not_configured / provider_consent_required), so
 *  anything that offers a provider to talk to checks the same two fields. */
export const isProviderReady = (row: ProviderStatus | undefined): boolean =>
  Boolean(row?.configured && row?.consented);
