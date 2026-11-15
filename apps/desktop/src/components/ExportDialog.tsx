import { useEffect, useState } from "react";
import { exportApi } from "../lib/rpc";
import { exportDestinationMessage, saveExportedFile, saveExportedMarkdown } from "../lib/exportSave";
import { useI18n } from "../lib/i18n";
import type { ExportDocumentOptions, ExportFormat } from "../lib/types";
import "./ExportDialog.css";

/** One thing the writer can export: the whole work (no nodeId), or a part,
 *  episode or scene of it. `leaf` marks a scene: the Markdown export works on
 *  the whole work or one scene, so it is offered only for those. */
export interface ExportScope {
  nodeId?: string;
  label: string;
  leaf?: boolean;
}

interface Props {
  projectId: string;
  /** The first is the default; the whole work comes first. */
  scopes: ExportScope[];
  onClose: () => void;
  /** Says where the file went, or why it could not be written. */
  onMessage: (message: string) => void;
}

const FONTS = ["함초롬바탕", "함초롬돋움", "바탕", "맑은 고딕", "나눔명조", "Noto Serif KR", "Times New Roman"];
const SIZES = [9, 10, 10.5, 11, 12];
const LINE_SPACINGS = [130, 150, 160, 180, 200];
const SCENE_BREAKS = ["* * *", "◇", "#", ""];

interface Remembered {
  format: ExportFormat;
  options: Required<ExportDocumentOptions>;
}

/** Korean manuscript practice, the engine's own defaults (docx.DefaultOptions). */
const DEFAULTS: Remembered = {
  format: "docx",
  options: {
    font: "함초롬바탕",
    size_pt: 10,
    line_spacing_pct: 160,
    first_line_indent: true,
    page_break_per_episode: true,
    headings: true,
    scene_break: "* * *",
    illustrations: false,
  },
};

const KEY = "linetta.exportOptions.";

/** The last choices for this work. Kept in this browser only: they are how
 *  the writer likes the file, not part of the work, and nothing syncs them. */
function readRemembered(projectId: string): Remembered {
  try {
    const raw = window.localStorage.getItem(KEY + projectId);
    if (!raw) return DEFAULTS;
    const saved = JSON.parse(raw) as Partial<Remembered>;
    return {
      format: saved.format === "txt" || saved.format === "md" ? saved.format : "docx",
      options: { ...DEFAULTS.options, ...(saved.options ?? {}) },
    };
  } catch {
    return DEFAULTS;
  }
}

function remember(projectId: string, value: Remembered) {
  try {
    window.localStorage.setItem(KEY + projectId, JSON.stringify(value));
  } catch {
    // A convenience; the export works without it.
  }
}

/** Exports the work, or a part of it, as DOCX, TXT or Markdown (#383). DOCX
 *  and TXT come from export.document; Markdown keeps its own export, which
 *  carries the frontmatter an import reads back. */
