import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { useI18n } from "../lib/i18n";
import { vaultApi } from "../lib/rpc";
import { vaultNewVersions } from "../lib/vault";
import type { VaultItemView } from "../lib/types";

/** The library's line for works with a newer version on the server. It only
 *  says so; taking one replaces a manuscript, and that stays the writer's call
 *  in the work's own dialog. A failure says nothing here: this is a courtesy on
 *  the way past, and the dialog and the settings pane explain problems. */
export function VaultNotice({ available }: { available: boolean }) {
  const { t } = useI18n();
  const [items, setItems] = useState<VaultItemView[]>([]);

  useEffect(() => {
    if (!available) return undefined;
    let cancelled = false;
    vaultApi
      .status()
      .then((overview) => {
        if (!cancelled) setItems(vaultNewVersions(overview));
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [available]);

  if (items.length === 0) return null;
  return (
    <p className="lib-vault-notice" data-testid="vault-notice">
      {t("vault.newVersions")}{" "}
      {items.map((item, i) => (
        <span key={item.item_id}>
          {i > 0 && ", "}
          <Link to={`/workspace/${item.project_id}`}>{item.title}</Link>
        </span>
      ))}
    </p>
  );
}
