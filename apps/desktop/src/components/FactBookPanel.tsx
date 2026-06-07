import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { BookOpen, CornerDownLeft, ExternalLink, Search, Trash2, X } from "lucide-react";
import { facts as factsApi } from "../lib/rpc";
import type { CompanionProposal, FactCard, FactStatus } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { extractApplyOpsProposal, stripProposalBlock } from "../lib/companionDisplay";
import { useCompanion } from "../hooks/useCompanion";
import { ChoiceCard } from "./companion/ChoiceCard";
import { Markdown } from "./companion/Markdown";
import { ProposalCard } from "./companion/ProposalCard";
import "./FactBookPanel.css";

interface Props {
  projectId: string;
  nodeId: string;
  sceneLabel: string;
  selectedClaimRequest?: { id: string; claim: string } | null;
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
    "web_fetch가 404/403/본문 부족이면 그 URL은 저장 후보에서 제외하고 대체 출처를 더 찾아.",
    "web_search API 키가 없어 실패하면 포기하지 말고 작가에게 출처 URL 직접 입력을 요청해. 작가가 URL을 입력하면 web_fetch로 확인한 뒤 저장해.",
    "출처 URL 없는 create_fact_card는 금지야. 후보가 없으면 짧게 이유만 말해.",
  ].join("\n");
}

function claimFromChoice(text: string): string {
  return text.replace(/^검색 후 자료집에 저장:\s*/i, "").trim();
}

function normalizeClaim(text: string): string {
  return text.replace(/\s+/g, " ").trim().toLowerCase();
}

function buildFactCheckPrompt(claim: string): string {
  return [
    `선택한 주장: ${claim}`,
    "지금 바로 web_search로 신뢰 가능한 출처 후보를 찾고, 최소 1개 URL은 web_fetch로 본문 접근을 확인해.",
    "web_fetch가 404/403/본문 부족이면 그 URL은 저장 후보에서 제외하고 같은 턴에 대체 출처를 더 검색해.",
    "확인된 출처 URL이 있으면 같은 턴에 linetta_apply_ops의 create_fact_card로 자료집에 저장해.",
    "create_fact_card 호출 없이 저장 완료라고 말하지 마. 저장이 실패하면 실패 이유와 필요한 다음 행동만 짧게 말해.",
    "web_search나 web_fetch가 실패하면 포기하지 말고 출처 URL 직접 입력을 요청해.",
  ].join("\n");
}

function buildAlternativeSourcePrompt(claim: string, failedURL: string, error: string): string {
  return [
    `선택한 주장: ${claim}`,
    `방금 이 출처 URL은 앱의 저장 경로에서 실패했습니다: ${failedURL}`,
    `실패 이유: ${error}`,
    "이 URL은 저장 후보에서 제외해.",
    "지금 바로 web_search로 신뢰 가능한 대체 출처 후보를 찾고, 최소 1개 URL은 web_fetch로 본문 접근을 확인해.",
    "web_fetch가 404/403/본문 부족이면 그 URL도 저장 후보에서 제외하고 같은 턴에 대체 출처를 더 검색해.",
    "확인된 출처 URL이 있으면 같은 턴에 linetta_apply_ops의 create_fact_card로 자료집에 저장해.",
    "확인된 대체 출처가 없으면 저장했다고 말하지 말고, 출처 URL 직접 입력만 요청해.",
  ].join("\n");
}

function firstURL(text: string): string {
  const match = text.match(/https?:\/\/[^\s<>"']+/i);
  return match ? match[0].replace(/[),.;\]]+$/, "") : "";
}

function sourceURLFromAssistant(text: string): string {
  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i += 1) {
    const urls = lines[i].match(/https?:\/\/[^\s<>"']+/gi) ?? [];
    for (const raw of urls) {
      const context = [lines[i - 1] ?? "", lines[i], lines[i + 1] ?? ""].join(" ").toLowerCase();
      if (isRejectedSourceContext(context)) continue;
      return raw.replace(/[),.;\]]+$/, "");
    }
  }
  return "";
}

function isRejectedSourceContext(context: string): boolean {
  return [
    /404/,
    /403/,
    /실패/,
    /오류/,
    /불충분/,
    /부족/,
    /차단/,
    /저장하지 못/,
    /저장 못/,
    /본문[^.]*충분하지/,
    /본문[^.]*추출[^.]*안/,
    /접근[^.]*안/,
    /접근[^.]*불/,
    /접근[^.]*실패/,
    /not accessible/,
    /inaccessible/,
    /blocked/,
    /failed/,
    /failure/,
    /insufficient/,
    /could not/,
    /unable/,
  ].some((pattern) => pattern.test(context));
}

function statusKey(status: FactStatus): string {
  switch (status) {
    case "verified": return "factBook.status.verified";
    case "intentional_fiction": return "factBook.status.intentionalFiction";
    case "stale": return "factBook.status.stale";
    default: return "factBook.status.uncertain";
  }
}

