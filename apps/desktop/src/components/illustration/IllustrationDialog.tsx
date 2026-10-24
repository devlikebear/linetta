import { useEffect, useMemo, useState } from "react";

import { useI18n } from "../../lib/i18n";
import type { MessageKey } from "../../lib/i18n";
import {
  entities as entitiesApi,
  illustrations as illustrationsApi,
  imageProviders as imageProvidersApi,
  mentions as mentionsApi,
} from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type {
  Entity,
  IllustrationAspect,
  IllustrationComposition,
  IllustrationFormat,
  IllustrationMeta,
  ImageProviderStatus,
} from "../../lib/types";
import { useIllustrationImage } from "./useIllustrationImage";
import "./Illustration.css";

interface Props {
  projectId: string;
  /** The scene being illustrated, if any: its mentioned characters are
   *  preselected and the picture remembers where it came from. */
  sourceNodeId?: string;
  initialScene?: string;
  onClose: () => void;
  onSaved?: (meta: IllustrationMeta) => void;
  onOpenSettings: () => void;
}

const ASPECTS: IllustrationAspect[] = ["portrait", "square", "landscape"];

/* Draw one illustration: describe the scene, pick who is in it, read the
 * prompt that will be sent (and edit it), then generate. Nothing reaches a
 * provider before the Generate button, and what is sent is exactly the text
 * in the prompt box. */
