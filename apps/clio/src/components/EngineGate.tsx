import { useCallback, useEffect, useState, type ReactNode } from "react";
import { enginePing } from "../lib/rpc";
import { t } from "../lib/i18n";

interface Props {
  children: ReactNode;
}

/** Holds the app back until the engine answers ping. */
export function EngineGate({ children }: Props) {
  const [state, setState] = useState<"checking" | "ready" | "failed">("checking");
  const [error, setError] = useState("");

  const check = useCallback(() => {
    setState("checking");
    enginePing()
      .then(() => setState("ready"))
      .catch((e) => {
        setError(e instanceof Error ? e.message : String(e));
        setState("failed");
      });
  }, []);

  useEffect(() => {
    check();
  }, [check]);

  if (state === "ready") return <>{children}</>;

  if (state === "checking") {
    return (
      <main className="gate">
        <h1>{t("app.name")}</h1>
        <p className="muted">{t("engine.checking")}</p>
      </main>
    );
  }

  return (
    <main className="gate">
      <h1>{t("engine.failedTitle")}</h1>
      <p className="error">{error}</p>
      <div className="row">
        <button type="button" onClick={check}>{t("engine.retry")}</button>
        <button type="button" onClick={() => navigator.clipboard?.writeText(error)}>
          {t("engine.copyDiagnostics")}
        </button>
      </div>
    </main>
  );
}
