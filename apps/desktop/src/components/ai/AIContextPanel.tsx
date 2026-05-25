import type { Entity, Project } from "../../lib/types";

interface Props {
  project: Project;
  mentioned: Entity[];
  options: { tone_preset: boolean; short_form: boolean };
  onOptionsChange: (o: { tone_preset: boolean; short_form: boolean }) => void;
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
        <h4>옵션</h4>
        <label className="ctx-check">
          <input
            type="checkbox"
            checked={options.tone_preset}
            onChange={(e) => onOptionsChange({ ...options, tone_preset: e.target.checked })}
          />
          톤 프리셋 "내 톤"
        </label>
        <label className="ctx-check">
          <input
            type="checkbox"
            checked={options.short_form}
            onChange={(e) => onOptionsChange({ ...options, short_form: e.target.checked })}
          />
          길이: 한 문단
        </label>
      </section>
    </aside>
  );
}
