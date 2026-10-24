import { useEffect, useState } from "react";
import { direction as directionApi } from "../lib/rpc";
import type { FocusState } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";
import "./DirectionSection.css";

/** The writer's own direction (story-assist Phase 1): a work's current focus
 *  and one scene's intent. Neither is written by an agent — there is no MCP
 *  tool for either — so this is the only place either is ever set.
 *
 *  Mirrors the engine's own limits (engine/internal/direction/direction.go)
 *  so the writer sees a reason before they save, not only after the engine
 *  refuses it.
 */
const MAX_FOCUS_RUNES = 500;
const MAX_GOAL_RUNES = 200;
const MAX_LIST_ITEMS = 5;
const MAX_ITEM_RUNES = 120;

interface Props {
  projectId: string;
  /** The scene open in the editor. The scene-intent block is hidden without one. */
  nodeId?: string;
  /** Controlled collapse state. Defaults to open (uncontrolled) without it. */
  collapsed?: boolean;
  /** Fires with the new collapsed state when the writer toggles the section. */
  onToggleCollapsed?: (collapsed: boolean) => void;
}

/** One item per line in the box; the engine trims, drops blanks and de-duplicates. */
function toItems(text: string): string[] {
  return text.split("\n").map((line) => line.trim()).filter((line) => line !== "");
}

function runeLength(s: string): number {
  return [...s].length;
}

export function DirectionSection({ projectId, nodeId, collapsed, onToggleCollapsed }: Readonly<Props>) {
  const { t } = useI18n();
  const [focus, setFocus] = useState<FocusState | null>(null);
  const [focusBody, setFocusBody] = useState("");
  const [goal, setGoal] = useState("");
  const [keepText, setKeepText] = useState("");
  const [avoidText, setAvoidText] = useState("");
  const [busy, setBusy] = useState<"focus" | "keepAsIs" | "intent" | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let cancelled = false;
    directionApi.getFocus(projectId).then((state) => {
      if (cancelled) return;
      setFocus(state);
      setFocusBody(state.present ? state.body : "");
    }).catch((e) => { if (!cancelled) setError(e); });
    return () => { cancelled = true; };
  }, [projectId]);

  useEffect(() => {
    if (!nodeId) {
      setGoal("");
      setKeepText("");
      setAvoidText("");
      return;
    }
    let cancelled = false;
    directionApi.getSceneIntent(nodeId).then((state) => {
      if (cancelled) return;
      setGoal(state.intent.goal ?? "");
      setKeepText((state.intent.keep ?? []).join("\n"));
      setAvoidText((state.intent.avoid ?? []).join("\n"));
    }).catch((e) => { if (!cancelled) setError(e); });
    return () => { cancelled = true; };
  }, [nodeId]);

  const run = async (kind: NonNullable<typeof busy>, work: () => Promise<void>) => {
    setBusy(kind);
    setError(null);
    try {
      await work();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  const saveFocus = () => run("focus", async () => {
    const state = await directionApi.setFocus(projectId, focusBody);
    setFocus(state);
    setFocusBody(state.present ? state.body : "");
  });

  // "그대로 유지": re-sets the same body, which re-anchors to the episode
  // that is newest right now (direction.Repo.SetFocus's rule) and so clears
  // the staleness the writer just confirmed still holds.
  const keepAsIs = () => run("keepAsIs", async () => {
    if (!focus?.present) return;
    setFocus(await directionApi.setFocus(projectId, focus.body));
  });

  const saveIntent = () => run("intent", async () => {
    if (!nodeId) return;
    const saved = await directionApi.setSceneIntent(nodeId, goal, toItems(keepText), toItems(avoidText));
    setGoal(saved.goal ?? "");
    setKeepText((saved.keep ?? []).join("\n"));
    setAvoidText((saved.avoid ?? []).join("\n"));
  });

  const working = busy != null;
  const focusOverLimit = runeLength(focusBody) > MAX_FOCUS_RUNES;

  const goalOverLimit = runeLength(goal) > MAX_GOAL_RUNES;
  const keepItems = toItems(keepText);
  const avoidItems = toItems(avoidText);
  const tooManyItems = keepItems.length > MAX_LIST_ITEMS || avoidItems.length > MAX_LIST_ITEMS;
  const itemTooLong = [...keepItems, ...avoidItems].some((item) => runeLength(item) > MAX_ITEM_RUNES);
  const intentOverLimit = goalOverLimit || tooManyItems || itemTooLong;
  const intentLimitReason = goalOverLimit
    ? t("direction.limit.goal")
    : tooManyItems
      ? t("direction.limit.items")
      : itemTooLong
        ? t("direction.limit.item")
        : null;

  return (
    <details
      className="sec es-field direction-section"
      open={collapsed === undefined ? true : !collapsed}
      onToggle={(e) => onToggleCollapsed?.(!(e.currentTarget as HTMLDetailsElement).open)}
    >
      <summary>{t("direction.title")}</summary>
      {error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}

      <div className="direction-block">
        <label>{t("direction.focus.title")}
          <textarea
            rows={3}
            value={focusBody}
            disabled={working}
            placeholder={t("direction.focus.placeholder")}
            onChange={(e) => setFocusBody(e.target.value)}
          />
        </label>
        <p className="direction-hint">{runeLength(focusBody)}/{MAX_FOCUS_RUNES}</p>
        {focusOverLimit && <p role="alert" className="direction-hint">{t("direction.limit.focus")}</p>}
        {focus?.present && focus.stale && (
          <div className="direction-stale" role="status">
            <span>
              {focus.episodes_since === -1
                ? t("direction.focus.anchorMissing")
                : t("direction.focus.stale", { episodes: focus.episodes_since })}
            </span>
            <button type="button" className="btn sm" disabled={working} onClick={() => void keepAsIs()}>
              {t("direction.focus.keep")}
            </button>
          </div>
        )}
        <div className="direction-actions">
          <button type="button" className="btn accent sm" disabled={working || focusOverLimit} onClick={() => void saveFocus()}>
            {busy === "focus" ? t("common.saving") : t("common.save")}
          </button>
        </div>
      </div>

      {nodeId && (
        <div className="direction-block">
          <h5>{t("direction.scene.title")}</h5>
          <label>{t("direction.scene.goal")}
            <input
              type="text"
              value={goal}
              disabled={working}
              placeholder={t("direction.scene.goalPlaceholder")}
              onChange={(e) => setGoal(e.target.value)}
            />
          </label>
          <label>{t("direction.scene.keep")}
            <textarea
              rows={3}
              value={keepText}
              disabled={working}
              placeholder={t("direction.scene.keepPlaceholder")}
              onChange={(e) => setKeepText(e.target.value)}
            />
          </label>
          <label>{t("direction.scene.avoid")}
            <textarea
              rows={3}
              value={avoidText}
              disabled={working}
              placeholder={t("direction.scene.avoidPlaceholder")}
              onChange={(e) => setAvoidText(e.target.value)}
            />
          </label>
          {intentOverLimit && <p role="alert" className="direction-hint">{intentLimitReason}</p>}
          <div className="direction-actions">
            <button type="button" className="btn accent sm" disabled={working || intentOverLimit} onClick={() => void saveIntent()}>
              {busy === "intent" ? t("common.saving") : t("common.save")}
            </button>
          </div>
        </div>
      )}
    </details>
  );
}
