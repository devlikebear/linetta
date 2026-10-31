import { useCallback, useEffect, useMemo, useState } from "react";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { ledger as ledgerApi } from "../lib/rpc";
import type { LedgerEntry, LedgerKind, LedgerState, LedgerStatus, PartArc } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";
import "./LedgerSection.css";

/** The work's account of itself: what the story owes the reader, what has been
 *  settled, what a restructure cut, and what each part is for. Agents keep it
 *  as they write and the brief reads it back; here the writer reads it and has
 *  the last word. Nothing in this section touches the manuscript. */

interface Props {
  projectId: string;
  /** Opens a scene an entry was planted in or paid in. */
  onOpenNode?: (nodeId: string) => void;
}

type Tab = "owed" | "settled" | "closed" | "cut";

const OWED: LedgerKind[] = ["promise", "gag", "question"];
const ADDABLE: LedgerKind[] = ["promise", "gag", "question", "decision", "number", "timeline", "knowledge"];

function tabOf(e: LedgerEntry): Tab {
  if (e.kind === "cut_plan") return "cut";
  if (e.status !== "open") return "closed";
  return OWED.includes(e.kind) ? "owed" : "settled";
}

export function LedgerSection({ projectId, onOpenNode }: Readonly<Props>) {
  const { t } = useI18n();
  const [state, setState] = useState<LedgerState | null>(null);
  const [tab, setTab] = useState<Tab>("owed");
  const [kind, setKind] = useState<LedgerKind>("promise");
  const [body, setBody] = useState("");
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async () => {
    try {
      setState(await ledgerApi.list(projectId));
    } catch (err) {
      setError(err);
    }
  }, [projectId]);

  useEffect(() => { void load(); }, [load]);
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id === projectId) void load();
  });

  const run = async (action: () => Promise<unknown>) => {
    setError(null);
    try {
      await action();
      await load();
    } catch (err) {
      setError(err);
    }
  };

  const shown = useMemo(() => (state?.entries ?? []).filter((e) => tabOf(e) === tab), [state, tab]);
  const owedCount = useMemo(() => (state?.entries ?? []).filter((e) => tabOf(e) === "owed").length, [state]);

  const add = () => run(async () => {
    await ledgerApi.save({ project_id: projectId, kind, body: body.trim() });
    setBody("");
  });
  const setStatus = (e: LedgerEntry, status: LedgerStatus) =>
    run(() => ledgerApi.save({ id: e.id, status }));

  return (
    <details className="sec es-field ledger-section">
      <summary>
        {t("ledger.title")}
        {owedCount > 0 && <span className="style-badge">{t("ledger.owedBadge", { count: owedCount })}</span>}
      </summary>
      <p className="style-hint">{t("ledger.description")}</p>
      {error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}

      {(state?.parts.length ?? 0) > 0 && (
        <div className="ledger-parts" role="group" aria-label={t("ledger.partsTitle")}>
          <p className="ledger-parts-title">{t("ledger.partsTitle")}</p>
          {state?.parts.map((part) => (
            <PartRow key={part.node_id} part={part} onSave={(next) => run(() => ledgerApi.setArc(next))} />
          ))}
        </div>
      )}

      <div className="continuity-tabs" role="tablist">
        {(["owed", "settled", "closed", "cut"] as const).map((key) => (
          <button
            key={key}
            type="button"
            role="tab"
            aria-selected={tab === key}
            className={tab === key ? "is-active" : ""}
            onClick={() => setTab(key)}
          >
            {t(`ledger.tab.${key}`)}
          </button>
        ))}
      </div>

      {shown.length === 0 ? (
        <p className="style-hint">{t("ledger.empty")}</p>
      ) : (
        <ul className="ledger-list">
          {shown.map((e) => (
            <li key={e.id}>
              <span className="ledger-kind">{t(`ledger.kind.${e.kind}`)}</span>
              {e.subject && <span className="ledger-subject">{e.subject}</span>}
              <span className="ledger-body">{e.body}</span>
              {e.planted_node_id && (
                <button type="button" className="ledger-where" onClick={() => onOpenNode?.(e.planted_node_id ?? "")}>
                  {t("ledger.planted", { label: state?.labels[e.planted_node_id] ?? "" })}
                </button>
              )}
              {e.due_node_id && (
                <span className="ledger-where">{t("ledger.due", { label: state?.labels[e.due_node_id] ?? "" })}</span>
              )}
              <span className="ledger-actions">
                {e.status === "open" && e.kind !== "cut_plan" && (
                  <button type="button" className="btn ghost sm" onClick={() => void setStatus(e, "dropped")}>
                    {t("ledger.drop")}
                  </button>
                )}
                {e.status !== "open" && (
                  <button type="button" className="btn ghost sm" onClick={() => void setStatus(e, "open")}>
                    {t("ledger.reopen")}
                  </button>
                )}
                <button type="button" className="btn ghost sm" onClick={() => void run(() => ledgerApi.remove(e.id))}>
                  {t("ledger.delete")}
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}

      <div className="ledger-add">
        <select aria-label={t("ledger.addKind")} value={kind} onChange={(e) => setKind(e.target.value as LedgerKind)}>
          {ADDABLE.map((k) => <option key={k} value={k}>{t(`ledger.kind.${k}`)}</option>)}
        </select>
        <input
          aria-label={t("ledger.addBody")}
          placeholder={t("ledger.addBody")}
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
        <button type="button" className="btn accent sm" disabled={body.trim() === ""} onClick={() => void add()}>
          {t("ledger.add")}
        </button>
      </div>
    </details>
  );
}

function PartRow({ part, onSave }: Readonly<{ part: PartArc; onSave: (next: PartArc) => void }>) {
  const { t } = useI18n();
  const [role, setRole] = useState(part.role);
  const [goal, setGoal] = useState(part.goal);
  const [tension, setTension] = useState(part.tension > 0 ? String(part.tension) : "");
  useEffect(() => {
    setRole(part.role);
    setGoal(part.goal);
    setTension(part.tension > 0 ? String(part.tension) : "");
  }, [part.role, part.goal, part.tension]);
  const commit = () => {
    const level = Math.min(10, Math.max(0, Math.trunc(Number(tension)) || 0));
    if (role === part.role && goal === part.goal && level === part.tension) return;
    onSave({ ...part, role, goal, tension: level });
  };
  return (
    <div className="ledger-part">
      <span className="ledger-part-label">{part.label}</span>
      <input aria-label={t("ledger.partRole", { label: part.label })} placeholder={t("ledger.role")} value={role} onChange={(e) => setRole(e.target.value)} onBlur={commit} />
      <input aria-label={t("ledger.partGoal", { label: part.label })} placeholder={t("ledger.goal")} value={goal} onChange={(e) => setGoal(e.target.value)} onBlur={commit} />
      <input aria-label={t("ledger.partTension", { label: part.label })} placeholder={t("ledger.tension")} inputMode="numeric" value={tension} onChange={(e) => setTension(e.target.value)} onBlur={commit} />
    </div>
  );
}
