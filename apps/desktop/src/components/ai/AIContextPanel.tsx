import type { AIOptions, Entity, Project } from "../../lib/types";
import { TONE_PRESETS } from "../../lib/tonePresets";

interface Props {
  project: Project;
  mentioned: Entity[];
  options: AIOptions;
  onOptionsChange: (o: AIOptions) => void;
}

export function AIContextPanel({ project, mentioned, options, onOptionsChange }: Props) {
  return (
    <aside className="ctx-panel">
      <section className="ctx-section">
        <h4>AI에게 전달됨</h4>
        <ul className="ai-context-checklist">
          <li>✓ 현재 씬 본문</li>
          <li>✓ 직전 씬 발췌 (300자)</li>
          {mentioned.length > 0 && (
            <li>
              ✓ @멘션: {mentioned.map((e) => e.name).join(", ")}
            </li>
          )}
          {project.style_notes && <li>✓ 작품 style notes</li>}
        </ul>
      </section>
      <section className="ctx-section">
        <h4>톤 · 길이</h4>
        <div className="ai-chip-row">
          {TONE_PRESETS.map((t) => (
            <button
              type="button"
              key={t.id}
              className={`ai-chip${options.tone === t.id ? " active" : ""}`}
              onClick={() => onOptionsChange({ ...options, tone: t.id })}
            >
              {t.label}
            </button>
          ))}
          <button
            type="button"
            className={`ai-chip${options.short_form ? " active" : ""}`}
            onClick={() => onOptionsChange({ ...options, short_form: !options.short_form })}
            aria-pressed={options.short_form}
          >
            한 문단
          </button>
        </div>
      </section>
    </aside>
  );
}
