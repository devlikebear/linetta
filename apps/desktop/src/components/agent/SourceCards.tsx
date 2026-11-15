import { displayNodeLabel, useI18n } from "../../lib/i18n";
import type { AgentCitation, AppLanguage, PassageTarget } from "../../lib/types";

/** Where a source sits, in the app's language: the outline path the engine
 *  sent in stored labels, then the scene's title. */
export function sourcePlace(language: AppLanguage, c: AgentCitation): string {
  const path = c.breadcrumb
    .split(" / ")
    .map((label) => displayNodeLabel(language, label))
    .join(" / ");
  return c.scene_title ? `${path} · ${c.scene_title}` : path;
}

export function sourceTarget(c: AgentCitation): PassageTarget {
  return { nodeId: c.node_id, paragraphFrom: c.paragraph_from, paragraphTo: c.paragraph_to, quote: c.quote };
}

/** The sources a reply rests on (#385), each a passage linetta_cite_sources
 *  found in the manuscript. Sources from one scene share a card, in the order
 *  the reply first cites them; each opens its passage. */
export function SourceCards({
  citations,
  onReveal,
}: Readonly<{ citations: AgentCitation[]; onReveal?: (target: PassageTarget) => void }>) {
  const { t, language } = useI18n();
  const scenes: { nodeId: string; place: string; items: AgentCitation[] }[] = [];
  for (const c of [...citations].sort((a, b) => a.n - b.n)) {
    const scene = scenes.find((s) => s.nodeId === c.node_id);
    if (scene) scene.items.push(c);
    else scenes.push({ nodeId: c.node_id, place: sourcePlace(language, c), items: [c] });
  }
  const paragraph = (c: AgentCitation) =>
    c.paragraph_from === c.paragraph_to
      ? t("continuity.paragraphSingle", { n: String(c.paragraph_from) })
      : t("continuity.paragraphRange", { from: String(c.paragraph_from), to: String(c.paragraph_to) });

  return (
    <div className="agent-sources" data-testid="agent-sources">
      <div className="agent-sources-title">{t("agentPanel.sources.title")}</div>
      {scenes.map((scene) => (
        <div key={scene.nodeId} className="agent-source" data-testid="agent-source">
          <div className="agent-source-place">{scene.place}</div>
          {scene.items.map((c) => (
            <button
              key={c.n}
              type="button"
              className="agent-source-item"
              data-testid={`agent-source-${c.n}`}
              disabled={!onReveal}
              title={t("agentPanel.sources.reveal")}
              onClick={() => onReveal?.(sourceTarget(c))}
            >
              <span className="agent-source-n">{c.n}</span>
              <span className="agent-source-para">{paragraph(c)}</span>
              <span className="agent-source-quote">&ldquo;{c.quote}&rdquo;</span>
            </button>
          ))}
        </div>
      ))}
    </div>
  );
}
