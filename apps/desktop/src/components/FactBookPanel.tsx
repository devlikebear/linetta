import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { BookOpen, CornerDownLeft, ExternalLink, Search, Trash2, X } from "lucide-react";
import { facts as factsApi } from "../lib/rpc";
import type { FactCard, FactStatus } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { useCompanion } from "../hooks/useCompanion";
import { ChoiceCard } from "./companion/ChoiceCard";
import { Markdown } from "./companion/Markdown";
import "./FactBookPanel.css";

interface Props {
  projectId: string;
  nodeId: string;
  sceneLabel: string;
  beforeReview?: () => Promise<void> | void;
  onClose: () => void;
  onChanged?: () => void;
}

function buildReviewPrompt(sceneLabel: string): string {
  return [
    `현재 씬 "${sceneLabel}"에서 웹검색 팩트체크가 필요한 현실 주장 후보를 찾아줘.`,
    "아직 web_search나 web_fetch를 실행하지 마.",
    "후보가 있으면 설명은 짧게 하고 linetta-choices 블록 하나로만 보여줘.",
    "각 options 항목은 반드시 `검색 후 자료집에 저장: <주장>` 형식으로 작성해.",
    "작가가 후보를 선택하면 그때 web_search와 web_fetch로 출처 URL을 확인하고, create_fact_card로 자료집에 저장해.",
    "web_search API 키가 없어 실패하면 포기하지 말고 작가에게 출처 URL 직접 입력을 요청해. 작가가 URL을 입력하면 web_fetch로 확인한 뒤 저장해.",
    "출처 URL 없는 create_fact_card는 금지야. 후보가 없으면 짧게 이유만 말해.",
  ].join("\n");
}

function statusKey(status: FactStatus): string {
  switch (status) {
    case "verified": return "factBook.status.verified";
    case "intentional_fiction": return "factBook.status.intentionalFiction";
    case "stale": return "factBook.status.stale";
    default: return "factBook.status.uncertain";
  }
}

export function FactBookPanel({ projectId, nodeId, sceneLabel, beforeReview, onClose, onChanged }: Props) {
  const { t } = useI18n();
  const [cards, setCards] = useState<FactCard[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reviewing, setReviewing] = useState(false);
  const [replyDraft, setReplyDraft] = useState("");
  const nodeIdRef = useRef<string | null>(nodeId);
  const replyInputRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => { nodeIdRef.current = nodeId; }, [nodeId]);

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

  const { messages, streaming, thinking, status, send } = useCompanion(projectId, nodeIdRef, () => {
    void loadFacts();
    onChanged?.();
  });
  const busy = status === "streaming";

  useEffect(() => { void loadFacts(); }, [loadFacts]);

  const latestAssistant = useMemo(() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].role === "assistant") return messages[i];
    }
    return null;
  }, [messages]);

  const startReview = async () => {
    if (reviewing || status === "streaming") return;
    setReviewing(true);
    try {
      await beforeReview?.();
      await send(buildReviewPrompt(sceneLabel));
    } finally {
      setReviewing(false);
    }
  };

  const submitReply = async () => {
    const text = replyDraft.trim();
    if (!text || busy) return;
    setReplyDraft("");
    await send(text);
  };

  const focusReplyInput = () => {
    replyInputRef.current?.focus();
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

      <div className="fact-review">
        <button type="button" className="btn accent sm" onClick={startReview} disabled={reviewing || busy}>
          <Search size={14} /> {reviewing || busy ? t("factBook.reviewing") : t("factBook.reviewScene")}
        </button>
        <p>{t("factBook.reviewHint")}</p>
      </div>

      {(status === "streaming" || streaming || latestAssistant?.choices) && (
        <section className="fact-companion-box">
          {status === "streaming" && <div className="companion-thinking"><span className="ai-working-dot" aria-hidden="true" /> {thinking || t("companion.thinking")}</div>}
          {streaming && <div className="fact-companion-prose"><Markdown text={streaming} /></div>}
          {latestAssistant?.content && <div className="fact-companion-prose"><Markdown text={latestAssistant.content} /></div>}
          {latestAssistant?.choices && (
            <ChoiceCard
              choices={latestAssistant.choices}
              disabled={busy}
              onPick={(text) => { void send(text); }}
              onCustom={focusReplyInput}
            />
          )}
        </section>
      )}

      <form
        className="fact-reply"
        onSubmit={(e) => {
          e.preventDefault();
          void submitReply();
        }}
      >
        <textarea
          ref={replyInputRef}
          value={replyDraft}
          aria-label={t("factBook.directInput")}
          placeholder={t("factBook.directPlaceholder")}
          rows={2}
          disabled={busy}
          onChange={(e) => setReplyDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void submitReply();
            }
          }}
        />
        <button type="submit" className="fact-reply-send" disabled={!replyDraft.trim() || busy} aria-label={t("companion.send")}>
          <CornerDownLeft size={15} />
        </button>
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
