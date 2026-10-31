import { useEffect, useState } from "react";
import { nodes as nodesApi } from "../lib/rpc";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";

/** What this scene is meant to do. It sits apart from the summary, so writing
 *  the scene never overwrites it and the plan can be held against the result. */
export function ScenePlanField({ nodeId, plan }: Readonly<{ nodeId: string; plan: string }>) {
  const { t } = useI18n();
  const [value, setValue] = useState(plan);
  const [saved, setSaved] = useState(plan);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    setValue(plan);
    setSaved(plan);
    setError(null);
  }, [nodeId, plan]);

  const commit = async () => {
    if (value === saved) return;
    try {
      await nodesApi.setPlan(nodeId, value);
      setSaved(value);
      setError(null);
    } catch (err) {
      setError(err);
    }
  };

  return (
    <details className="sec es-field" open={plan.trim() !== ""}>
      <summary>{t("scenePlan.title")}</summary>
      <textarea
        rows={4}
        aria-label={t("scenePlan.title")}
        placeholder={t("scenePlan.placeholder")}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={() => void commit()}
      />
      {error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}
    </details>
  );
}
