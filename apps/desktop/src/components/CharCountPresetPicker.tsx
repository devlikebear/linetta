import { useEffect, useState } from "react";
import type { CharCountPreset, Project } from "../lib/types";
import { projects as projectsApi, settings as settingsApi } from "../lib/rpc";
import { localeForLanguage, useI18n } from "../lib/i18n";
import { charCountBasisLabelKey } from "../lib/charCount";
import {
  BUILTIN_CHAR_COUNT_PRESETS,
  MAX_CHAR_COUNT_PRESET_NAME,
  lengthRuleOf,
  matchingPresetKey,
  newCharCountPreset,
  ruleForPresetKey,
  type LengthRule,
} from "../lib/charCountPresets";

interface Props {
  project: Project;
  onProjectChanged?: (project: Project) => void;
}

/** Applies a length preset — a count basis and an episode target together —
 *  to the open work, and saves the work's current rule as the writer's own
 *  (#239). Saved presets live in the app settings, shared across works. */
export function CharCountPresetPicker({ project, onProjectChanged }: Readonly<Props>) {
  const { language, t } = useI18n();
  const [userPresets, setUserPresets] = useState<CharCountPreset[]>([]);
  const [preferred, setPreferred] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [naming, setNaming] = useState(false);
  const [draftName, setDraftName] = useState("");
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    settingsApi
      .get()
      .then((cfg) => {
        if (alive) setUserPresets(cfg.char_count_presets ?? []);
      })
      .catch(() => {
        /* the built-in presets still work without the saved ones */
      });
    return () => {
      alive = false;
    };
  }, []);

  const rule = lengthRuleOf(project);
  const selected = matchingPresetKey(rule, userPresets, preferred);
  const selectedUserId = selected.startsWith("user:") ? selected.slice("user:".length) : null;

  const optionLabel = (name: string, r: LengthRule) =>
    t("charPreset.option", {
      name,
      basis: t(charCountBasisLabelKey(r.char_count_basis)),
      count: r.episode_char_target.toLocaleString(localeForLanguage(language)),
    });

  /** Runs work with the picker busy; false if it failed. */
  const run = async (work: () => Promise<void>): Promise<boolean> => {
    setBusy(true);
    setFailed(false);
    try {
      await work();
      return true;
    } catch {
      setFailed(true);
      return false;
    } finally {
      setBusy(false);
    }
  };

  const apply = (key: string) => {
    const next = ruleForPresetKey(key, userPresets);
    if (!next) return;
    setPreferred(key);
    return run(async () => {
      const updated = await projectsApi.update({
        id: project.id,
        char_count_basis: next.char_count_basis,
        episode_char_target: next.episode_char_target,
      });
      onProjectChanged?.(updated);
    });
  };

  const storePresets = (next: CharCountPreset[]) =>
    run(async () => {
      const cfg = await settingsApi.set({ char_count_presets: next });
      setUserPresets(cfg.char_count_presets ?? []);
    });

  const saveCurrent = async () => {
    const name = draftName.trim();
    if (!name) return;
    const preset = newCharCountPreset(name, rule);
    if (!(await storePresets([...userPresets, preset]))) return;
    setPreferred(`user:${preset.id}`);
    setNaming(false);
    setDraftName("");
  };

  const deleteSelected = () => {
    if (!selectedUserId) return;
    return storePresets(userPresets.filter((p) => p.id !== selectedUserId));
  };

  return (
    <div className="char-preset" data-testid="char-count-preset">
      <label className="char-basis-row">
        <span>{t("charPreset.label")}</span>
        <select
          value={selected}
          disabled={busy}
          onChange={(e) => void apply(e.target.value)}
          data-testid="char-count-preset-select"
        >
          <option value="" disabled>{t("charPreset.custom")}</option>
          <optgroup label={t("charPreset.builtinGroup")}>
            {BUILTIN_CHAR_COUNT_PRESETS.map((p) => (
              <option key={p.id} value={`builtin:${p.id}`}>{optionLabel(t(p.nameKey), p)}</option>
            ))}
          </optgroup>
          {userPresets.length > 0 && (
            <optgroup label={t("charPreset.userGroup")}>
              {userPresets.map((p) => (
                <option key={p.id} value={`user:${p.id}`}>{optionLabel(p.name, p)}</option>
              ))}
            </optgroup>
          )}
        </select>
      </label>
      {naming ? (
        <form
          className="char-preset-name"
          onSubmit={(e) => {
            e.preventDefault();
            void saveCurrent();
          }}
        >
          <input
            value={draftName}
            maxLength={MAX_CHAR_COUNT_PRESET_NAME}
            placeholder={t("charPreset.namePlaceholder")}
            aria-label={t("charPreset.namePlaceholder")}
            onChange={(e) => setDraftName(e.target.value)}
            autoFocus
            data-testid="char-count-preset-name"
          />
          <button type="submit" className="chip" disabled={busy || !draftName.trim()}>
            {t("charPreset.saveConfirm")}
          </button>
          <button
            type="button"
            className="chip"
            onClick={() => {
              setNaming(false);
              setDraftName("");
            }}
          >
            {t("charPreset.cancel")}
          </button>
        </form>
      ) : (
        <div className="char-preset-actions">
          <button type="button" className="chip" disabled={busy} onClick={() => setNaming(true)} data-testid="char-count-preset-save">
            {t("charPreset.save")}
          </button>
          {selectedUserId && (
            <button type="button" className="chip" disabled={busy} onClick={() => void deleteSelected()} data-testid="char-count-preset-delete">
              {t("charPreset.delete")}
            </button>
          )}
        </div>
      )}
      {failed && <p className="char-preset-error" role="alert">{t("charPreset.error")}</p>}
    </div>
  );
}
