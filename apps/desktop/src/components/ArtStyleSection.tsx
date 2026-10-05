import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { useEffect, useState } from "react";
import { visuals } from "../lib/rpc";
import type { ArtStyle } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";

export function ArtStyleSection({ projectId }: Readonly<{ projectId: string }>) {
  const { t } = useI18n();
  const [revision, setRevision] = useState(0);
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id && event.project_id !== projectId) return;
    if (event.tool === "linetta_undo_last_change" || ["linetta_set_character_visuals", "linetta_set_art_style", "linetta_add_reference_image", "linetta_delete_reference_image"].includes(event.tool ?? "")) setRevision((n) => n + 1);
  });
  const [value, setValue] = useState<ArtStyle | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    let cancelled = false;
    visuals.getArtStyle(projectId).then((style) => { if (!cancelled) setValue(style); })
      .catch((e) => { if (!cancelled) setError(e); });
    return () => { cancelled = true; };
  }, [projectId, revision]);
  return <details className="sec es-field">
    <summary>{t("visual.artTitle")}</summary>
    {error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}
    {value && <>
      <label>{t("visual.style")}<textarea value={value.style} disabled={saving} onChange={(e) => setValue({ ...value, style: e.target.value })} /></label>
      <label>{t("visual.negative")}<textarea value={value.negative_prompt} disabled={saving} onChange={(e) => setValue({ ...value, negative_prompt: e.target.value })} /></label>
      <button type="button" className="btn accent sm" disabled={saving} onClick={async () => {
        setSaving(true); setError(null);
        try { setValue(await visuals.setArtStyle(value)); } catch (e) { setError(e); } finally { setSaving(false); }
      }}>{saving ? t("common.saving") : t("common.save")}</button>
    </>}
  </details>;
}
