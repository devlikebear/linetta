import { useCallback, useEffect, useRef, useState } from "react";

import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import { syncApi, vaultApi } from "../../lib/rpc";
import { reasonMessage, rpcErrorMessage } from "../../lib/rpcMessage";
import { formatVaultBytes } from "../../lib/vault";
import type { SyncTargets, SyncTargetView } from "../../lib/types";
import { SyncAutoPanel } from "./SyncAutoPanel";
import { VaultItemList } from "./VaultItemList";

const TARGET_KEYS: Record<string, MessageKey> = {
  works: "settings.sync.target.works",
  git: "settings.sync.target.git",
  folder: "settings.sync.target.folder",
};

/** Settings → Sync: which target is the two-way one, which only receive, and
 *  what travels. The recovery key belongs to the library, not to a target, so a
 *  writer on GitHub or a folder alone makes it here and needs no account. */
export function SyncOverview() {
  const { t } = useI18n();
  const [view, setView] = useState<SyncTargets | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [pendingGit, setPendingGit] = useState(false);
  const [restored, setRestored] = useState(false);
  const [consented, setConsented] = useState(false);
  const [keyText, setKeyText] = useState("");
  const [newKey, setNewKey] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const live = useRef(true);

  const load = useCallback(async () => {
    try {
      const next = await syncApi.targets();
      if (live.current) setView(next);
    } catch (e) {
      if (live.current) setError(e);
    }
  }, []);

  useEffect(() => {
    live.current = true;
    void load();
    return () => {
      live.current = false;
    };
  }, [load]);

  const guard = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      if (live.current) setError(e);
    } finally {
      if (live.current) setBusy(false);
    }
  };

  if (!view) {
    return error ? <p className="sd" role="alert" data-testid="sync-targets-error">{rpcErrorMessage(error, t)}</p> : null;
  }

  const setTargets = (primary: string, mirrors: string[]) =>
    guard(async () => {
      setView(await syncApi.setTargets(primary, mirrors));
      setPendingGit(false);
    });

  const choosePrimary = (id: string) => {
    if (id === "git" && view.primary !== "git") {
      setPendingGit(true);
      return;
    }
    void setTargets(id, view.mirrors.filter((m) => m !== id));
  };

  const toggleMirror = (id: string, on: boolean) => {
    const mirrors = on ? [...view.mirrors, id] : view.mirrors.filter((m) => m !== id);
    void setTargets(view.primary, mirrors);
  };

  const restore = (id: string) =>
    guard(async () => {
      await syncApi.restore(id);
      setRestored(true);
    });

  const keyTarget = view.targets.find((target) => target.id !== "works" && target.configured);
  const makeKey = () =>
    guard(async () => {
      const result = await vaultApi.setup({ consented, target: keyTarget?.id });
      if (result.recovery_key) setNewKey(result.recovery_key);
      else await load();
    });
  const storeKey = (text: string) =>
    guard(async () => {
      await vaultApi.setup({ consented, recovery_key: text, target: keyTarget?.id });
      setNewKey(null);
      setKeyText("");
      await load();
    });

  const line = (target: SyncTargetView) => {
    if (target.reason) return reasonMessage(target.reason, t);
    if (!target.configured) return t("settings.sync.target.notSet");
    return target.bytes > 0 ? formatVaultBytes(target.bytes) : "";
  };

  return (
    <section className="settings-section" data-testid="sync-overview">
      <h3>{t("settings.sync.targetsTitle")}</h3>
      <p className="sd">{t("settings.sync.targetsNote")}</p>
      {!view.key_stored && keyTarget && (
        <div className="modal-field vault-step" data-testid="sync-key-step">
          <p className="sd">{t("settings.sync.keyNote")}</p>
          {newKey ? (
            <>
              <code className="vault-key" data-testid="sync-new-key">{newKey}</code>
              <p className="sd">{t("settings.vault.keyWarning")}</p>
              <label className="modal-field modal-check">
                <input type="checkbox" checked={saved} onChange={(e) => setSaved(e.target.checked)} data-testid="sync-key-saved" />
                {t("settings.vault.savedCheck")}
              </label>
              <div className="field-actions">
                <button type="button" className="btn sm" disabled={busy || !saved} onClick={() => void storeKey(newKey)} data-testid="sync-key-confirm">
                  {t("settings.vault.saveKey")}
                </button>
              </div>
            </>
          ) : (
            <>
              <label className="modal-field modal-check">
                <input type="checkbox" checked={consented} disabled={busy} onChange={(e) => setConsented(e.target.checked)} data-testid="sync-key-consent" />
                {t("settings.sync.keyConsent")}
              </label>
              <div className="field-actions">
                <button type="button" className="btn sm" disabled={busy || !consented} onClick={() => void makeKey()} data-testid="sync-key-make">
                  {t("settings.vault.makeKey")}
                </button>
              </div>
              <div className="modal-field">
                <label htmlFor="sync-key-input">{t("settings.vault.keyLabel")}</label>
                <input
                  id="sync-key-input"
                  type="text"
                  autoComplete="off"
                  spellCheck={false}
                  value={keyText}
                  disabled={busy}
                  onChange={(e) => setKeyText(e.target.value)}
                  data-testid="sync-key-input"
                />
              </div>
              <div className="field-actions">
                <button
                  type="button"
                  className="btn ghost sm"
                  disabled={busy || !consented || keyText.trim() === ""}
                  onClick={() => void storeKey(keyText.trim())}
                  data-testid="sync-key-use"
                >
                  {t("settings.vault.useKey")}
                </button>
              </div>
            </>
          )}
        </div>
      )}
      <ul className="vault-items" data-testid="sync-targets">
        {view.targets.map((target) => {
          const primary = view.primary === target.id;
          const usable = target.configured && (target.id === "works" || view.key_stored);
          return (
            <li key={target.id}>
              <span className="vault-item-title">{t(TARGET_KEYS[target.id] ?? "settings.sync.target.works")}</span>
              <span className="sd" data-testid={`sync-target-line-${target.id}`}>{line(target)}</span>
              <span className="vault-item-actions">
                <label className="set-toggle">
                  <input
                    type="radio"
                    name="sync-primary"
                    checked={primary}
                    disabled={busy || !usable || !target.can_primary}
                    onChange={() => choosePrimary(target.id)}
                    data-testid={`sync-primary-${target.id}`}
                  />
                  {t("settings.sync.primary")}
                </label>
                <label className="set-toggle">
                  <input
                    type="checkbox"
                    checked={primary || view.mirrors.includes(target.id)}
                    disabled={busy || primary || !usable}
                    onChange={(e) => toggleMirror(target.id, e.target.checked)}
                    data-testid={`sync-mirror-${target.id}`}
                  />
                  {t("settings.sync.mirror")}
                </label>
                {usable && !primary && (
                  <button type="button" className="btn ghost sm" disabled={busy} onClick={() => void restore(target.id)} data-testid={`sync-restore-${target.id}`}>
                    {t("settings.sync.restore")}
                  </button>
                )}
              </span>
              {!target.can_primary && <span className="sd" data-testid={`sync-mirror-only-${target.id}`}>{t("settings.sync.mirrorOnly")}</span>}
            </li>
          );
        })}
      </ul>
      {pendingGit && (
        <div className="vault-delete-confirm" data-testid="sync-git-confirm">
          <p className="sd vault-warning" role="alert">{t("settings.sync.gitWarning")}</p>
          <div className="field-actions">
            <button type="button" className="btn sm" disabled={busy} onClick={() => void setTargets("git", view.mirrors.filter((m) => m !== "git"))} data-testid="sync-git-yes">
              {t("settings.sync.gitYes")}
            </button>
            <button type="button" className="btn ghost sm" disabled={busy} onClick={() => setPendingGit(false)} data-testid="sync-git-no">
              {t("common.cancel")}
            </button>
          </div>
        </div>
      )}
      {busy && <p className="sd" role="status" data-testid="sync-targets-busy">{t("settings.sync.running")}</p>}
      {restored && <p className="sd" role="status" data-testid="sync-restored">{t("settings.sync.restored")}</p>}
      {error ? <p className="sd" role="alert" data-testid="sync-targets-error">{rpcErrorMessage(error, t)}</p> : null}
      <SyncAutoPanel editable />
      <VaultItemList editable />
    </section>
  );
}
