import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  Archive,
  Book,
  Check,
  Copy,
  CornerDownLeft,
  HelpCircle,
  ImagePlus,
  Layers,
  Lightbulb,
  MessageSquare,
  Pencil,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { useCompanion, type ChatMessage } from "../../hooks/useCompanion";
import { useSmoothStream } from "../../hooks/useSmoothStream";
import { companion as companionApi } from "../../lib/rpc";
import type { AIContextPreview, AIContextSelection, CompanionImageAttachment } from "../../lib/types";
import { AIContextChecklistList, DEFAULT_AI_CONTEXT_SELECTION, totalContextItems } from "../ai/AIContextChecklist";
import { ProposalCard } from "./ProposalCard";
import { ChoiceCard } from "./ChoiceCard";
import { Markdown } from "./Markdown";
import { useI18n } from "../../lib/i18n";
import "./CompanionPanel.css";

interface Props {
  projectId: string;
  nodeIdRef: { current: string | null };
  onClose: () => void;
  onApplied: () => void;
  beforeSend?: () => Promise<void> | void;
}

// hide proposal/query blocks (even partial/unclosed) from the live stream preview,
// cutting at whichever fence appears first.
function streamProse(text: string): string {
  let idx = -1;
  for (const fence of ["```linetta-proposal", "```linetta-query", "```linetta-choices"]) {
    const i = text.indexOf(fence);
    if (i >= 0 && (idx < 0 || i < idx)) idx = i;
  }
  return (idx >= 0 ? text.slice(0, idx) : text).trimEnd();
}

type Translate = ReturnType<typeof useI18n>["t"];

const PROMPT_EXAMPLE_KEYS = [
  "companion.example.conflict",
  "companion.example.outline",
  "companion.example.search",
  "companion.example.fetch",
  "companion.example.apply",
] as const;

const EMPTY_CONTEXT_PREVIEW: AIContextPreview = {
  counts: {
    nearbyScenes: 0,
    hasOutline: false,
    hasSynopsis: false,
    relatedScenes: 0,
    entities: 0,
    relationships: 0,
    plotBeats: 0,
    notes: 0,
    projectMetaFields: 0,
    hasStyleNotes: false,
  },
  sections: [],
  selectedItemCount: 0,
};

const MAX_COMPANION_IMAGES = 4;
const MAX_COMPANION_IMAGE_BYTES = 8 * 1024 * 1024;
const SUPPORTED_IMAGE_TYPES = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);

type CompanionImageDraft = CompanionImageAttachment & {
  id: string;
  previewUrl: string;
};

function readAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error ?? new Error("failed to read image"));
    reader.readAsDataURL(file);
  });
}

async function fileToImageDraft(file: File): Promise<CompanionImageDraft> {
  const previewUrl = await readAsDataURL(file);
  const comma = previewUrl.indexOf(",");
  const meta = comma >= 0 ? previewUrl.slice(0, comma) : "";
  const data = comma >= 0 ? previewUrl.slice(comma + 1) : previewUrl;
  const mediaType = /^data:([^;]+)/.exec(meta)?.[1] || file.type || "image/png";
  return {
    id: `${file.name}-${file.size}-${file.lastModified}-${Math.random().toString(36).slice(2)}`,
    name: file.name || "image.png",
    media_type: mediaType,
    data,
    size: file.size,
    previewUrl,
  };
}

function toWireImageAttachments(images: CompanionImageDraft[]): CompanionImageAttachment[] {
  return images.map(({ id: _id, previewUrl: _previewUrl, ...image }) => image);
}

function formatAttachmentSize(size?: number): string {
  if (!size) return "";
  if (size < 1024) return `${size}B`;
  if (size < 1024 * 1024) return `${Math.round(size / 1024)}KB`;
  return `${(size / 1024 / 1024).toFixed(1)}MB`;
}

