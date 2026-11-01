import { useEffect, useRef, useState } from "react";

import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { vaultApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import { forgetImagesOmitted, formatVaultBytes } from "../../lib/vault";
import type { VaultItemCategory } from "../../lib/types";

const CATEGORY_KEYS: Record<string, MessageKey> = {
  "": "settings.vault.category.required",
  images: "settings.vault.category.images",
  history: "settings.vault.category.history",
  work_agent: "settings.vault.category.workAgent",
  global_skills: "settings.vault.category.globalSkills",
  writer_profile: "settings.vault.category.writerProfile",
};

/** What travels, with what each line weighs on this device: pictures are most of
 *  a library, and the choice is easier made looking at that. The choice belongs
 *  to the account, so one device turning a line off turns it off on all of them. */
export function VaultItemList({ editable }: { editable: boolean }) {
  const { t } = useI18n();
  const [categories, setCategories] = useState<VaultItemCategory[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const live = useRef(true);

  useEffect(() => {
    live.current = true;
    vaultApi
      .items()
      .then((next) => {
        if (live.current) setCategories(next.categories);
      })
      .catch((e: unknown) => {
        if (live.current) setError(e);
      });
    return () => {
      live.current = false;
    };
  }, []);

  if (!categories) {
    return error ? <p className="sd" role="alert" data-testid="vault-items-error">{rpcErrorMessage(error, t)}</p> : null;
  }

  const toggle = async (id: string, enabled: boolean) => {
    const omit = categories.filter((c) => !c.required && (c.id === id ? !enabled : !c.enabled)).map((c) => c.id);
    setBusy(true);
    setError(null);
    try {
      const next = await vaultApi.setItems(omit);
      forgetImagesOmitted();
      if (live.current) setCategories(next.categories);
    } catch (e) {
      if (live.current) setError(e);
    } finally {
      if (live.current) setBusy(false);
    }
  };

  return (
    <section className="settings-section" data-testid="vault-categories">
      <h3>{t("settings.vault.whatTravels")}</h3>
      <p className="sd">{t("settings.vault.whatTravelsNote")}</p>
      <ul className="vault-categories">
        {categories.map((category) => (
          <li key={category.id}>
            <label className="set-toggle">
              <input
                type="checkbox"
                checked={category.enabled}
                disabled={category.required || !editable || busy}
                onChange={(e) => void toggle(category.id, e.target.checked)}
                data-testid={`vault-category-${category.id || "required"}`}
              />
              {t(CATEGORY_KEYS[category.id] ?? "settings.vault.category.required")}
            </label>
            <span className="sd" data-testid={`vault-category-size-${category.id || "required"}`}>
              {formatVaultBytes(category.bytes)}
            </span>
          </li>
        ))}
      </ul>
      {error ? <p className="sd" role="alert" data-testid="vault-items-error">{rpcErrorMessage(error, t)}</p> : null}
    </section>
  );
}
