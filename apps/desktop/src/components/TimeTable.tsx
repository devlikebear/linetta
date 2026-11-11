import { useEffect, useMemo, useState } from "react";
import { ledger as ledgerApi } from "../lib/rpc";
import type { KnowsScene, StoryTime, TimeFact, TimeRow } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";

/** How long it has been since each event, scene by scene (longform Phase 8).
 *  Rows are the events an elapsed fact points at, and below them the ages
 *  written in years; columns are the scenes that recorded one. A conflict the
 *  engine found is marked as one. A big jump between scenes whose story date
 *  is not known is only shown: the prose may say the time passed. */

interface Props {
  projectId: string;
  /** Changes whenever the ledger was reloaded, so the table follows edits. */
  version?: unknown;
  onOpenNode?: (nodeId: string) => void;
}

export function TimeTable({ projectId, version, onOpenNode }: Readonly<Props>) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [data, setData] = useState<StoryTime | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    // Fetched only while unfolded: the section is closed most of the time.
    if (!open) return;
    let cancelled = false;
    ledgerApi.storyTime(projectId)
      .then((res) => { if (!cancelled) { setData(res); setError(null); } })
      .catch((err) => { if (!cancelled) setError(err); });
    return () => { cancelled = true; };
  }, [open, projectId, version, reload]);

  // Only the scenes something was recorded in: a long work has hundreds.
  const columns = useMemo(() => {
    if (!data) return [];
    const used = new Set<string>(Object.keys(data.clocks ?? {}));
    for (const row of [...data.events, ...data.ages]) {
      for (const f of row.facts) if (f.entry.planted_node_id) used.add(f.entry.planted_node_id);
    }
    return data.scenes.filter((s) => used.has(s.node_id));
  }, [data]);

  const empty = data != null && data.events.length === 0 && data.ages.length === 0;
  return (
    <details className="knows-table time-table" onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}>
      <summary>{t("time.title")}</summary>
      <p className="style-hint">{t("time.hint")}</p>
      {error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}
      <button type="button" className="btn ghost sm" onClick={() => setReload((n) => n + 1)}>{t("time.refresh")}</button>
      {empty && <p className="style-hint">{t("time.empty")}</p>}
      {data != null && !empty && (
        <div className="knows-scroll">
          <table>
            <thead>
              <tr>
                <th scope="col">{t("time.event")}</th>
                {columns.map((s) => (
                  <th key={s.node_id} scope="col" data-flashback={s.flashback ? "yes" : undefined}>
                    <button type="button" className="ledger-where" onClick={() => onOpenNode?.(s.node_id)}>{sceneLabel(s)}</button>
                    {s.flashback && <span className="time-flashback" title={t("time.flashbackHint")}>{t("time.flashback")}</span>}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {Object.keys(data.clocks ?? {}).length > 0 && (
                <tr className="knows-reader">
                  <th scope="row">{t("time.clock")}</th>
                  {columns.map((s) => {
                    const day = data.clocks[s.node_id];
                    return <td key={s.node_id}>{day == null ? "" : t("time.day", { n: fmt(day) })}</td>;
                  })}
                </tr>
              )}
              {data.events.map((row) => (
                <Row key={row.event?.id ?? row.subject} row={row} columns={columns} unit="days" />
              ))}
              {data.ages.length > 0 && (
                <tr className="time-ages">
                  <th scope="rowgroup" colSpan={columns.length + 1}>{t("time.age")}</th>
                </tr>
              )}
              {data.ages.map((row) => <Row key={`age-${row.subject}`} row={row} columns={columns} unit="years" />)}
            </tbody>
          </table>
        </div>
      )}
    </details>
  );
}

function Row({ row, columns, unit }: Readonly<{ row: TimeRow; columns: KnowsScene[]; unit: "days" | "years" }>) {
  const { t } = useI18n();
  const at = new Map<string, TimeFact[]>();
  for (const f of row.facts) {
    const id = f.entry.planted_node_id ?? "";
    at.set(id, [...(at.get(id) ?? []), f]);
  }
  const eventDay = row.event?.value_num;
  return (
    <tr>
      <th scope="row">
        {row.subject}
        {eventDay != null && <span className="ledger-where">{t("time.eventDay", { n: fmt(eventDay) })}</span>}
      </th>
      {columns.map((s) => {
        const facts = at.get(s.node_id);
        if (!facts) return <td key={s.node_id} data-time="none" />;
        return <Cell key={s.node_id} facts={facts} unit={unit} />;
      })}
    </tr>
  );
}

function Cell({ facts, unit }: Readonly<{ facts: TimeFact[]; unit: "days" | "years" }>) {
  const { t } = useI18n();
  const conflicts = facts.flatMap((f) => f.conflicts ?? []);
  const jump = facts.find((f) => (f.previous?.jump_days ?? 0) > 0)?.previous;
  const state = conflicts.length > 0 ? "conflict" : jump ? "jump" : facts.every((f) => f.entry.value_num == null) ? "uncompared" : "ok";
  const title = conflicts.length > 0
    ? conflicts.map((c) => c.reason).join("\n")
    : jump
      ? t("time.jumpTitle", { days: fmt(jump.jump_days ?? 0), scenes: String(jump.scenes_between ?? 0) })
      : state === "uncompared" ? t("time.uncompared") : undefined;
  return (
    <td data-time={state} title={title}>
      {facts.map((f) => (
        <span key={f.entry.id} className="time-value">
          {f.entry.body}
          {f.entry.value_num != null && (
            <span className="time-num">{t(unit === "years" ? "time.years" : "time.day", { n: fmt(f.entry.value_num) })}</span>
          )}
        </span>
      ))}
      {conflicts.map((c, i) => <span key={i} className="time-note">{c.reason}</span>)}
      {conflicts.length === 0 && jump && (
        <span className="time-note">{t("time.jump", { days: fmt(jump.jump_days ?? 0) })}</span>
      )}
    </td>
  );
}

function fmt(n: number): string {
  return Number.isInteger(n) ? n.toLocaleString() : n.toFixed(1);
}

function sceneLabel(s: KnowsScene): string {
  return s.parent_label ? `${s.parent_label} · ${s.label}` : s.label;
}
