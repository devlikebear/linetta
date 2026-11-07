import { useEffect, useMemo, useState } from "react";
import { ledger as ledgerApi } from "../lib/rpc";
import type { KnowsAt, KnowsScene } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";

/** Who knows which open secret at a scene (longform Phase 7). The reader's row
 *  is worked out from the questions the story has paid, the characters' rows
 *  from knows facts that point at a question. Answers never appear here: the
 *  columns are the questions, and the answer stays folded on the question. */

interface Props {
  projectId: string;
  /** The scene open in the editor, picked first when it is on the story. */
  nodeId?: string;
  /** Changes whenever the ledger was reloaded, so the table follows edits. */
  version?: unknown;
  onOpenNode?: (nodeId: string) => void;
}

export function KnowsTable({ projectId, nodeId, version, onOpenNode }: Readonly<Props>) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [scene, setScene] = useState(nodeId ?? "");
  const [data, setData] = useState<KnowsAt | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => { setScene(nodeId ?? ""); }, [nodeId]);
  useEffect(() => {
    // Fetched only while unfolded: the section is closed most of the time.
    if (!open) return;
    let cancelled = false;
    ledgerApi.knowsAt(projectId, scene)
      .then((res) => { if (!cancelled) { setData(res); setError(null); } })
      .catch((err) => { if (!cancelled) setError(err); });
    return () => { cancelled = true; };
  }, [open, projectId, scene, version]);

  const labels = useMemo(() => {
    const m = new Map<string, string>();
    for (const s of data?.scenes ?? []) m.set(s.node_id, sceneLabel(s));
    return m;
  }, [data]);

  const picked = data?.node_id ?? "";
  return (
    <details className="knows-table" onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}>
      <summary>{t("knows.title")}</summary>
      <p className="style-hint">{t("knows.hint")}</p>
      {error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}
      <label className="knows-scene">
        <span>{t("knows.scene")}</span>
        <select aria-label={t("knows.scene")} value={picked} onChange={(e) => setScene(e.target.value)}>
          <option value="">{t("knows.pickScene")}</option>
          {(data?.scenes ?? []).map((s) => <option key={s.node_id} value={s.node_id}>{sceneLabel(s)}</option>)}
        </select>
      </label>
      {picked !== "" && data != null && (
        data.secrets.length === 0 ? (
          <p className="style-hint">{t("knows.noSecrets")}</p>
        ) : (
          <div className="knows-scroll">
            <table>
              <thead>
                <tr>
                  <th scope="col">{t("knows.who")}</th>
                  {data.secrets.map((q) => (
                    <th key={q.id} scope="col">{q.subject ? `${q.subject} — ${q.body}` : q.body}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                <tr className="knows-reader">
                  <th scope="row">{t("knows.reader")}</th>
                  {data.secrets.map((q) => (
                    <td key={q.id} data-knows="no">
                      {t("knows.notKnows")}
                      {q.paid_node_id && (
                        <button type="button" className="ledger-where" onClick={() => onOpenNode?.(q.paid_node_id ?? "")}>
                          {t("knows.readerLearns", { label: labels.get(q.paid_node_id) ?? "" })}
                        </button>
                      )}
                    </td>
                  ))}
                </tr>
                {data.rows.map((row) => (
                  <tr key={row.entity_id}>
                    <th scope="row">{row.subject}</th>
                    {data.secrets.map((q) => {
                      const cell = row.cells[q.id];
                      if (!cell) return <td key={q.id} data-knows="none">{t("knows.noRecord")}</td>;
                      const where = cell.node_id ? t("knows.recordedIn", { label: labels.get(cell.node_id) ?? "" }) : undefined;
                      return (
                        <td key={q.id} data-knows={cell.knows ? "yes" : "no"} title={where}>
                          {t(cell.knows ? "knows.knows" : "knows.notKnows")}
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      )}
      {picked !== "" && data != null && data.rows.length === 0 && (
        <p className="style-hint">{t("knows.noCast")}</p>
      )}
    </details>
  );
}

function sceneLabel(s: KnowsScene): string {
  return s.parent_label ? `${s.parent_label} · ${s.label}` : s.label;
}