function extractFactCardProposal(text: string, runId = "fact-book-inline-apply-ops"): CompanionProposal | null {
  const proposal = extractApplyOpsProposal(text, runId);
  if (!proposal?.ops?.length) return null;
  if (!proposal.ops.every((op) => op.op === "create_fact_card")) return null;
  return proposal;
}

export function FactBookPanel({ projectId, nodeId, sceneLabel, selectedClaimRequest, beforeReview, onClose, onChanged }: Props) {
  const { t } = useI18n();
  const [cards, setCards] = useState<FactCard[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reviewing, setReviewing] = useState(false);
  const [directSaving, setDirectSaving] = useState(false);
  const [replyDraft, setReplyDraft] = useState("");
  const [feedbackAnchor, setFeedbackAnchor] = useState<number | null>(null);
  const [feedbackNote, setFeedbackNote] = useState("");
  const [feedbackKind, setFeedbackKind] = useState<"ok" | "error">("ok");
  const [directFactClaim, setDirectFactClaim] = useState("");
  const [sourceRetry, setSourceRetry] = useState<{ claim: string; url: string; error: string } | null>(null);
  const [awaitingFactSave, setAwaitingFactSave] = useState(false);
  const nodeIdRef = useRef<string | null>(nodeId);
  const replyInputRef = useRef<HTMLTextAreaElement | null>(null);
  const handledSelectedClaimRequestRef = useRef<string | null>(null);

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

  const handleApplied = useCallback(() => {
    setAwaitingFactSave(false);
    setFeedbackKind("ok");
    setFeedbackNote(t("factBook.applied"));
    void loadFacts();
    onChanged?.();
  }, [loadFacts, onChanged, t]);

  const { messages, streaming, thinking, status, send } = useCompanion(projectId, nodeIdRef, handleApplied);
  const busy = status === "streaming";

  useEffect(() => { void loadFacts(); }, [loadFacts]);

  const saveDirectURL = useCallback(async (claim: string, url: string, startMessage?: string) => {
    setDirectSaving(true);
    setSourceRetry(null);
    if (startMessage) {
      setFeedbackKind("ok");
      setFeedbackNote(startMessage);
    }
    try {
      await factsApi.createFromUrl({ project_id: projectId, node_id: nodeId, claim, url });
      setReplyDraft("");
      setDirectFactClaim("");
      setAwaitingFactSave(false);
      setSourceRetry(null);
      setFeedbackKind("ok");
      setFeedbackNote(t("factBook.directSaved"));
      await loadFacts();
      onChanged?.();
    } catch (err) {
      const error = String(err);
      setFeedbackKind("error");
      setFeedbackNote(t("factBook.directSaveFailed", { error }));
      setSourceRetry({ claim, url, error });
    } finally {
      setDirectSaving(false);
    }
  }, [loadFacts, nodeId, onChanged, projectId, t]);

  const latestAssistantInfo = useMemo(() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].role === "assistant") return { index: i, message: messages[i] };
    }
    return null;
  }, [messages]);
  const latestAssistant = latestAssistantInfo?.message ?? null;
  const displayStreaming = stripProposalBlock(streaming);
  const latestAssistantDisplayContent = stripProposalBlock(latestAssistant?.content ?? "");
  const inlineFactProposal = useMemo(() => {
    if (!latestAssistant || latestAssistant.proposal) return null;
    return extractFactCardProposal(latestAssistant.content ?? "", latestAssistant.choices?.run_id);
  }, [latestAssistant]);
  const latestProposal = latestAssistant?.proposal ?? inlineFactProposal ?? undefined;
  const savedClaims = useMemo(() => new Set(cards.map((card) => normalizeClaim(card.claim))), [cards]);
  const latestChoices = useMemo(() => {
    if (!latestAssistant?.choices) return null;
    return {
      ...latestAssistant.choices,
      options: latestAssistant.choices.options.filter((option) => !savedClaims.has(normalizeClaim(claimFromChoice(option)))),
    };
  }, [latestAssistant, savedClaims]);
  const hasLatestChoices = Boolean(latestChoices && (latestChoices.options.length > 0 || latestChoices.allow_custom));
  const isNewFeedback = feedbackAnchor !== null && latestAssistantInfo !== null && latestAssistantInfo.index > feedbackAnchor;
  const showAssistantContent = Boolean(
    latestAssistantDisplayContent &&
    (isNewFeedback || latestAssistant?.errored || latestAssistant?.choices || latestProposal),
  );
  const showCompanionFeedback = Boolean(
    busy || streaming || feedbackNote || showAssistantContent || hasLatestChoices || latestProposal,
  );

  useEffect(() => {
    if (!awaitingFactSave || busy || directSaving || feedbackNote || !isNewFeedback || !latestAssistant) return;
    if (latestProposal || latestAssistant.choices) return;
    const url = sourceURLFromAssistant(latestAssistant.content ?? "");
    const claim = directFactClaim.trim();
    if (url && claim) {
      setAwaitingFactSave(false);
      void saveDirectURL(claim, url, t("factBook.autoSaving"));
      return;
    }
    setFeedbackKind("error");
    setFeedbackNote(t("factBook.saveNotApplied"));
  }, [awaitingFactSave, busy, directFactClaim, directSaving, feedbackNote, isNewFeedback, latestAssistant, latestProposal, saveDirectURL, t]);

  const markFeedbackStart = () => {
    setFeedbackAnchor(messages.length);
    setFeedbackNote("");
    setFeedbackKind("ok");
  };

  const startReview = async () => {
    if (reviewing || status === "streaming") return;
    setDirectFactClaim("");
    setSourceRetry(null);
    setAwaitingFactSave(false);
    markFeedbackStart();
    setReviewing(true);
    try {
      await beforeReview?.();
      await send(buildReviewPrompt(sceneLabel));
    } finally {
      setReviewing(false);
    }
  };

  useEffect(() => {
    if (!selectedClaimRequest || handledSelectedClaimRequestRef.current === selectedClaimRequest.id) return;
    if (busy || directSaving) return;
    const claim = selectedClaimRequest.claim.trim();
    if (!claim) return;
    handledSelectedClaimRequestRef.current = selectedClaimRequest.id;
    setDirectFactClaim(claim);
    setSourceRetry(null);
    setAwaitingFactSave(true);
    markFeedbackStart();
    void (async () => {
      await beforeReview?.();
      await send(buildFactCheckPrompt(claim));
    })();
  }, [beforeReview, busy, directSaving, selectedClaimRequest, send]);

  const submitReply = async () => {
    const text = replyDraft.trim();
    if (!text || busy || directSaving) return;
    const url = firstURL(text);
    markFeedbackStart();
    if (url) {
      const claim = directFactClaim.trim();
      if (!claim) {
        setFeedbackKind("error");
        setFeedbackNote(t("factBook.directNeedsClaim"));
        return;
      }
      await saveDirectURL(claim, url);
      return;
    }
    setReplyDraft("");
    setSourceRetry(null);
    await send(text);
  };

  const pickChoice = (text: string) => {
    const claim = claimFromChoice(text);
    setDirectFactClaim(claim);
    setSourceRetry(null);
    setAwaitingFactSave(Boolean(claim));
    markFeedbackStart();
    void send(claim ? buildFactCheckPrompt(claim) : text);
  };

  const retryAlternativeSource = () => {
    if (!sourceRetry || busy || directSaving) return;
    const retry = sourceRetry;
    setDirectFactClaim(retry.claim);
    setAwaitingFactSave(true);
    setReplyDraft("");
    setSourceRetry(null);
    markFeedbackStart();
    setFeedbackKind("ok");
    setFeedbackNote(t("factBook.retryingSource"));
    void send(buildAlternativeSourcePrompt(retry.claim, retry.url, retry.error));
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
        <button type="button" className="btn accent sm" onClick={startReview} disabled={reviewing || busy || directSaving}>
          <Search size={14} /> {reviewing || busy || directSaving ? t("factBook.reviewing") : t("factBook.reviewScene")}
        </button>
        <p>{t("factBook.reviewHint")}</p>
      </div>

      {showCompanionFeedback && (
        <section className="fact-companion-box">
          {status === "streaming" && <div className="companion-thinking"><span className="ai-working-dot" aria-hidden="true" /> {thinking || t("companion.thinking")}</div>}
          {displayStreaming && <div className="fact-companion-prose"><Markdown text={displayStreaming} /></div>}
          {feedbackNote && <div className={`fact-feedback ${feedbackKind}`}>{feedbackNote}</div>}
          {sourceRetry && feedbackKind === "error" && !busy && !directSaving && (
            <div className="fact-feedback-actions">
              <button type="button" className="fact-feedback-action" onClick={retryAlternativeSource}>
                <Search size={13} /> {t("factBook.findAlternativeSource")}
              </button>
            </div>
          )}
          {showAssistantContent && (
            <div className={`fact-companion-prose${latestAssistant?.errored ? " errored" : ""}`}>
              {latestAssistant?.errored ? latestAssistantDisplayContent : <Markdown text={latestAssistantDisplayContent} />}
            </div>
          )}
          {latestProposal && (
            <ProposalCard proposal={latestProposal} projectId={projectId} nodeIdRef={nodeIdRef} onApplied={handleApplied} />
          )}
          {hasLatestChoices && latestChoices && (
            <ChoiceCard
              choices={latestChoices}
              disabled={busy || directSaving}
              lockAfterPick={false}
              onPick={pickChoice}
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
          disabled={busy || directSaving}
          onChange={(e) => setReplyDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void submitReply();
            }
          }}
        />
        <button type="submit" className="fact-reply-send" disabled={!replyDraft.trim() || busy || directSaving} aria-label={t("companion.send")}>
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
