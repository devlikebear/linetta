import { useCallback, useEffect, useRef, useState } from "react";
import { BookOpen, ExternalLink, Search, Trash2, X } from "lucide-react";
import { facts as factsApi } from "../lib/rpc";
import type { FactCard, FactStatus } from "../lib/types";
import { useI18n, type MessageKey } from "../lib/i18n";
import "./FactBookPanel.css";

/** The writer's dossier: claims they have checked, and where they checked them.
 *
 *  This used to drive the companion — "review this scene", "find me a source"
 *  — and the MCP pivot took that half away. What is left is the half that was
 *  never about a model: the writer names a claim, gives a URL, and Linetta
 *  fetches the page and files it. An agent connected over MCP can read these
 *  cards with linetta_get_fact_cards.
 */

interface Props {
  projectId: string;
  nodeId: string;
  selectedClaimRequest?: { id: string; claim: string } | null;
  onClose: () => void;
  onChanged?: () => void;
  onImpactCheck?: (text: string) => void;
}

function statusKey(status: FactStatus): MessageKey {
  switch (status) {
    case "verified": return "factBook.status.verified";
    case "intentional_fiction": return "factBook.status.intentionalFiction";
    case "stale": return "factBook.status.stale";
    default: return "factBook.status.uncertain";
  }
}

function firstURL(text: string): string {
  const match = text.match(/https?:\/\/[^\s<>"']+/i);
  return match ? match[0].replace(/[),.;\]]+$/, "") : "";
}

export function FactBookPanel({
  projectId,
  nodeId,
  selectedClaimRequest,
  onClose,
  onChanged,
  onImpactCheck,
}: Props) {
  const { t } = useI18n();
  const [cards, setCards] = useState<FactCard[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [claimDraft, setClaimDraft] = useState("");
  const [urlDraft, setUrlDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const [note, setNote] = useState("");
  const [noteKind, setNoteKind] = useState<"ok" | "error">("ok");
  const claimInputRef = useRef<HTMLInputElement | null>(null);
  const handledClaimRequestRef = useRef<string | null>(null);

  const loadFacts = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setCards(await factsApi.list(projectId, nodeId));
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  }, [projectId, nodeId]);

  useEffect(() => { void loadFacts(); }, [loadFacts]);

  // Selecting prose and choosing "fact check" fills the claim in rather than
  // asking a model about it. The writer still has to supply the source, which
  // is the part that makes the card worth anything.
  useEffect(() => {
    if (!selectedClaimRequest || handledClaimRequestRef.current === selectedClaimRequest.id) return;
    const claim = selectedClaimRequest.claim.trim();
    if (!claim) return;
    handledClaimRequestRef.current = selectedClaimRequest.id;
    setClaimDraft(claim);
    setNote("");
    claimInputRef.current?.focus();
  }, [selectedClaimRequest]);

  const saveCard = async () => {
    const claim = claimDraft.trim();
    const url = firstURL(urlDraft);
    if (saving) return;
    if (!claim) {
      setNoteKind("error");
      setNote(t("factBook.directNeedsClaim"));
      return;
    }
    if (!url) {
      setNoteKind("error");
      setNote(t("factBook.needsUrl"));
      return;
    }
    setSaving(true);
    setNote("");
    try {
      // The engine has no idea what language the writer reads, so the card's
      // default sentence is supplied here (#45).
      await factsApi.createFromUrl({
        project_id: projectId,
        node_id: nodeId,
        claim,
        url,
        result: t("factBook.savedFromUrlResult"),
      });
      setClaimDraft("");
      setUrlDraft("");
      setNoteKind("ok");
      setNote(t("factBook.directSaved"));
      await loadFacts();
      onChanged?.();
    } catch (err) {
      setNoteKind("error");
      setNote(t("factBook.directSaveFailed", { error: String(err) }));
    } finally {
      setSaving(false);
    }
  };

  const deleteCard = async (id: string) => {
    await factsApi.delete(id);
    await loadFacts();
    onChanged?.();
  };

  return (
    <aside className="panel fact-panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl"><span className="ic"><BookOpen size={16} /></span> {t("factBook.title")}</span>
        <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
      </div>

      <form
        className="fact-new"
        onSubmit={(e) => {
          e.preventDefault();
          void saveCard();
        }}
      >
        <input
          ref={claimInputRef}
          type="text"
          value={claimDraft}
          aria-label={t("factBook.claimLabel")}
          placeholder={t("factBook.claimPlaceholder")}
          disabled={saving}
          onChange={(e) => setClaimDraft(e.target.value)}
        />
        <input
          type="text"
          value={urlDraft}
          aria-label={t("factBook.urlLabel")}
          placeholder={t("factBook.urlPlaceholder")}
          disabled={saving}
          onChange={(e) => setUrlDraft(e.target.value)}
        />
        <button type="submit" className="btn accent sm" disabled={saving}>
          {saving ? t("factBook.saving") : t("factBook.save")}
        </button>
        <p className="fact-new-hint">{t("factBook.hint")}</p>
        {note && <div className={`fact-feedback ${noteKind}`}>{note}</div>}
      </form>

      <div className="panel-scroll fact-list">
        {loading && <p className="fact-empty">{t("factBook.loading")}</p>}
        {error && <p className="fact-empty">{t("factBook.failed", { error })}</p>}
        {!loading && !error && cards.length === 0 && <p className="fact-empty">{t("factBook.empty")}</p>}
        {cards.map((card) => (
          <article key={card.id} className="fact-card">
            <div className="fact-card-head">
              <span className={`fact-status ${card.status}`}>{t(statusKey(card.status))}</span>
              <span className="fact-scope">{card.node_id ? t("factBook.currentScene") : t("factBook.projectWide")}</span>
              {onImpactCheck && (
                <button
                  type="button"
                  className="panel-close"
                  onClick={() => onImpactCheck([card.claim, card.result].filter(Boolean).join(" "))}
                  aria-label={t("factBook.impactCheck")}
                  title={t("factBook.impactCheck")}
                >
                  <Search size={14} />
                </button>
              )}
              <button type="button" className="panel-close" onClick={() => { void deleteCard(card.id); }} aria-label={t("factBook.delete")} title={t("factBook.delete")}>
                <Trash2 size={14} />
              </button>
            </div>
            <h3>{card.claim}</h3>
            <p>{card.result}</p>
            {card.category && <div className="fact-category">{card.category}</div>}
            {card.sources.length > 0 && (
              <div className="fact-sources">
                <div className="fact-sources-title">{t("factBook.sources")}</div>
                {card.sources.map((source) => (
                  <a key={source.id} href={source.url} target="_blank" rel="noreferrer">
                    <ExternalLink size={12} />
                    <span>{source.title || source.url}</span>
                  </a>
                ))}
              </div>
            )}
          </article>
        ))}
      </div>
    </aside>
  );
}
