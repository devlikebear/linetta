import type { MessageKey } from "./i18n";
import { useI18n } from "./i18n";

type Translate = ReturnType<typeof useI18n>["t"];

/**
 * Import warnings arrive from the engine as codes, because the engine does not
 * know what language the reader uses. A code may carry a value after a colon —
 * `import.unknown_outline_preset:novella` — which becomes a message variable.
 *
 * Keep in step with engine/internal/importmd/warnings.go.
 */
const WARNING_KEYS: Record<string, MessageKey> = {
  "import.no_headings": "importWarning.noHeadings",
  "import.frontmatter_unreadable": "importWarning.frontmatterUnreadable",
  "import.relationships_skipped": "importWarning.relationshipsSkipped",
  "import.unknown_outline_preset": "importWarning.unknownOutlinePreset",
  "import.project_field_skipped": "importWarning.projectFieldSkipped",
  "import.nodes_meta_partial": "importWarning.nodesMetaPartial",
  "import.node_links_dropped": "importWarning.nodeLinksDropped",
  "import.notes_skipped": "importWarning.notesSkipped",
  "import.fact_cards_skipped": "importWarning.factCardsSkipped",
};

/** Human-readable text for one engine warning.
 *
 *  Anything unrecognised is shown as-is: a warning the reader cannot act on is
 *  still better than a warning that silently disappears. */
export function importWarningMessage(t: Translate, warning: string): string {
  const colon = warning.indexOf(":");
  const code = colon === -1 ? warning : warning.slice(0, colon);
  const value = colon === -1 ? "" : warning.slice(colon + 1);
  const key = WARNING_KEYS[code];
  if (!key) return warning;
  return t(key, value ? { value } : undefined);
}
