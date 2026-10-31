import { useCallback, useEffect, useRef, useState } from "react";

import { localeForLanguage, useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { openExternalUrl, publishApi, vaultApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { AppLanguage, VaultAccount, VaultOverview } from "../../lib/types";

/** Settings → Private Vault: everything a device needs before a work can be
 *  uploaded. Which step shows comes from the site and the secret store alone,
 *  so a store that is locked or a site that is down leaves an error and a retry
 *  here, never the "make a key" step: a second key over the first would strand
 *  everything sealed under it.
 *
 *  The recovery key is the one thing here that cannot be read back, so a new
 *  key lives only in this component until the writer says they saved it. The
 *  engine stores nothing before that; leaving costs a new key, not a key nobody
 *  kept.
 */

export type VaultSectionProps = {
  /** False on builds without the vault (iOS): the pane renders nothing. */
  available: boolean;
  /** The vault shares the publish connection, which is set up in that pane. */
  onOpenPublish: () => void;
};

const NO_ACCESS_KEYS: Record<string, MessageKey> = {
  pending: "settings.vault.pending",
  denied: "settings.vault.denied",
  revoked: "settings.vault.revoked",
};

function formatBytes(bytes: number): string {
  if (bytes < 1 << 20) return `${(bytes / (1 << 10)).toFixed(0)} KiB`;
  return `${(bytes / (1 << 20)).toFixed(1)} MiB`;
}

/** PocketBase writes a space where ISO has a T, which not every parser takes. */
function formatDate(value: string, language: AppLanguage): string {
  const date = new Date(value.replace(" ", "T"));
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString(localeForLanguage(language));
}

function needsKey(overview: VaultOverview, account: VaultAccount): boolean {
  if (overview.key_mismatch) return true;
  if (overview.key_stored) return false;
  // Nothing to read and no way to write: a key would do nothing.
  return !(account.mode === "read_only" && overview.items.length === 0);
}

export function VaultSection({ available, onOpenPublish }: VaultSectionProps) {
  const { t, language } = useI18n();
  const [overview, setOverview] = useState<VaultOverview | null>(null);
  const [site, setSite] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [consented, setConsented] = useState(false);
  const [keyText, setKeyText] = useState("");
  const [newKey, setNewKey] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [copied, setCopied] = useState<"copied" | "failed" | null>(null);
  // A slow answer must not paint over the one that came after it.
  const loadRef = useRef(0);

  const load = useCallback(async () => {
    const gen = ++loadRef.current;
    setLoading(true);
    setError(null);
    try {
      const [next, connection] = await Promise.all([vaultApi.status(), publishApi.connectStatus()]);
      if (gen !== loadRef.current) return;
      setOverview(next);
      setSite(connection.site_url ?? "");
    } catch (e) {
      if (gen !== loadRef.current) return;
      setError(e);
    } finally {
      if (gen === loadRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!available) return;
    void load();
    return () => {
      loadRef.current += 1;
    };
  }, [available, load]);

  if (!available) return null;

  const guard = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const makeKey = () =>
    guard(async () => {
      const result = await vaultApi.setup({ consented });
      if (result.recovery_key) setNewKey(result.recovery_key);
      else await load();
    });

  const enterKey = (replace: boolean) =>
    guard(async () => {
      await vaultApi.setup({ consented, recovery_key: keyText.trim(), replace });
      setKeyText("");
      await load();
    });

  const confirmSaved = () =>
    guard(async () => {
      await vaultApi.setup({ consented, recovery_key: newKey ?? "" });
      setNewKey(null);
      setSaved(false);
      setCopied(null);
      await load();
    });

  const copyKey = async () => {
    try {
      if (!navigator.clipboard?.writeText) throw new Error("clipboard unavailable");
      await navigator.clipboard.writeText(newKey ?? "");
      setCopied("copied");
    } catch {
      setCopied("failed");
    }
  };

  const siteButton = site && (
    <button
      type="button"
      className="btn ghost sm"
      disabled={busy}
      onClick={() => void guard(() => openExternalUrl(`${site}/me/vault`))}
      data-testid="vault-open-site"
    >
      {t("settings.vault.openSite")}
    </button>
  );

  const refreshButton = (
    <button
      type="button"
      className="btn ghost sm"
      disabled={busy || loading}
      onClick={() => void load()}
      data-testid="vault-refresh"
    >
      {t("settings.vault.refresh")}
    </button>
  );

  const renderBody = () => {
    if (newKey) {
      return (
        <div className="modal-field vault-step" data-testid="vault-new-key">
          <p className="sd vault-warning" role="alert">{t("settings.vault.keyWarning")}</p>
          <p className="vault-key" aria-label={t("settings.vault.keyLabel")} data-testid="vault-recovery-key">
            {newKey}
          </p>
          <div className="field-actions">
            <button type="button" className="btn ghost sm" onClick={() => void copyKey()} data-testid="vault-copy-key">
              {t("settings.vault.copyKey")}
            </button>
          </div>
          {copied && (
            <p className="sd" role="status" data-testid="vault-copy-result">
              {t(copied === "copied" ? "settings.vault.copied" : "settings.vault.copyFailed")}
            </p>
          )}
          <label className="modal-field modal-check">
            <input
              type="checkbox"
              checked={saved}
              disabled={busy}
              onChange={(e) => setSaved(e.target.checked)}
              data-testid="vault-saved-check"
            />
            {t("settings.vault.savedCheck")}
          </label>
          <div className="field-actions">
            <button
              type="button"
              className="btn sm"
              disabled={busy || !saved}
              onClick={() => void confirmSaved()}
              data-testid="vault-confirm-saved"
            >
              {t("settings.vault.saveKey")}
            </button>
          </div>
        </div>
      );
    }

    if (!overview) return null;

    if (!overview.connected) {
      return (
        <div className="modal-field" data-testid="vault-not-connected">
          <p className="sd">{t("settings.vault.connectFirst")}</p>
          <div className="field-actions">
            <button type="button" className="btn sm" onClick={onOpenPublish} data-testid="vault-open-publish">
              {t("settings.vault.openPublish")}
            </button>
          </div>
        </div>
      );
    }

    const account = overview.account;
    if (!account) return null;

    if (account.mode === "none") {
      return (
        <div className="modal-field" data-testid="vault-no-access">
          <p className="sd" data-testid="vault-no-access-note">
            {t(NO_ACCESS_KEYS[account.status] ?? "settings.vault.request")}
          </p>
          <div className="field-actions">
            {siteButton}
            {refreshButton}
          </div>
        </div>
      );
    }

    if (needsKey(overview, account)) {
      const hasItems = overview.items.length > 0;
      const canMake = !overview.key_stored && !hasItems;
      const note = overview.key_mismatch
        ? "settings.vault.mismatch"
        : hasItems
          ? "settings.vault.enterKeyNeeded"
          : "settings.vault.chooseKey";
      return (
        <div className="modal-field vault-step" data-testid="vault-key-step">
          <p
            className={overview.key_mismatch ? "sd vault-warning" : "sd"}
            role={overview.key_mismatch ? "alert" : undefined}
            data-testid="vault-key-note"
          >
            {t(note)}
          </p>
          <p className="sd" data-testid="vault-disclosure">{t("settings.vault.disclosure")}</p>
          <label className="modal-field modal-check">
            <input
              type="checkbox"
              checked={consented}
              disabled={busy}
              onChange={(e) => setConsented(e.target.checked)}
              data-testid="vault-consent"
            />
            {t("settings.vault.consent")}
          </label>
          {canMake && (
            <div className="field-actions">
              <button
                type="button"
                className="btn sm"
                disabled={busy || !consented}
                onClick={() => void makeKey()}
                data-testid="vault-make-key"
              >
                {t("settings.vault.makeKey")}
              </button>
            </div>
          )}
          <div className="modal-field">
            <label htmlFor="vault-key-input">{t("settings.vault.keyLabel")}</label>
            <input
              id="vault-key-input"
              type="text"
              autoComplete="off"
              autoCapitalize="characters"
              spellCheck={false}
              value={keyText}
              disabled={busy}
              onChange={(e) => setKeyText(e.target.value)}
              data-testid="vault-key-input"
            />
          </div>
          <div className="field-actions">
            <button
              type="button"
              className={canMake ? "btn ghost sm" : "btn sm"}
              disabled={busy || !consented || keyText.trim() === ""}
              onClick={() => void enterKey(overview.key_mismatch)}
              data-testid="vault-use-key"
            >
              {t("settings.vault.useKey")}
            </button>
          </div>
        </div>
      );
    }

    const { bytes, quota_bytes: quota } = account.usage;
    const percent = quota > 0 ? Math.min(100, Math.round((bytes / quota) * 100)) : 0;
    return (
      <div data-testid="vault-ready">
        {account.mode === "full" && <p className="sd">{t("settings.vault.ready")}</p>}
        {account.mode === "terms_outdated" && (
          <p className="sd" role="alert" data-testid="vault-terms-outdated">{t("settings.vault.termsOutdated")}</p>
        )}
        {account.mode === "read_only" && (
          <p className="sd" role="alert" data-testid="vault-read-only">
            {t("settings.vault.readOnly", { date: formatDate(account.retrieve_until, language) })}
          </p>
        )}
        <div className="vault-usage">
          <p className="sd vault-usage-label">
            <span>{t("settings.vault.usage")}</span>
            <span data-testid="vault-usage">{formatBytes(bytes)} / {formatBytes(quota)}</span>
          </p>
          <div
            className="vault-usage-bar"
            role="progressbar"
            aria-label={t("settings.vault.usage")}
            aria-valuenow={percent}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <div style={{ width: `${percent}%` }} />
          </div>
        </div>
        <h4>{t("settings.vault.items")}</h4>
        {overview.items.length === 0 ? (
          <p className="sd">{t("settings.vault.empty")}</p>
        ) : (
          <ul className="vault-items" data-testid="vault-items">
            {overview.items.map((item) => (
              <li key={item.item_id}>
                <span className="vault-item-title">{item.title || t("settings.vault.lockedItem")}</span>
                <span className="sd">
                  {formatBytes(item.bytes)} · {formatDate(item.updated, language)}
                  {!item.on_this_device && ` · ${t("settings.vault.notOnDevice")}`}
                </span>
              </li>
            ))}
          </ul>
        )}
        <p className="sd">{t("settings.vault.closeNote")}</p>
        <div className="field-actions">
          {siteButton}
          {refreshButton}
        </div>
        <p className="sd">{t("settings.vault.storage")}</p>
      </div>
    );
  };

  return (
    <section className="settings-section" id="vault-settings" data-testid="vault-section">
      <h3>{t("settings.vault.title")}</h3>
      <p className="sd">{t("settings.vault.description")}</p>
      {renderBody()}
      {error ? (
        <>
          <p className="sd" role="alert" data-testid="vault-error">
            {rpcErrorMessage(error, t)}
          </p>
          {!overview && !newKey && <div className="field-actions">{refreshButton}</div>}
        </>
      ) : null}
    </section>
  );
}
