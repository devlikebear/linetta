import { useEffect, useRef, useState } from "react";
import { visuals } from "../lib/rpc";
import type { CharacterVisualSheet, ReferenceImageMeta } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";

const fields = ["age_range", "build", "hair", "outfit", "signature", "palette", "notes"] as const;

function ReferenceImage({ image }: { image: ReferenceImageMeta }) {
  const [src, setSrc] = useState("");
  const [error, setError] = useState<unknown>(null);
  const ref = useRef<HTMLImageElement>(null);
  const { t } = useI18n();
  useEffect(() => {
    let cancelled = false;
    let started = false;
    const load = () => {
      if (started) return;
      started = true;
      visuals.getImage(image.id).then((data) => {
        if (!cancelled) setSrc(`data:${data.mime};base64,${data.data_base64}`);
      }).catch((e) => { if (!cancelled) setError(e); });
    };
    const observer = typeof IntersectionObserver === "undefined" ? null : new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) { load(); observer?.disconnect(); }
    });
    if (observer && ref.current) observer.observe(ref.current); else load();
    return () => { cancelled = true; observer?.disconnect(); };
  }, [image.id]);
  return <>{error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}<img ref={ref} src={src || undefined} alt={image.caption} width={96} height={96} style={{ objectFit: "contain" }} /></>;
}

export function CharacterVisualSection({ value, onChange }: {
  value: CharacterVisualSheet;
  onChange: (value: CharacterVisualSheet) => void;
}) {
  const { t } = useI18n();
  const [images, setImages] = useState<ReferenceImageMeta[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    let cancelled = false;
    visuals.listImages(value.entity_id).then((list) => {
      if (!cancelled) { setImages(list); setLoaded(true); }
    }).catch((e) => { if (!cancelled) setError(e); });
    return () => { cancelled = true; };
  }, [value.entity_id]);

  const addImage = async (file: File) => {
    setError(null);
    if (file.size > 5 * 1024 * 1024) { setError(t("visual.tooLarge")); return; }
    if (!["image/png", "image/jpeg", "image/webp", "image/gif"].includes(file.type)) { setError(t("visual.unsupported")); return; }
    setBusy(true);
    try {
      const data = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result).split(",")[1]);
        reader.onerror = () => reject(reader.error);
        reader.readAsDataURL(file);
      });
      const image = await visuals.addImage({ entity_id: value.entity_id, mime: file.type, caption: "", data_base64: data });
      setImages((list) => [...list, image]);
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  return <section className="sec es-field" aria-label={t("visual.title")}>
    <h4>{t("visual.title")}</h4>
    <div className="attr-grid">
      {fields.map((field) => <label key={field}>{t(`visual.${field}`)}
        <textarea value={value[field]} rows={field === "notes" ? 3 : 1} onChange={(e) => onChange({ ...value, [field]: e.target.value })} />
      </label>)}
    </div>
    {error != null && <p className="es-error" role="alert">{rpcErrorMessage(error, t)}</p>}
    <p>{images.length}/8</p>
    <div>{images.map((image) => <div key={image.id}>
      <ReferenceImage image={image} />
      <button type="button" className="btn ghost sm" disabled={busy} aria-label={t("workspace.delete")} onClick={async () => {
        setBusy(true); setError(null);
        try { await visuals.deleteImage(image.id); setImages((list) => list.filter((item) => item.id !== image.id)); }
        catch (e) { setError(e); } finally { setBusy(false); }
      }}>{t("workspace.delete")}</button>
    </div>)}</div>
    <input ref={input} type="file" hidden accept="image/png,image/jpeg,image/webp,image/gif" aria-label={t("visual.addImage")} onChange={(e) => {
      const file = e.target.files?.[0]; e.target.value = "";
      if (file) void addImage(file);
    }} />
    <button type="button" className="btn ghost sm" disabled={!loaded || busy || images.length >= 8} onClick={() => input.current?.click()}>{t("visual.addImage")}</button>
  </section>;
}
