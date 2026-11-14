import { useEffect, useState } from "react";

import type { MessageKey } from "../../lib/i18n";
import { useI18n } from "../../lib/i18n";
import { agent as agentApi } from "../../lib/rpc";
import type { AgentUsageRow, AgentUsageSummary } from "../../lib/types";

/** The days the section sums. The engine keeps 90. */
const USAGE_DAYS = 30;

const KIND_KEYS: Record<string, MessageKey> = {
  turn: "settings.agentUsage.kind.turn",
  self_review: "settings.agentUsage.kind.self_review",
  settlement: "settings.agentUsage.kind.settlement",
  settlement_catch_up: "settings.agentUsage.kind.settlement_catch_up",
  episode_review: "settings.agentUsage.kind.episode_review",
  impact_review: "settings.agentUsage.kind.impact_review",
  sample_check: "settings.agentUsage.kind.sample_check",
};

/** The share of prompt tokens the provider served from its cache. Works for
 *  both ways providers count: Anthropic's cache reads and writes come on top of
 *  input, OpenAI's cached tokens are part of it. */
export function cacheHitRate(rows: AgentUsageRow[]): number | null {
  let hit = 0;
  let prompt = 0;
  for (const r of rows) {
    hit += r.cache_read + r.cached;
    prompt += r.input + r.cache_read + r.cache_write;
  }
  return prompt > 0 ? hit / prompt : null;
}

/** What the built-in agent's requests have cost lately: the turns themselves
 *  and each background pass, by model, with the cache split — so a writer can
 *  see where the tokens go before turning a pass on or picking a pass model. */
export function AgentUsageSection() {
  const { t, language } = useI18n();
  const [summary, setSummary] = useState<AgentUsageSummary | null>(null);

  useEffect(() => {
    let cancelled = false;
    agentApi
      .usage(USAGE_DAYS)
      .then((s) => {
        if (!cancelled) setSummary(s);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  if (!summary) return null;
  const rows = summary.rows ?? [];
  const num = (n: number) => new Intl.NumberFormat(language, { notation: "compact", maximumFractionDigits: 1 }).format(n);
  const calls = rows.reduce((sum, r) => sum + r.calls, 0);
  const rate = cacheHitRate(rows);

  return (
    <section className="settings-section" data-testid="agent-usage-section">
      <h3>{t("settings.agentUsage.title")}</h3>
      <p className="sd">{t("settings.agentUsage.description", { days: USAGE_DAYS })}</p>
      {rows.length === 0 ? (
        <p className="sd" data-testid="agent-usage-empty">
          {t("settings.agentUsage.empty")}
        </p>
      ) : (
        <>
          <p className="sd" data-testid="agent-usage-total">
            {t("settings.agentUsage.total", {
              calls: num(calls),
              rate: rate === null ? "—" : `${Math.round(rate * 100)}%`,
            })}
          </p>
          {rows.map((r) => {
            const key = KIND_KEYS[r.kind];
            return (
              <div className="set-row" key={`${r.kind}/${r.provider}/${r.model}`} data-testid="agent-usage-row">
                <span className="sk-wrap">
                  <span className="sk">
                    {key ? t(key) : r.kind}
                    {r.model ? ` · ${r.model}` : ""}
                  </span>
                  <span className="sd">
                    {t("settings.agentUsage.row", {
                      calls: num(r.calls),
                      input: num(r.input),
                      cacheRead: num(r.cache_read + r.cached),
                      cacheWrite: num(r.cache_write),
                      output: num(r.output),
                    })}
                  </span>
                </span>
              </div>
            );
          })}
        </>
      )}
    </section>
  );
}
