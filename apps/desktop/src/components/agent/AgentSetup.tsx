import { useState } from "react";
import { Link } from "react-router-dom";

import { useTrialStatus } from "../../hooks/useTrialStatus";
import { useI18n } from "../../lib/i18n";
import { isProviderReady } from "../../lib/providers";
import type { ProviderStatus } from "../../lib/types";
import { TrialSetup } from "../trial/TrialSetup";

/** What the agent panel shows when the active provider cannot take a turn.
 *
 *  A writer with no provider ready at all is offered two ways in: the Linetta
 *  trial first, set up right here, and their own key in Settings. A writer
 *  who has some other provider ready keeps the plain notice: switching is
 *  the picker's and Settings' job, and a trial card would only be in the way. */
export function AgentSetup({ rows, onStarted }: { rows: ProviderStatus[]; onStarted: () => void }) {
  const { t } = useI18n();
  const anyReady = rows.some((row) => isProviderReady(row));
  const offersTrial = rows.some((row) => row.id === "linetta-trial");
  const [trying, setTrying] = useState(false);
  const { status } = useTrialStatus(offersTrial && !anyReady);

  if (anyReady || !offersTrial) {
    return (
      <p className="agent-empty" data-testid="agent-unconfigured">
        {t("agentPanel.unconfigured")}{" "}
        <Link to="/settings">{t("agentPanel.openSettings")}</Link>
      </p>
    );
  }

  if (trying) {
    return (
      <div className="agent-empty agent-setup" data-testid="agent-unconfigured">
        <TrialSetup mode="panel" onStarted={onStarted} onCancel={() => setTrying(false)} />
      </div>
    );
  }

  const used = status?.state === "exhausted";
  return (
    <div className="agent-empty agent-setup" data-testid="agent-unconfigured">
      <button
        type="button"
        className="agent-setup-card is-primary"
        disabled={used}
        onClick={() => setTrying(true)}
        data-testid="agent-try-trial"
      >
        <strong>{used ? t("agentPanel.trial.used") : t("agentPanel.trial.title")}</strong>
        <span>{t("agentPanel.trial.description")}</span>
      </button>
      <Link className="agent-setup-card" to="/settings" data-testid="agent-own-key">
        <strong>{t("agentPanel.ownKey.title")}</strong>
        <span>{t("agentPanel.ownKey.description")}</span>
      </Link>
    </div>
  );
}
