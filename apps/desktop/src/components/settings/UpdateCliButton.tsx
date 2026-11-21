import { useState } from "react";
import { providers as providersApi } from "../../lib/rpc";
import { useI18n } from "../../lib/i18n";
import { rpcErrorMessage } from "../../lib/rpcMessage";

// Runs `claude update` on the Claude Code CLI Linetta uses. Offered where a
// model was refused for needing a newer CLI (provider_cli_outdated) and on the
// settings pane's Claude Code card. The engine looks for the CLI again
// afterwards, so the next turn runs the updated binary without a restart.
export function UpdateCliButton({
  label,
  updatedLabel,
  onUpdated,
}: {
  label: string;
  updatedLabel: (version: string) => string;
  onUpdated?: () => void;
}) {
  const { t } = useI18n();
  const [state, setState] = useState<"idle" | "updating" | "done">("idle");
  const [message, setMessage] = useState("");

  const run = async () => {
    setState("updating");
    setMessage("");
    try {
      const res = await providersApi.updateCli();
      setState("done");
      setMessage(updatedLabel(res.version ?? ""));
      onUpdated?.();
    } catch (err) {
      setState("idle");
      setMessage(rpcErrorMessage(err, t));
    }
  };

  return (
    <span data-testid="update-cli">
      {state === "done" ? null : (
        <button
          type="button"
          className="btn ghost sm"
          disabled={state === "updating"}
          onClick={() => void run()}
          data-testid="update-cli-button"
        >
          {state === "updating" ? t("settings.providers.cli.updating") : label}
        </button>
      )}
      {message ? (
        <span className="sd" role="status" data-testid="update-cli-message">
          {" "}
          {message}
        </span>
      ) : null}
    </span>
  );
}