export function IllustrationDialog({ projectId, sourceNodeId, initialScene, onClose, onSaved, onOpenSettings }: Props) {
  const { t } = useI18n();
  const [provider, setProvider] = useState<ImageProviderStatus | null | undefined>(undefined);
  const [cast, setCast] = useState<Entity[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [scene, setScene] = useState(initialScene ?? "");
  const [format, setFormat] = useState<IllustrationFormat>("single");
  const [aspect, setAspect] = useState<IllustrationAspect>("portrait");
  const [includeReferences, setIncludeReferences] = useState(true);
  const [composition, setComposition] = useState<IllustrationComposition | null>(null);
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState<"compose" | "generate" | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [result, setResult] = useState<IllustrationMeta | null>(null);
  const resultUrl = useIllustrationImage(result?.id ?? null);

  useEffect(() => {
    imageProvidersApi
      .list()
      .then((rows) => setProvider(rows.find((r) => r.active && r.configured && r.consented) ?? null))
      .catch(() => setProvider(null));
  }, []);

  useEffect(() => {
    let cancelled = false;
    entitiesApi
      .list(projectId)
      .then(async (all) => {
        const characters = all.filter((e) => e.kind === "character");
        if (cancelled) return;
        setCast(characters);
        if (!sourceNodeId) return;
        const mentioned = await mentionsApi.listForNode(sourceNodeId).catch(() => [] as Entity[]);
        if (cancelled) return;
        const ids = new Set(characters.map((c) => c.id));
        setSelected(mentioned.filter((e) => ids.has(e.id)).map((e) => e.id));
      })
      .catch(setError);
    return () => {
      cancelled = true;
    };
  }, [projectId, sourceNodeId]);

  // A changed request makes the prompt stale; the writer builds it again.
  useEffect(() => {
    setComposition(null);
    setPrompt("");
  }, [scene, selected, format]);

  const providerName = provider ? t(`settings.providers.name.${provider.id}` as MessageKey) : "";
  const canCompose = scene.trim() !== "" && selected.length > 0 && busy === null;
  const canGenerate = Boolean(provider) && prompt.trim() !== "" && busy === null;
  const sortedCast = useMemo(() => [...cast].sort((a, b) => a.name.localeCompare(b.name)), [cast]);

  const toggle = (id: string) =>
    setSelected((cur) => (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]));

  const compose = async () => {
    setBusy("compose");
    setError(null);
    try {
      const out = await illustrationsApi.compose({ project_id: projectId, entity_ids: selected, scene, format });
      setComposition(out);
      setPrompt(out.prompt);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  const generate = async () => {
    setBusy("generate");
    setError(null);
    try {
      const meta = await illustrationsApi.generate({
        project_id: projectId,
        source_node_id: sourceNodeId,
        prompt,
        character_ids: composition?.characters.map((c) => c.entity_id) ?? selected,
        include_references: includeReferences,
        aspect,
      });
      setResult(meta);
      onSaved?.(meta);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="backdrop center" onMouseDown={busy ? undefined : onClose}>
      <div
        className="modal illustration-modal"
        role="dialog"
        aria-label={t("illustration.dialog.title")}
        onMouseDown={(e) => e.stopPropagation()}
        data-testid="illustration-dialog"
      >
        <h2>{t("illustration.dialog.title")}</h2>

        {provider === null ? (
          <div className="modal-field" data-testid="illustration-off">
            <p className="modal-sub">{t("illustration.off")}</p>
            <button type="button" className="btn ghost sm" onClick={onOpenSettings}>
              {t("illustration.openSettings")}
            </button>
          </div>
        ) : null}

        <div className="modal-field">
          <label htmlFor="illustration-scene">{t("illustration.scene")}</label>
          <textarea
            id="illustration-scene"
            rows={4}
            value={scene}
            placeholder={t("illustration.scene.placeholder")}
            onChange={(e) => setScene(e.target.value)}
            data-testid="illustration-scene"
          />
        </div>

        <div className="modal-field">
          <label>{t("illustration.characters")}</label>
          {sortedCast.length === 0 ? (
            <p className="sd">{t("illustration.characters.none")}</p>
          ) : (
            <div className="illustration-chips" data-testid="illustration-cast">
              {sortedCast.map((c) => (
                <button
                  key={c.id}
                  type="button"
                  className={selected.includes(c.id) ? "is-selected" : undefined}
                  aria-pressed={selected.includes(c.id)}
                  onClick={() => toggle(c.id)}
                  data-testid={`illustration-cast-${c.id}`}
                >
                  {c.name}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="illustration-options">
          <div className="modal-field settings-segmented" role="group">
            {(["single", "four_panel"] as IllustrationFormat[]).map((f) => (
              <button
                key={f}
                type="button"
                className={format === f ? "is-selected" : undefined}
                aria-pressed={format === f}
                onClick={() => setFormat(f)}
              >
                {f === "single" ? t("illustration.format.single") : t("illustration.format.fourPanel")}
              </button>
            ))}
          </div>
          <div className="modal-field settings-segmented" role="group" aria-label={t("illustration.aspect")}>
            {ASPECTS.map((a) => (
              <button
                key={a}
                type="button"
                className={aspect === a ? "is-selected" : undefined}
                aria-pressed={aspect === a}
                onClick={() => setAspect(a)}
                data-testid={`illustration-aspect-${a}`}
              >
                {t(`illustration.aspect.${a}` as MessageKey)}
              </button>
            ))}
          </div>
        </div>

        <label className="modal-field modal-check">
          <input
            type="checkbox"
            checked={includeReferences}
            onChange={(e) => setIncludeReferences(e.target.checked)}
            data-testid="illustration-references"
          />
          {t("illustration.includeReferences")}
        </label>

        <button
          type="button"
          className="btn ghost"
          disabled={!canCompose}
          onClick={() => void compose()}
          data-testid="illustration-compose"
        >
          {t("illustration.compose")}
        </button>

        {composition ? (
          <div className="modal-field illustration-prompt">
            <label htmlFor="illustration-prompt">{t("illustration.prompt")}</label>
            <textarea
              id="illustration-prompt"
              rows={9}
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              data-testid="illustration-prompt"
            />
            {provider ? <p className="sd">{t("illustration.prompt.hint", { provider: providerName })}</p> : null}
            {composition.missing_sheets.length > 0 ? (
              <p className="sd" data-testid="illustration-missing">
                {t("illustration.missingSheets", { names: composition.missing_sheets.join(", ") })}
              </p>
            ) : null}
          </div>
        ) : null}

        {busy === "generate" ? (
          <p className="sd" role="status" data-testid="illustration-generating">
            {t("illustration.generating")}
          </p>
        ) : null}
        {error ? (
          <p className="sd" role="alert" data-testid="illustration-error">
            {rpcErrorMessage(error, t)}
          </p>
        ) : null}

        {result ? (
          <figure className="illustration-result" data-testid="illustration-result">
            {resultUrl ? <img src={resultUrl} alt={scene} /> : null}
            <figcaption className="sd">{t("illustration.saved")}</figcaption>
          </figure>
        ) : null}

        <div className="modal-actions">
          <button type="button" className="btn ghost" onClick={onClose} disabled={busy === "generate"}>
            {t("common.close")}
          </button>
          <button
            type="button"
            className="btn accent"
            disabled={!canGenerate}
            onClick={() => void generate()}
            data-testid="illustration-generate"
          >
            {result ? t("illustration.again") : t("illustration.generate")}
          </button>
        </div>
      </div>
    </div>
  );
}
