import { useCallback, useEffect, useMemo, useState } from "react";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { ledger as ledgerApi, mentions as mentionsApi, projects as projectsApi } from "../lib/rpc";
import type { LedgerEntry, LedgerKind, LedgerState, LedgerStatus, PartArc } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";
import { KnowsTable } from "./KnowsTable";
import { TimeTable } from "./TimeTable";
import "./LedgerSection.css";

/** The work's account of itself: what the story owes the reader, what has been
 *  settled, what a restructure cut, and what each part is for. Agents keep it
 *  as they write and the brief reads it back; here the writer reads it and has
 *  the last word. Nothing in this section touches the manuscript. */

interface Props {
  projectId: string;
  /** Opens a scene an entry was planted in or paid in. */
  onOpenNode?: (nodeId: string) => void;
  /** The scene open in the editor; the "who knows what" table starts there. */
  nodeId?: string;
}

type Tab = "owed" | "settled" | "closed" | "cut";

const OWED: LedgerKind[] = ["promise", "gag", "question"];
const ADDABLE: LedgerKind[] = ["promise", "gag", "question", "decision", "number", "timeline", "knowledge"];

/** The engine's attribute names (internal/ledger). They are stored as written,
 *  so the panel sends these and only translates what it shows. */
const ATTR_DATE = "작중 날짜";
const ATTR_ELAPSED = "경과";
const ATTR_AGE = "나이";

/** Which counted attributes a new fact of each kind can carry. An elapsed
 *  count is a timeline fact; an age belongs to a person, whatever the kind. */
function attributesFor(kind: LedgerKind): string[] {
  if (kind === "timeline") return [ATTR_DATE, ATTR_ELAPSED];
  if (kind === "decision" || kind === "number" || kind === "knowledge") return [ATTR_AGE];
  return [];
}

/** An event an elapsed count can point at: a timeline entry that is not
 *  itself a count. Matches the engine's ErrAboutNotEvent rule. */
function isEvent(e: LedgerEntry): boolean {
  return e.kind === "timeline" && e.attribute !== ATTR_ELAPSED;
}

/** Matches the engine's briefPinnedRunes. Past it a pin may not reach the brief. */
const PINNED_BUDGET_RUNES = 3000;

/** Settled facts can be pinned, and so can a question the story still owes
 *  (longform Phase 7): a pinned question's answer is the writer's alone. */
function pinnable(e: LedgerEntry): boolean {
  return tabOf(e) === "settled" || (e.kind === "question" && e.status === "open");
}

function attrKey(a: string): "ledger.attr.date" | "ledger.attr.elapsed" | "ledger.attr.age" {
  if (a === ATTR_DATE) return "ledger.attr.date";
  if (a === ATTR_ELAPSED) return "ledger.attr.elapsed";
  return "ledger.attr.age";
}

function tabOf(e: LedgerEntry): Tab {
  if (e.kind === "cut_plan") return "cut";
  if (e.status !== "open") return "closed";
  return OWED.includes(e.kind) ? "owed" : "settled";
}