function CompanionEmpty({
  t,
  onPick,
}: {
  t: Translate;
  onPick: (prompt: string) => void;
}) {
  return (
    <section className="companion-empty-card" aria-label={t("companion.examplesLabel")}>
      <div className="companion-empty-kicker">
        <Lightbulb size={14} />
        <span>{t("companion.examplesTitle")}</span>
      </div>
      <h3>{t("companion.emptyTitle")}</h3>
      <p>{t("companion.empty")}</p>
      <div className="companion-example-list">
        {PROMPT_EXAMPLE_KEYS.map((key) => {
          const prompt = t(key);
          return (
            <button
              key={key}
              type="button"
              className="companion-example"
              onClick={() => onPick(prompt)}
            >
              <Lightbulb size={13} />
              <span>{prompt}</span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

function CompanionHelp({ t }: { t: Translate }) {
  return (
    <section className="companion-help-card" aria-label={t("companion.help")}>
      <div className="companion-help-title">
        <HelpCircle size={15} />
        <span>{t("companion.helpTitle")}</span>
      </div>
      <p>{t("companion.helpBody")}</p>
      <div className="companion-tool-list">
        <div className="companion-tool-row">
          <Search size={14} />
          <span>{t("companion.tool.webSearch")}</span>
        </div>
        <div className="companion-tool-row">
          <Book size={14} />
          <span>{t("companion.tool.webFetch")}</span>
        </div>
        <div className="companion-tool-row">
          <Pencil size={14} />
          <span>{t("companion.tool.applyOps")}</span>
        </div>
      </div>
    </section>
  );
}

function formatTranscript(messages: ChatMessage[], liveProse: string, t: Translate): string {
  const rows = messages.map((m) => `${m.role === "user" ? t("companion.transcript.user") : t("companion.transcript.assistant")}:\n${m.content.trim()}`);
  const live = liveProse.trim();
  if (live) rows.push(`${t("companion.transcript.assistant")}:\n${live}`);
  return rows.filter((row) => row.trim()).join("\n\n");
}

function copyLabelSnippet(text: string): string {
  const trimmed = text.trim().replace(/\s+/g, " ");
  const runes = Array.from(trimmed);
  if (runes.length <= 80) return trimmed;
  return `${runes.slice(0, 80).join("")}…`;
}

function MessageCopyButton({
  text,
  copied,
  onCopy,
  t,
}: {
  text: string;
  copied: boolean;
  onCopy: () => void;
  t: Translate;
}) {
  const label = t("companion.copyMessage", { text: copyLabelSnippet(text) });
  return (
    <button type="button" className="msg-copy" onClick={onCopy} aria-label={label} title={label}>
      {copied ? <Check size={13} /> : <Copy size={13} />}
    </button>
  );
}

export function CompanionPanel({ projectId, nodeIdRef, onClose, onApplied, beforeSend }: Props) {
  const { t } = useI18n();
  const [contextSelection, setContextSelection] = useState<AIContextSelection>(DEFAULT_AI_CONTEXT_SELECTION);
  const { messages, streaming, thinking, reasoning, status, send, cancel, clear, compact } = useCompanion(projectId, nodeIdRef, onApplied, contextSelection);
  const [draft, setDraft] = useState("");
  const [flushing, setFlushing] = useState(false);
  const [copied, setCopied] = useState(false);
  const [copiedMessageKey, setCopiedMessageKey] = useState<string | null>(null);
  const [showHelp, setShowHelp] = useState(false);
  const [showContext, setShowContext] = useState(false);
  const [contextPreview, setContextPreview] = useState<AIContextPreview>(EMPTY_CONTEXT_PREVIEW);
  const [contextLoading, setContextLoading] = useState(false);
  const [attachments, setAttachments] = useState<CompanionImageDraft[]>([]);
  const [attachmentNotice, setAttachmentNotice] = useState("");
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const contextReqIdRef = useRef(0);
  const loadedContextSelectionRef = useRef<AIContextSelection | null>(null);
  const focusInput = () => inputRef.current?.focus();

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [messages, streaming]);

  useLayoutEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    input.style.height = "auto";
    input.style.height = `${input.scrollHeight}px`;
  }, [draft]);

  const loadContextPreview = useCallback(async (selection: AIContextSelection, flushEditor = false) => {
    const reqId = ++contextReqIdRef.current;
    setContextLoading(true);
    try {
      if (flushEditor) await beforeSend?.();
      const preview = await companionApi.previewContext(projectId, nodeIdRef.current ?? "", { context: selection });
      if (reqId === contextReqIdRef.current) {
        setContextPreview(preview);
      }
    } catch {
      if (reqId === contextReqIdRef.current) {
        setContextPreview(EMPTY_CONTEXT_PREVIEW);
      }
    } finally {
      if (reqId === contextReqIdRef.current) {
        setContextLoading(false);
      }
    }
  }, [beforeSend, nodeIdRef, projectId]);

  useEffect(() => {
    if (!showContext) return;
    if (loadedContextSelectionRef.current === contextSelection) return;
    loadedContextSelectionRef.current = contextSelection;
    void loadContextPreview(contextSelection);
  }, [contextSelection, loadContextPreview, showContext]);

  const toggleContext = () => {
    setShowContext((open) => {
      const next = !open;
      if (next) {
        loadedContextSelectionRef.current = contextSelection;
        void loadContextPreview(contextSelection, true);
      }
      return next;
    });
  };

  const addImageFiles = useCallback(async (files: File[]) => {
    const usable = files.filter((file) => SUPPORTED_IMAGE_TYPES.has(file.type) && file.size <= MAX_COMPANION_IMAGE_BYTES);
    if (usable.length === 0) {
      if (files.length > 0) setAttachmentNotice(t("companion.attachUnsupported"));
      return;
    }
    const slots = MAX_COMPANION_IMAGES - attachments.length;
    if (slots <= 0) {
      setAttachmentNotice(t("companion.attachLimit"));
      return;
    }
    const selected = usable.slice(0, slots);
    if (usable.length > selected.length) {
      setAttachmentNotice(t("companion.attachLimit"));
    } else if (files.length > usable.length) {
      setAttachmentNotice(t("companion.attachUnsupported"));
    } else {
      setAttachmentNotice("");
    }
    const drafts = await Promise.all(selected.map(fileToImageDraft));
    setAttachments((prev) => [...prev, ...drafts].slice(0, MAX_COMPANION_IMAGES));
  }, [attachments.length, t]);

  const removeAttachment = (id: string) => {
    setAttachments((prev) => prev.filter((item) => item.id !== id));
    setAttachmentNotice("");
  };

  const sendWithFreshContext = async (text: string, imageDrafts: CompanionImageDraft[] = []) => {
    const trimmed = text.trim();
    if ((!trimmed && imageDrafts.length === 0) || flushing) return false;
    setFlushing(true);
    try {
      await beforeSend?.();
      const sendText = trimmed || t("companion.defaultImagePrompt");
      const images = toWireImageAttachments(imageDrafts);
      if (images.length > 0) {
        await send(sendText, images);
      } else {
        await send(sendText);
      }
      return true;
    } catch {
      return false;
    } finally {
      setFlushing(false);
    }
  };

  const submit = () => {
    const text = draft;
    const imageDrafts = attachments;
    if (!text.trim() && imageDrafts.length === 0) return;
    void sendWithFreshContext(text, imageDrafts).then((sent) => {
      if (sent) {
        setDraft("");
        setAttachments([]);
        setAttachmentNotice("");
      }
    });
  };

  const isStreaming = status === "streaming";
  const isBusy = isStreaming || flushing;
  // Smooth out chunky/bursty provider deltas so the prose reveals evenly
  // instead of jumping. The completed message still uses the full text.
  const smoothStreaming = useSmoothStream(streaming, isStreaming);
  const liveProse = streamProse(smoothStreaming);
  const hasTranscript = messages.length > 0 || liveProse.trim().length > 0;
  const contextItemCount = totalContextItems(contextPreview, contextSelection);

  const copyTranscript = async () => {
    const text = formatTranscript(messages, liveProse, t);
    if (!text) return;
    await navigator.clipboard.writeText(text);
    setCopied(true);
  };

  const copyMessage = async (key: string, text: string) => {
    const trimmed = text.trim();
    if (!trimmed) return;
    await navigator.clipboard.writeText(trimmed);
    setCopiedMessageKey(key);
  };

  const pickExample = (prompt: string) => {
    setDraft(prompt);
    window.requestAnimationFrame(() => focusInput());
  };

  return (
    <aside className="panel" onMouseDown={(e) => e.stopPropagation()}>
      <div className="panel-head">
        <span className="ttl"><span className="ic"><MessageSquare size={16} /></span> {t("companion.title")}</span>
        <div className="companion-head-actions">
          <button
            type="button"
            className="panel-close companion-action"
            onClick={copyTranscript}
            disabled={!hasTranscript}
            aria-label={t("companion.copy")}
            title={t("companion.copy")}
          >
            {copied ? <Check size={15} /> : <Copy size={15} />}
          </button>
          <button
            type="button"
            className="panel-close companion-action"
            onClick={() => { void compact(); }}
            disabled={!hasTranscript || isStreaming}
            aria-label={t("companion.compact")}
            title={t("companion.compact")}
          >
            <Archive size={15} />
          </button>
          <button
            type="button"
            className="panel-close companion-action"
            onClick={() => { void clear(); }}
            disabled={!hasTranscript || isStreaming}
            aria-label={t("companion.clear")}
            title={t("companion.clear")}
          >
            <Trash2 size={15} />
          </button>
          <button
            type="button"
            className={`panel-close companion-action${showHelp ? " is-active" : ""}`}
            onClick={() => setShowHelp((v) => !v)}
            aria-label={t("companion.help")}
            aria-pressed={showHelp}
            title={t("companion.help")}
          >
            <HelpCircle size={15} />
          </button>
          <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}><X size={16} /></button>
        </div>
      </div>

      <div className="panel-scroll cmp-stream" ref={scrollRef}>
        {showHelp && <CompanionHelp t={t} />}
        {messages.length === 0 && (
          <CompanionEmpty t={t} onPick={pickExample} />
        )}
        {messages.map((m, i) => {
          const isUser = m.role === "user";
          const messageKey = `${m.role}-${i}`;
          if (isUser) {
            return (
              <div key={i} className="msg user">
                <div className="msg-line">
                  <MessageCopyButton
                    text={m.content}
                    copied={copiedMessageKey === messageKey}
                    onCopy={() => { void copyMessage(messageKey, m.content); }}
                    t={t}
                  />
                  <div className={`msg-bubble${m.errored ? " errored" : ""}`}>{m.content}</div>
                </div>
              </div>
            );
          }
          const hasCard = !!m.proposal || !!m.choices;
          return (
            <div key={i} className="msg bot">
              {(m.content || !hasCard) && (
                <>
                  <span className="msg-who">companion</span>
                  <div className="msg-line">
                    <div className={`msg-bubble${m.errored ? " errored" : ""}`}>
                      {m.errored ? m.content : <Markdown text={m.content} />}
                    </div>
                    <MessageCopyButton
                      text={m.content}
                      copied={copiedMessageKey === messageKey}
                      onCopy={() => { void copyMessage(messageKey, m.content); }}
                      t={t}
                    />
                  </div>
                </>
              )}
              {m.proposal && (
                <ProposalCard proposal={m.proposal} projectId={projectId} nodeIdRef={nodeIdRef} onApplied={onApplied} />
              )}
              {m.choices && (
                <ChoiceCard choices={m.choices} disabled={isBusy} onPick={(text) => { void sendWithFreshContext(text); }} onCustom={focusInput} />
              )}
            </div>
          );
        })}
        {isStreaming && (
          <div className="msg bot">
            <span className="msg-who">companion</span>
            <div className="companion-thinking">
              <span className="ai-working-dot" aria-hidden="true" />
              {thinking || (liveProse ? t("companion.writing") : t("companion.thinking"))}
            </div>
            {reasoning && (
              <details className="companion-reasoning">
                <summary>{t("companion.reasoning")}</summary>
                <div className="companion-reasoning-body">{reasoning}</div>
              </details>
            )}
            <div className="msg-bubble">{liveProse ? <Markdown text={liveProse} /> : <span className="ai-cursor">&nbsp;</span>}</div>
          </div>
        )}
      </div>

      <div className="cmp-input-wrap">
        {attachments.length > 0 && (
          <div className="companion-attachments" aria-label={t("companion.attachments")}>
            {attachments.map((item) => (
              <div key={item.id} className="companion-attachment">
                <img src={item.previewUrl} alt="" />
                <span className="companion-attachment-name" title={item.name}>{item.name}</span>
                <span className="companion-attachment-size">{formatAttachmentSize(item.size)}</span>
                <button
                  type="button"
                  className="companion-attachment-remove"
                  onClick={() => removeAttachment(item.id)}
                  aria-label={t("companion.removeImage", { name: item.name })}
                  title={t("companion.removeImage", { name: item.name })}
                >
                  <X size={13} />
                </button>
              </div>
            ))}
          </div>
        )}
        {attachmentNotice && <div className="companion-attachment-notice" aria-live="polite">{attachmentNotice}</div>}
        <div className="cmp-input">
          <textarea
            ref={inputRef}
            value={draft}
            placeholder={t("companion.placeholder")}
            rows={1}
            onChange={(e) => setDraft(e.target.value)}
            onPaste={(e) => {
              const files = Array.from(e.clipboardData.items)
                .filter((item) => item.kind === "file" && item.type.startsWith("image/"))
                .map((item) => item.getAsFile())
                .filter((file): file is File => !!file);
              if (files.length > 0) {
                e.preventDefault();
                void addImageFiles(files);
              }
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); submit(); }
            }}
          />
          {isStreaming ? (
            <button type="button" className="cmp-send cmp-stop" onClick={cancel} aria-label={t("companion.stop")}>{t("companion.stop")}</button>
          ) : (
            <button type="button" className="cmp-send" onClick={submit} disabled={(!draft.trim() && attachments.length === 0) || flushing} aria-label={t("companion.send")}>
              <CornerDownLeft size={16} />
            </button>
          )}
        </div>
        <div className="cmp-hint companion-input-toolbar">
          <input
            ref={fileInputRef}
            type="file"
            accept="image/png,image/jpeg,image/webp,image/gif"
            multiple
            className="companion-file-input"
            aria-label={t("companion.attachImage")}
            onChange={(e) => {
              const files = Array.from(e.currentTarget.files ?? []);
              e.currentTarget.value = "";
              void addImageFiles(files);
            }}
          />
          <button
            type="button"
            className="chip companion-attach-chip"
            onClick={() => fileInputRef.current?.click()}
            disabled={isBusy || attachments.length >= MAX_COMPANION_IMAGES}
            aria-label={t("companion.attachImageButton")}
            title={t("companion.attachImageButton")}
          >
            <ImagePlus size={13} />
          </button>
          <span>web_search</span>
          <span>web_fetch</span>
          <span>linetta_apply_ops</span>
          <button
            type="button"
            className={`chip ctx companion-context-chip${showContext ? " on" : ""}`}
            onClick={toggleContext}
            aria-label={t("companion.context")}
            aria-pressed={showContext}
            title={t("companion.context")}
          >
            <Layers size={13} />
            ctx {contextLoading ? "…" : (contextPreview.sections.length > 0 ? contextItemCount : "")}
          </button>
        </div>
        {showContext && (
          <section className="companion-context-card" aria-label={t("companion.context")}>
            <div className="companion-context-title">
              <Layers size={15} />
              <span>{t("companion.contextTitle")}</span>
              <span className="companion-context-count">{contextItemCount}</span>
            </div>
            {contextLoading ? (
              <div className="companion-context-loading">
                <span className="ai-working-dot" aria-hidden="true" />
                {t("companion.contextLoading")}
              </div>
            ) : (
              <AIContextChecklistList
                preview={contextPreview}
                selection={contextSelection}
                onSelectionChange={setContextSelection}
                disabled={isBusy}
              />
            )}
          </section>
        )}
      </div>
    </aside>
  );
}
