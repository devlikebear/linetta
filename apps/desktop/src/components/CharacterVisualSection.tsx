import { useEngineEvent } from "../hooks/useEngineEvent";
import type { McpChangedPayload } from "../hooks/useMcpChanges";
import { useEffect, useRef, useState } from "react";
import { ImagePlus, Images, X } from "lucide-react";
import { illustrations, visuals } from "../lib/rpc";
import type { CharacterVisualSheet, IllustrationMeta, ReferenceImageMeta } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { rpcErrorMessage } from "../lib/rpcMessage";
import { IMAGE_ACCEPT, ImageFileProblem, imageFiles, readImageFile } from "../lib/imageFile";
import { useIllustrationImage } from "./illustration/useIllustrationImage";

const fields = ["age_range", "build", "hair", "outfit", "signature", "palette", "notes"] as const;
/** visual.MaxImagesPerEntity */
const MAX_IMAGES = 8;

function GalleryPick({ meta, disabled, onPick }: Readonly<{ meta: IllustrationMeta; disabled: boolean; onPick: () => void }>) {
  const url = useIllustrationImage(meta.id);
  return <button type="button" className="visual-gallery-pick" disabled={disabled} onClick={onPick} aria-label={meta.caption || meta.id} title={meta.caption || undefined} data-testid={`visual-gallery-${meta.id}`}>
    {url ? <img src={url} alt="" /> : null}
  </button>;
}

function ReferenceImage({ image }: Readonly<{ image: ReferenceImageMeta }>) {
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
  return <>{error != null && <p role="alert">{rpcErrorMessage(error, t)}</p>}<img ref={ref} src={src || undefined} alt={image.caption} /></>;
}

export function CharacterVisualSection({ value, projectId, onChange, onRefresh }: Readonly<{
  value: CharacterVisualSheet;
  projectId: string;
  onRefresh: (value: CharacterVisualSheet) => void;
  onChange: (value: CharacterVisualSheet) => void;
}>) {
  const { t } = useI18n();
  const [revision, setRevision] = useState(0);
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.project_id && event.project_id !== projectId) return;
    if (event.tool === "linetta_undo_last_change" || ["linetta_set_character_visuals", "linetta_set_art_style", "linetta_add_reference_image", "linetta_delete_reference_image"].includes(event.tool ?? "")) setRevision((n) => n + 1);
  });
  const [images, setImages] = useState<ReferenceImageMeta[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const onRefreshRef = useRef(onRefresh);
  onRefreshRef.current = onRefresh;
  useEffect(() => {
    let cancelled = false;
    if (revision > 0) visuals.getSheet(value.entity_id).then((sheet) => { if (!cancelled) onRefreshRef.current(sheet); }).catch((e) => { if (!cancelled) setError(e); });
    visuals.listImages(value.entity_id).then((list) => {
      if (!cancelled) { setImages(list); setLoaded(true); }
    }).catch((e) => { if (!cancelled) setError(e); });
    return () => { cancelled = true; };
  }, [value.entity_id, revision]);

  const [gallery, setGallery] = useState<IllustrationMeta[] | null>(null);
  const [dragging, setDragging] = useState(false);
  const room = MAX_IMAGES - images.length;

  // Several files at once: picked, or dropped on the section. Each goes up on
  // its own, so one refused file does not lose the others.
  const addFiles = async (files: File[]) => {
    setError(null);
    if (files.length === 0) return;
    if (files.length > room) setError(t("visual.tooMany"));
    setBusy(true);
    try {
      for (const file of files.slice(0, Math.max(0, room))) {
        try {
          const read = await readImageFile(file);
          const image = await visuals.addImage({ entity_id: value.entity_id, mime: read.mime, caption: "", data_base64: read.data_base64 });
          setImages((list) => [...list, image]);
        } catch (e) {
          setError(e instanceof ImageFileProblem ? t(e.reason === "tooLarge" ? "visual.tooLarge" : "visual.unsupported") : e);
        }
      }
    } finally { setBusy(false); }
  };

  // A picture already drawn for the work — the face the writer liked in
  // episode 1 — kept as this character's reference.
  const addFromGallery = async (meta: IllustrationMeta) => {
    setError(null);
    setBusy(true);
    try {
      const img = await illustrations.get(meta.id);
      const image = await visuals.addImage({ entity_id: value.entity_id, mime: img.mime, caption: meta.caption, data_base64: img.data_base64 });
      setImages((list) => [...list, image]);
      setGallery(null);
    } catch (e) { setError(e); } finally { setBusy(false); }
  };

  const openGallery = async () => {
    if (gallery) { setGallery(null); return; }
    setError(null);
    try { setGallery((await illustrations.list(projectId)).items); } catch (e) { setError(e); }
  };

  return <section className="sec es-field" aria-label={t("visual.title")}>
    <h4>{t("visual.title")}</h4>
    <div className="attr-grid">
      {fields.map((field) => <label key={field}>{t(`visual.${field}`)}
        <textarea value={value[field]} rows={field === "notes" ? 3 : 1} onChange={(e) => onChange({ ...value, [field]: e.target.value })} />
      </label>)}
    </div>
    <div
      className={`visual-refs${dragging ? " is-dragging" : ""}`}
      data-testid="visual-refs"
      onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) { e.preventDefault(); setDragging(true); } }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        setDragging(false);
        const files = imageFiles(e.dataTransfer.files);
        if (files.length === 0 || !loaded || busy) return;
        e.preventDefault();
        void addFiles(files);
      }}
    >
      <div className="visual-refs-head">
        <span>{t("visual.images")}</span>
        <span className="sd">{images.length}/{MAX_IMAGES}</span>
      </div>
      <p className="sd">{t("visual.images.hint")}</p>
      {images.length > 0 && <div className="visual-refs-grid">{images.map((image) => <div key={image.id} className="visual-ref">
        <ReferenceImage image={image} />
        <button type="button" className="visual-ref-remove" disabled={busy} aria-label={t("workspace.delete")} onClick={async () => {
          setBusy(true); setError(null);
          try { await visuals.deleteImage(image.id); setImages((list) => list.filter((item) => item.id !== image.id)); }
          catch (e) { setError(e); } finally { setBusy(false); }
        }}><X size={12} /></button>
      </div>)}</div>}
      {error != null && <p className="es-error" role="alert">{rpcErrorMessage(error, t)}</p>}
      <input ref={input} type="file" hidden multiple accept={IMAGE_ACCEPT} aria-label={t("visual.addImage")} onChange={(e) => {
        const files = Array.from(e.target.files ?? []); e.target.value = "";
        void addFiles(files);
      }} />
      <div className="visual-refs-actions">
        <button type="button" className="btn ghost sm" disabled={!loaded || busy || room <= 0} onClick={() => input.current?.click()}><ImagePlus size={13} /> {t("visual.addImage")}</button>
        <button type="button" className="btn ghost sm" disabled={!loaded || busy || room <= 0} aria-expanded={gallery !== null} onClick={() => void openGallery()} data-testid="visual-from-gallery"><Images size={13} /> {t("visual.fromGallery")}</button>
      </div>
      {gallery && (gallery.length === 0
        ? <p className="sd">{t("visual.fromGallery.empty")}</p>
        : <div className="visual-gallery" role="group" aria-label={t("visual.fromGallery")}>
          {gallery.map((meta) => <GalleryPick key={meta.id} meta={meta} disabled={busy || room <= 0} onPick={() => void addFromGallery(meta)} />)}
        </div>)}
    </div>
  </section>;
}