export function ExportDialog({ projectId, scopes, onClose, onMessage }: Readonly<Props>) {
  const { t } = useI18n();
  const [scopeIndex, setScopeIndex] = useState(0);
  const [{ format, options }, setChoices] = useState<Remembered>(() => readRemembered(projectId));
  const [summary, setSummary] = useState<{ scenes: number; chars: number } | null>(null);
  const [busy, setBusy] = useState(false);
  const scope = scopes[scopeIndex] ?? scopes[0];
  const markdownFits = !scope.nodeId || scope.leaf === true;

  const choose = (next: Partial<Remembered>) => {
    setChoices((cur) => {
      const value = { format: next.format ?? cur.format, options: { ...cur.options, ...(next.options ?? {}) } };
      remember(projectId, value);
      return value;
    });
  };
  const setOption = <K extends keyof ExportDocumentOptions>(key: K, value: Required<ExportDocumentOptions>[K]) =>
    choose({ options: { ...options, [key]: value } });

  useEffect(() => {
    let cancelled = false;
    setSummary(null);
    exportApi
      .document({ project_id: projectId, node_id: scope.nodeId, format: "txt", preview: true })
      .then((p) => {
        if (!cancelled) setSummary({ scenes: p.scenes, chars: p.chars });
      })
      .catch(() => {
        // The summary is a courtesy; exporting still works without it.
      });
    return () => {
      cancelled = true;
    };
  }, [projectId, scope.nodeId]);

  const effectiveFormat: ExportFormat = format === "md" && !markdownFits ? "docx" : format;

  const run = async () => {
    setBusy(true);
    try {
      let path: string | null;
      if (effectiveFormat === "md") {
        const payload = scope.nodeId ? await exportApi.node(scope.nodeId) : await exportApi.project(projectId);
        path = await saveExportedMarkdown(payload);
      } else {
        const payload = await exportApi.document({
          project_id: projectId,
          node_id: scope.nodeId,
          format: effectiveFormat,
          options,
        });
        path = await saveExportedFile(payload, effectiveFormat);
      }
      if (path) {
        onMessage(exportDestinationMessage(t, path));
        onClose();
      }
    } catch (e) {
      onMessage(t("exportDialog.failed", { error: String(e) }));
    } finally {
      setBusy(false);
    }
  };

  const layout = effectiveFormat === "docx";
  const prose = effectiveFormat !== "md";
  return (
    <div className="backdrop center" onClick={onClose}>
      <div
        className="modal export-dialog"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="export-dialog-title"
        data-testid="export-dialog"
      >
        <h2 id="export-dialog-title">{t("exportDialog.title")}</h2>

        <div className="modal-field">
          <label>{t("exportDialog.scope")}</label>
          <div className="chips">
            {scopes.map((s, i) => (
              <button
                key={s.nodeId ?? "work"}
                type="button"
                className={`chip${i === scopeIndex ? " on" : ""}`}
                aria-pressed={i === scopeIndex}
                data-testid={`export-scope-${i}`}
                onClick={() => setScopeIndex(i)}
              >
                {s.label}
              </button>
            ))}
          </div>
        </div>

        <div className="modal-field">
          <label>{t("exportDialog.format")}</label>
          <div className="chips">
            {(["docx", "txt", "md"] as const).map((f) => (
              <button
                key={f}
                type="button"
                className={`chip${effectiveFormat === f ? " on" : ""}`}
                aria-pressed={effectiveFormat === f}
                data-testid={`export-format-${f}`}
                disabled={f === "md" && !markdownFits}
                title={f === "md" && !markdownFits ? t("exportDialog.markdownScope") : undefined}
                onClick={() => choose({ format: f })}
              >
                {t(`exportDialog.format.${f}`)}
              </button>
            ))}
          </div>
        </div>

        {layout ? (
          <div className="export-grid" data-testid="export-layout">
            <label>
              {t("exportDialog.font")}
              <select value={options.font} onChange={(e) => setOption("font", e.target.value)}>
                {[...new Set([options.font, ...FONTS])].map((f) => (
                  <option key={f} value={f}>{f}</option>
                ))}
              </select>
            </label>
            <label>
              {t("exportDialog.size")}
              <select value={options.size_pt} onChange={(e) => setOption("size_pt", Number(e.target.value))}>
                {SIZES.map((s) => (
                  <option key={s} value={s}>{s}pt</option>
                ))}
              </select>
            </label>
            <label>
              {t("exportDialog.lineSpacing")}
              <select value={options.line_spacing_pct} onChange={(e) => setOption("line_spacing_pct", Number(e.target.value))}>
                {LINE_SPACINGS.map((s) => (
                  <option key={s} value={s}>{s}%</option>
                ))}
              </select>
            </label>
            <label className="export-check">
              <input
                type="checkbox"
                checked={options.first_line_indent}
                onChange={(e) => setOption("first_line_indent", e.target.checked)}
              />
              {t("exportDialog.firstLineIndent")}
            </label>
            <label className="export-check">
              <input
                type="checkbox"
                checked={options.page_break_per_episode}
                onChange={(e) => setOption("page_break_per_episode", e.target.checked)}
              />
              {t("exportDialog.pageBreak")}
            </label>
          </div>
        ) : null}

        {prose ? (
          <div className="export-grid">
            <label className="export-check">
              <input type="checkbox" checked={options.headings} onChange={(e) => setOption("headings", e.target.checked)} />
              {t("exportDialog.headings")}
            </label>
            <label>
              {t("exportDialog.sceneBreak")}
              <select value={options.scene_break} onChange={(e) => setOption("scene_break", e.target.value)}>
                {SCENE_BREAKS.map((s) => (
                  <option key={s} value={s}>{s === "" ? t("exportDialog.sceneBreak.blank") : s}</option>
                ))}
              </select>
            </label>
            <label className="export-check">
              <input
                type="checkbox"
                checked={options.illustrations}
                onChange={(e) => setOption("illustrations", e.target.checked)}
              />
              {t("exportDialog.illustrations")}
            </label>
          </div>
        ) : (
          <p className="modal-sub">{t("exportDialog.markdownNote")}</p>
        )}

        <p className="modal-sub export-summary" data-testid="export-summary">
          {summary
            ? t("exportDialog.summary", { scenes: summary.scenes.toLocaleString(), chars: summary.chars.toLocaleString() })
            : " "}
        </p>

        <div className="modal-actions">
          <button type="button" className="btn ghost" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="button" className="btn accent" data-testid="export-run" disabled={busy} onClick={() => void run()}>
            {t("exportDialog.run")}
          </button>
        </div>
      </div>
    </div>
  );
}