export function LedgerSection({ projectId, onOpenNode, nodeId }: Readonly<Props>) {
  const { t } = useI18n();
  const [state, setState] = useState<LedgerState | null>(null);
  const [tab, setTab] = useState<Tab>("owed");
  const [kind, setKind] = useState<LedgerKind>("promise");
  const [body, setBody] = useState("");
  const [attribute, setAttribute] = useState("");
  const [about, setAbout] = useState("");
  const [subject, setSubject] = useState("");
  const [amount, setAmount] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [notice, setNotice] = useState<string | null>(null);

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

  const shown = useMemo(
    () => (state?.entries ?? [])
      .filter((e) => tabOf(e) === tab)
      .sort((a, b) => Number(b.pinned ?? false) - Number(a.pinned ?? false)),
    [state, tab],
  );
  const pinnedOverBudget = useMemo(() => {
    const used = (state?.entries ?? [])
      .filter((e) => e.pinned)
      .reduce((sum, e) => sum + [...e.subject].length + [...e.body].length, 0);
    return used > PINNED_BUDGET_RUNES;
  }, [state]);
  const owedCount = useMemo(() => (state?.entries ?? []).filter((e) => tabOf(e) === "owed").length, [state]);

  const events = useMemo(() => (state?.entries ?? []).filter(isEvent), [state]);
  const attrs = attributesFor(kind);
  const attr = attrs.includes(attribute) ? attribute : "";
  const counted = attr !== "";
  const value = amount.trim() === "" ? undefined : Number(amount);
  const valueOk = value === undefined || Number.isFinite(value);
  // A count is compared only where it was recorded, so it needs the open scene.
  const addBlocked = body.trim() === "" || !valueOk || (counted && !nodeId)
    || (attr === ATTR_ELAPSED && about === "") || (attr === ATTR_AGE && subject.trim() === "");

  const add = () => run(async () => {
    const event = events.find((e) => e.id === about);
    await ledgerApi.save({
      project_id: projectId,
      kind,
      body: body.trim(),
      ...(counted ? { attribute: attr, planted_node_id: nodeId, ...(value === undefined ? {} : { value_num: value }) } : {}),
      ...(attr === ATTR_ELAPSED && event ? { about: event.id, subject: event.subject || event.body } : {}),
      ...(attr === ATTR_AGE ? { subject: subject.trim() } : {}),
    });
    setBody("");
    setAmount("");
  });
  const setStatus = (e: LedgerEntry, status: LedgerStatus) =>
    run(() => ledgerApi.save({ id: e.id, status }));
  const relink = () => run(async () => {
    const done = await mentionsApi.relinkWork(projectId);
    setNotice(t("ledger.relinkDone", { scenes: String(done.scenes), links: String(done.links) }));
  });
  const duplicate = () => run(async () => {
    await projectsApi.duplicate(projectId, t("ledger.duplicateSuffix"));
    setNotice(t("ledger.duplicateDone"));
  });

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

      {tab === "settled" && <p className="style-hint">{t("ledger.pinHint")}</p>}
      {tab === "owed" && shown.some((e) => e.kind === "question") && <p className="style-hint">{t("ledger.questionPinHint")}</p>}
      {(tab === "settled" || tab === "owed") && pinnedOverBudget && <p role="status">{t("ledger.pinnedOverBudget")}</p>}

      {shown.length === 0 ? (
        <p className="style-hint">{t("ledger.empty")}</p>
      ) : (
        <ul className="ledger-list">
          {shown.map((e) => (
            <li key={e.id} className={e.pinned ? "is-pinned" : undefined}>
              <span className="ledger-kind">{t(`ledger.kind.${e.kind}`)}</span>
              {e.subject && <span className="ledger-subject">{e.subject}</span>}
              {e.attribute && <span className="ledger-kind">{e.attribute}</span>}
              {e.strength === "soft" && <span className="ledger-kind">{t("ledger.softTag")}</span>}
              {e.superseded_by && <span className="ledger-kind">{t("ledger.supersededTag")}</span>}
              {e.kind === "question" && (e.answer ?? "") !== "" && <span className="ledger-kind">{t("ledger.answeredTag")}</span>}
              <span className="ledger-body">{e.body}</span>
              {e.kind === "question" && (
                <AnswerField entry={e} onSave={(answer) => run(() => ledgerApi.save({ id: e.id, answer }))} />
              )}
              {e.planted_node_id && (
                <button type="button" className="ledger-where" onClick={() => onOpenNode?.(e.planted_node_id ?? "")}>
                  {t("ledger.planted", { label: state?.labels[e.planted_node_id] ?? "" })}
                </button>
              )}
              {e.due_node_id && (
                <span className="ledger-where">{t("ledger.due", { label: state?.labels[e.due_node_id] ?? "" })}</span>
              )}
              {e.valid_to_node_id && (
                <span className="ledger-where">{t("ledger.validUntil", { label: state?.labels[e.valid_to_node_id] ?? "" })}</span>
              )}
              <span className="ledger-actions">
                {tabOf(e) === "owed" && (
                  <select
                    aria-label={t("ledger.reach.label")}
                    value={e.reach ?? "arc"}
                    onChange={(ev) => void run(() => ledgerApi.save({ id: e.id, reach: ev.target.value as LedgerEntry["reach"] }))}
                  >
                    {(["arc", "volume", "work"] as const).map((r) => <option key={r} value={r}>{t(`ledger.reach.${r}`)}</option>)}
                  </select>
                )}
                {pinnable(e) && (
                  <button
                    type="button"
                    className="btn ghost sm"
                    aria-pressed={e.pinned ?? false}
                    onClick={() => void run(() => ledgerApi.setPinned(e.id, !e.pinned))}
                  >
                    {t(e.pinned ? "ledger.unpin" : "ledger.pin")}
                  </button>
                )}
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
        {attrs.length > 0 && (
          <select aria-label={t("ledger.addAttribute")} value={attr} onChange={(e) => setAttribute(e.target.value)}>
            <option value="">{t("ledger.attr.none")}</option>
            {attrs.map((a) => <option key={a} value={a}>{t(attrKey(a))}</option>)}
          </select>
        )}
        {attr === ATTR_ELAPSED && (
          <select aria-label={t("ledger.addEvent")} value={about} onChange={(e) => setAbout(e.target.value)}>
            <option value="">{t("ledger.pickEvent")}</option>
            {events.map((e) => (
              <option key={e.id} value={e.id}>
                {[e.subject, e.body].filter(Boolean).join(" — ")}
                {e.planted_node_id && state?.labels[e.planted_node_id] ? ` (${state.labels[e.planted_node_id]})` : ""}
              </option>
            ))}
          </select>
        )}
        {attr === ATTR_AGE && (
          <input
            aria-label={t("ledger.addPerson")}
            placeholder={t("ledger.addPerson")}
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
          />
        )}
        {counted && (
          <input
            aria-label={t(attr === ATTR_AGE ? "ledger.addYears" : "ledger.addDays")}
            placeholder={t(attr === ATTR_AGE ? "ledger.addYears" : "ledger.addDays")}
            inputMode="decimal"
            className="ledger-amount"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
          />
        )}
        <input
          aria-label={t("ledger.addBody")}
          placeholder={t("ledger.addBody")}
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
        <button type="button" className="btn accent sm" disabled={addBlocked} onClick={() => void add()}>
          {t("ledger.add")}
        </button>
      </div>
      {counted && (
        <p className="style-hint ledger-add-hint">
          {attr === ATTR_AGE ? t("ledger.yearsHint") : t("ledger.daysHint")}
          {" "}
          {!nodeId ? t("ledger.countNeedsScene") : state?.labels[nodeId] ? t("ledger.countAt", { label: state.labels[nodeId] }) : t("ledger.countAtOpen")}
          {attr === ATTR_ELAPSED && events.length === 0 && ` ${t("ledger.noEvents")}`}
        </p>
      )}
      <div className="ledger-tools">
        <button type="button" className="btn ghost sm" onClick={() => void relink()}>{t("ledger.relink")}</button>
        <button type="button" className="btn ghost sm" onClick={() => void duplicate()}>{t("ledger.duplicate")}</button>
        {notice && <span className="style-hint" role="status">{notice}</span>}
      </div>
      <KnowsTable projectId={projectId} nodeId={nodeId} version={state} onOpenNode={onOpenNode} />
      <TimeTable projectId={projectId} version={state} onOpenNode={onOpenNode} />
    </details>
  );
}

/** A question's answer, folded until the writer asks for it: a writer who
 *  shares or streams their screen should not show the twist by opening the
 *  ledger. Folding again drops it from the page, not only from view. */
function AnswerField({ entry, onSave }: Readonly<{ entry: LedgerEntry; onSave: (answer: string) => void }>) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(entry.answer ?? "");
  useEffect(() => { setDraft(entry.answer ?? ""); }, [entry.answer]);
  const commit = () => {
    const next = draft.trim();
    if (next !== (entry.answer ?? "")) onSave(next);
  };
  return (
    <span className="ledger-answer">
      <button type="button" className="btn ghost sm" aria-expanded={open} onClick={() => setOpen(!open)}>
        {t(open ? "ledger.answer.hide" : "ledger.answer.show")}
      </button>
      {open && (
        <>
          <textarea
            aria-label={t("ledger.answer.label")}
            placeholder={t("ledger.answer.placeholder")}
            rows={2}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commit}
          />
          <span className="style-hint">{t("ledger.answer.hint")}</span>
        </>
      )}
    </span>
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
