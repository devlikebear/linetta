import { useEffect, useState } from "react";
import { ledger as ledgerApi } from "../lib/rpc";
import type { PendingSettlementItem } from "../lib/types";
import { useI18n } from "../lib/i18n";

interface Props {
  projectId: string;
  /** Bumped by the caller when an agent or another panel changed the work. */
  refreshKey?: number;
  onOpenNode?: (nodeId: string) => void;
}

/** "정산 대기 n화" above the ledger (longform Phase 6): the episodes written or
 *  changed since settlement was switched on whose changes the ledger does not
 *  hold yet. Nothing shows while settlement is off or nothing is waiting — the
 *  writer who never switched it on never sees it. */
export function SettlementPending({ projectId, refreshKey = 0, onOpenNode }: Readonly<Props>) {
  const { t } = useI18n();
  const [items, setItems] = useState<PendingSettlementItem[]>([]);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    ledgerApi
      .pendingSettlement(projectId)
      .then((res) => {
        if (!cancelled) setItems(res.enabled ? res.items : []);
      })
      .catch(() => {
        /* a nicety, like the suggestions badge: a failed fetch just hides it */
        if (!cancelled) setItems([]);
      });
    return () => { cancelled = true; };
  }, [projectId, refreshKey]);

  if (items.length === 0) return null;
  return (
    <section className="canon-settlement">
      <button
        type="button"
        className="canon-settlement-toggle"
        aria-expanded={open}
        title={t("canon.settlementPendingHint")}
        onClick={() => setOpen((v) => !v)}
      >
        {t("canon.settlementPending", { count: items.length })}
      </button>
      {open && (
        <>
          <p className="sd">{t("canon.settlementPendingHint")}</p>
          <ul className="canon-settlement-list">
            {items.map((item) => {
              const label = item.parent_label ? `${item.parent_label} · ${item.label}` : item.label;
              return (
                <li key={item.node_id}>
                  {onOpenNode ? (
                    <button type="button" onClick={() => onOpenNode(item.node_id)}>{label}</button>
                  ) : (
                    <span>{label}</span>
                  )}
                </li>
              );
            })}
          </ul>
        </>
      )}
    </section>
  );
}
