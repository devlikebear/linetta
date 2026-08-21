import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import { readTextFile } from "@tauri-apps/plugin-fs";
import {
  Archive,
  Book,
  Check,
  Clipboard,
  Copy,
  CornerDownLeft,
  FileText,
  HelpCircle,
  ImagePlus,
  KeyRound,
  Layers,
  Lightbulb,
  MessageSquare,
  Pencil,
  Search,
  Trash2,
  type LucideIcon,
  X,
} from "lucide-react";
import { useCompanion, type ChatMessage } from "../../hooks/useCompanion";
import { useSmoothStream } from "../../hooks/useSmoothStream";
import { companion as companionApi, openRouter as openRouterApi, providers as providersApi, settings as settingsApi } from "../../lib/rpc";
import { stripProposalBlock } from "../../lib/companionDisplay";
import type {
  AIContextPreview,
  AIContextSelection,
  CompanionHistoryScope,
  CompanionImageAttachment,
  CompanionReference,
  CompanionReferencePurpose,
  CompanionReferenceSource,
  CompanionReferenceStatus,
  AISetupIssue,
  OpenRouterKeyInfo,
  ProviderConfig,
  ProviderID,
} from "../../lib/types";
import { AIDraftComposer, type AIDraftComposerProps } from "../ai/AIPanel";
import { AIContextChecklistList, DEFAULT_AI_CONTEXT_SELECTION, formatTokenEstimate, totalContextItems, totalContextTokens } from "../ai/AIContextChecklist";
import { AISetupStart, guideForProvider, type GuideID } from "../ai/AISetupStart";
import { ProposalCard } from "./ProposalCard";
import { ChoiceCard } from "./ChoiceCard";
import { Markdown } from "./Markdown";
import { localeForLanguage, useI18n, type MessageKey } from "../../lib/i18n";
import { dispatchAppEvent } from "../../lib/appEvents";
import { keyStoreLabelKey } from "../../lib/secretStore";
import {
  OPENROUTER_DEFAULT_MODEL_OPTIONS,
  OPENROUTER_SMART_DEFAULT_MODEL,
  organizeOpenRouterModelOptions,
} from "../../lib/openRouterDefaults";
import "./CompanionPanel.css";

export type SelectionRewriteKind = "rewrite" | "proofread";

interface Props {
  projectId: string;
  nodeIdRef: { current: string | null };
  currentNodeId?: string | null;
  onClose: () => void;
  onApplied: () => void;
  beforeSend?: () => Promise<void> | void;
  outlineStructure?: string;
  aiDraft?: AIDraftComposerProps;
  selectionRewriteRequest?: {
    id: string;
    text: string;
    kind?: SelectionRewriteKind;
  } | null;
}

// hide proposal/query blocks (even partial/unclosed) from the live stream preview,
// cutting at whichever fence appears first.
function streamProse(text: string): string {
  let idx = -1;
  for (const fence of ["```linetta-proposal", "```linetta-query", "```linetta-choices"]) {
    const i = text.indexOf(fence);
    if (i >= 0 && (idx < 0 || i < idx)) idx = i;
  }
  return stripProposalBlock(idx >= 0 ? text.slice(0, idx) : text).trimEnd();
}

type Translate = ReturnType<typeof useI18n>["t"];

interface CompanionActionPreset {
  id: string;
  scope: "scene" | "work";
  icon: LucideIcon;
  labelKey: MessageKey;
  descriptionKey: MessageKey;
  promptKey: MessageKey;
}

const COMPANION_SCENE_ACTIONS: CompanionActionPreset[] = [
  {
    id: "continue-scene",
    scope: "scene",
    icon: Pencil,
    labelKey: "companion.actions.continueScene.label",
    descriptionKey: "companion.actions.continueScene.description",
    promptKey: "companion.actions.continueScene.prompt",
  },
  {
    id: "rewrite-scene",
    scope: "scene",
    icon: FileText,
    labelKey: "companion.actions.rewriteScene.label",
    descriptionKey: "companion.actions.rewriteScene.description",
    promptKey: "companion.actions.rewriteScene.prompt",
  },
  {
    id: "tighten-dialogue",
    scope: "scene",
    icon: MessageSquare,
    labelKey: "companion.actions.tightenDialogue.label",
    descriptionKey: "companion.actions.tightenDialogue.description",
    promptKey: "companion.actions.tightenDialogue.prompt",
  },
  {
    id: "raise-tension",
    scope: "scene",
    icon: Lightbulb,
    labelKey: "companion.actions.raiseTension.label",
    descriptionKey: "companion.actions.raiseTension.description",
    promptKey: "companion.actions.raiseTension.prompt",
  },
  {
    id: "next-episode-hook",
    scope: "scene",
    icon: Book,
    labelKey: "companion.actions.nextEpisodeHook.label",
    descriptionKey: "companion.actions.nextEpisodeHook.description",
    promptKey: "companion.actions.nextEpisodeHook.prompt",
  },
  {
    id: "finish-episode",
    scope: "scene",
    icon: Check,
    labelKey: "companion.actions.finishEpisode.label",
    descriptionKey: "companion.actions.finishEpisode.description",
    promptKey: "companion.actions.finishEpisode.prompt",
  },
];

const COMPANION_WORK_ACTIONS: CompanionActionPreset[] = [
  {
    id: "plot-structure",
    scope: "work",
    icon: Layers,
    labelKey: "companion.actions.plotStructure.label",
    descriptionKey: "companion.actions.plotStructure.description",
    promptKey: "companion.actions.plotStructure.prompt",
  },
  {
    id: "outline-structure",
    scope: "work",
    icon: FileText,
    labelKey: "companion.actions.outlineStructure.label",
    descriptionKey: "companion.actions.outlineStructure.description",
    promptKey: "companion.actions.outlineStructure.prompt",
  },
  {
    id: "check-continuity",
    scope: "work",
    icon: Search,
    labelKey: "companion.actions.checkContinuity.label",
    descriptionKey: "companion.actions.checkContinuity.description",
    promptKey: "companion.actions.checkContinuity.prompt",
  },
  {
    id: "rename-across-work",
    scope: "work",
    icon: Search,
    labelKey: "companion.actions.renameAcrossWork.label",
    descriptionKey: "companion.actions.renameAcrossWork.description",
    promptKey: "companion.actions.renameAcrossWork.prompt",
  },
  {
    id: "setting-impact",
    scope: "work",
    icon: Lightbulb,
    labelKey: "companion.actions.settingImpact.label",
    descriptionKey: "companion.actions.settingImpact.description",
    promptKey: "companion.actions.settingImpact.prompt",
  },
];

function companionActionsForScope(scope: CompanionHistoryScope): CompanionActionPreset[] {
  return scope === "project" ? COMPANION_WORK_ACTIONS : COMPANION_SCENE_ACTIONS;
}

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
  selectedCharCount: 0,
  selectedTokenEstimate: 0,
  budgetTokenEstimate: 0,
};

const MAX_COMPANION_IMAGES = 4;
const MAX_COMPANION_IMAGE_BYTES = 8 * 1024 * 1024;
const SUPPORTED_IMAGE_TYPES = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);
const REFERENCE_PURPOSES: CompanionReferencePurpose[] = ["content", "style", "canon", "constraint"];

type CompanionImageDraft = CompanionImageAttachment & {
  id: string;
  previewUrl: string;
};

type ReferenceScopeDraft = "project" | "scene";

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

function referencePurposeLabel(t: Translate, purpose: CompanionReferencePurpose): string {
  return t(`companion.reference.purpose.${purpose}`);
}

function referenceStatusLabel(t: Translate, status: CompanionReferenceStatus): string {
  return t(`companion.reference.status.${status}`);
}

function referenceScopeOf(ref: CompanionReference): ReferenceScopeDraft {
  return ref.node_id ? "scene" : "project";
}

function companionPendingSteps(t: Translate): string[] {
  return [
    t("companion.preparing.step.intent"),
    t("companion.preparing.step.context"),
    t("companion.preparing.step.draft"),
  ];
}

function contextBudgetLevel(tokens: number): "normal" | "large" | "too-large" {
  if (tokens > 24000) return "too-large";
  if (tokens >= 12000) return "large";
  return "normal";
}

function markdownTitleFromPath(path: string): string {
  const name = path.split(/[\\/]/).filter(Boolean).pop() ?? "reference.md";
  return name.replace(/\.(md|markdown|txt)$/i, "");
}

function CompanionEmpty({
  t,
  scope,
  onPickAction,
}: {
  t: Translate;
  scope: CompanionHistoryScope;
  onPickAction: (action: CompanionActionPreset) => void;
}) {
  const actions = companionActionsForScope(scope);
  const titleKey = scope === "project" ? "companion.actions.workTitle" : "companion.actions.sceneTitle";
  const descriptionKey = scope === "project" ? "companion.actions.workDescription" : "companion.actions.sceneDescription";
  const renderAction = (action: CompanionActionPreset) => {
    const Icon = action.icon;
    return (
      <button
        key={action.id}
        type="button"
        className="companion-action-preset"
        onClick={() => onPickAction(action)}
      >
        <Icon size={14} />
        <span className="companion-action-copy">
          <strong>{t(action.labelKey)}</strong>
          <small>{t(action.descriptionKey)}</small>
        </span>
      </button>
    );
  };

  return (
    <section className="companion-empty-card" aria-label={t("companion.actions.ariaLabel")}>
      <div className="companion-empty-kicker">
        <Pencil size={14} />
        <span>{t("companion.actions.title")}</span>
      </div>
      <h3>{t("companion.emptyTitle")}</h3>
      <p>{t("companion.empty")}</p>
      <div className="companion-action-list">
        <div className="companion-action-section">
          <div className="companion-action-section-head">
            <span>{t(titleKey)}</span>
            <small>{t(descriptionKey)}</small>
          </div>
          <div className="companion-action-grid">
            {actions.map(renderAction)}
          </div>
        </div>
      </div>
    </section>
  );
}

function CompanionCuratedActions({
  t,
  actions,
  onPickAction,
  disabled,
}: {
  t: Translate;
  actions: CompanionActionPreset[];
  onPickAction: (action: CompanionActionPreset) => void;
  disabled: boolean;
}) {
  return (
    <div className="companion-curated-actions" role="group" aria-label={t("companion.actions.curatedLabel")}>
      {actions.map((action) => {
        const Icon = action.icon;
        return (
          <button
            key={action.id}
            type="button"
            className="chip companion-curated-action"
            onClick={() => onPickAction(action)}
            disabled={disabled}
          >
            <Icon size={12} />
            <span>{t(action.labelKey)}</span>
          </button>
        );
      })}
    </div>
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

function selectionRewritePrompt(text: string, kind: SelectionRewriteKind = "rewrite"): string {
  const trimmed = text.trim();
  if (kind === "proofread") {
    return [
      "선택한 문장을 맞춤법·띄어쓰기·조사 오류·비문 중심으로 퇴고해줘.",
      "원문 의미·문체·고유명사·대사 톤은 유지하고, 바꾼 부분의 변경 목록을 함께 제시해줘.",
      "설명으로 끝내지 말고, 현재 씬 본문에서 아래 선택문만 교정되도록 전체 씬 원고를 set_scene_text로 실제 반영해줘.",
      "",
      "선택문:",
      "```",
      trimmed,
      "```",
    ].join("\n");
  }
  return [
    "선택한 문장을 현재 씬의 캐릭터, 플롯, 문체에 맞게 자연스럽게 수정해줘.",
    "설명으로 끝내지 말고, 현재 씬 본문에서 아래 선택문만 바뀌도록 전체 씬 원고를 set_scene_text로 실제 반영해줘.",
    "",
    "선택문:",
    "```",
    trimmed,
    "```",
  ].join("\n");
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

function aiSetupBodyKey(issue: AISetupIssue): MessageKey {
  switch (issue) {
    case "missing_key":
      return "companion.aiSetup.body.missingKey";
    case "auth_required":
      return "companion.aiSetup.body.authRequired";
    case "model_unavailable":
      return "companion.aiSetup.body.modelUnavailable";
    case "rate_or_spend_limit":
      return "companion.aiSetup.body.rateOrSpendLimit";
    case "unknown_provider_error":
    default:
      return "companion.aiSetup.body.unknownProviderError";
  }
}

function CompanionAISetupCard({
  message,
  t,
  isBusy,
  onRetry,
  onOpenSetup,
}: {
  message: ChatMessage;
  t: Translate;
  isBusy: boolean;
  onRetry: () => void;
  onOpenSetup: (path: "easy" | "subscription" | "direct") => void;
}) {
  if (!message.aiSetupIssue) return null;
  return (
    <section className="companion-ai-setup-card" aria-label={t("companion.aiSetup.title")}>
      <div className="companion-ai-setup-title">
        <KeyRound size={15} />
        <strong>{t("companion.aiSetup.title")}</strong>
      </div>
      <p>{t(aiSetupBodyKey(message.aiSetupIssue))}</p>
      <div className="companion-ai-setup-actions">
        <button type="button" className="btn primary sm" onClick={() => onOpenSetup("easy")}>
          {t("companion.aiSetup.connectEasy")}
        </button>
        <button type="button" className="btn ghost sm" onClick={() => onOpenSetup("subscription")}>
          {t("companion.aiSetup.connectSubscription")}
        </button>
        <button type="button" className="btn ghost sm" onClick={() => onOpenSetup("direct")}>
          {t("companion.aiSetup.connectDirect")}
        </button>
        {message.retryText && (
          <button type="button" className="btn ghost sm" onClick={onRetry} disabled={isBusy}>
            {t("companion.aiSetup.retryLast")}
          </button>
        )}
      </div>
      <div className="companion-ai-setup-notes">
        <span>
          {keyStoreLabelKey()
            ? t("companion.aiSetup.keychain", { store: t(keyStoreLabelKey()!) })
            : t("companion.aiSetup.keychainUnsupported")}
        </span>
        <span>{t("companion.aiSetup.limit")}</span>
      </div>
      {message.rawError && (
        <details className="companion-ai-setup-details">
          <summary>{t("companion.aiSetup.details")}</summary>
          <code>{message.rawError}</code>
        </details>
      )}
    </section>
  );
}

export function CompanionPanel({
  projectId,
  nodeIdRef,
  currentNodeId: currentNodeIdProp,
  onClose,
  onApplied,
  beforeSend,
  outlineStructure,
  aiDraft,
  selectionRewriteRequest,
}: Props) {
  const { language, t } = useI18n();
  const [contextSelection, setContextSelection] = useState<AIContextSelection>(DEFAULT_AI_CONTEXT_SELECTION);
  const currentNodeId = currentNodeIdProp ?? nodeIdRef.current;
  const [historyScope, setHistoryScope] = useState<CompanionHistoryScope>(() => currentNodeId ? "scene" : "project");
  const effectiveHistoryScope: CompanionHistoryScope = currentNodeId ? historyScope : "project";
  const { messages, streaming, thinking, reasoning, status, send, cancel, clear, compact } = useCompanion(
    projectId,
    currentNodeId,
    onApplied,
    contextSelection,
    outlineStructure,
    effectiveHistoryScope,
  );
  const [draft, setDraft] = useState("");
  const [flushing, setFlushing] = useState(false);
  const [copied, setCopied] = useState(false);
  const [copiedMessageKey, setCopiedMessageKey] = useState<string | null>(null);
  const [showHelp, setShowHelp] = useState(false);
  const [showContext, setShowContext] = useState(false);
  const [actionTrayTouched, setActionTrayTouched] = useState(false);
  const [contextPreview, setContextPreview] = useState<AIContextPreview>(EMPTY_CONTEXT_PREVIEW);
  const [contextLoading, setContextLoading] = useState(false);
  const [references, setReferences] = useState<CompanionReference[]>([]);
  const [referencesLoading, setReferencesLoading] = useState(false);
  const [referenceDraftOpen, setReferenceDraftOpen] = useState(false);
  const [referenceTitle, setReferenceTitle] = useState("");
  const [referenceText, setReferenceText] = useState("");
  const [referencePurpose, setReferencePurpose] = useState<CompanionReferencePurpose>("content");
  const [referenceSource, setReferenceSource] = useState<CompanionReferenceSource>("text");
  const [referenceScope, setReferenceScope] = useState<ReferenceScopeDraft>(() => currentNodeId ? "scene" : "project");
  const [referenceSaving, setReferenceSaving] = useState(false);
  const [referenceNotice, setReferenceNotice] = useState("");
  const [attachments, setAttachments] = useState<CompanionImageDraft[]>([]);
  const [attachmentNotice, setAttachmentNotice] = useState("");
  const [aiSetupOpen, setAISetupOpen] = useState(false);
  const [aiSetupGuideId, setAISetupGuideId] = useState<GuideID>("chatgpt-subscription");
  const [aiSetupProvider, setAISetupProvider] = useState<ProviderID>("openai-codex");
  const [aiSetupOpenRouterKeyDraft, setAISetupOpenRouterKeyDraft] = useState("");
  const [aiSetupOpenRouterKeySaved, setAISetupOpenRouterKeySaved] = useState(false);
  const [aiSetupOpenRouterModelDraft, setAISetupOpenRouterModelDraft] = useState(OPENROUTER_SMART_DEFAULT_MODEL);
  const [aiSetupOpenRouterModels, setAISetupOpenRouterModels] = useState<string[]>(OPENROUTER_DEFAULT_MODEL_OPTIONS);
  const [aiSetupOpenRouterModelsLoading, setAISetupOpenRouterModelsLoading] = useState(false);
  const [aiSetupOpenRouterModelsError, setAISetupOpenRouterModelsError] = useState("");
  const [aiSetupOpenRouterBusy, setAISetupOpenRouterBusy] = useState(false);
  const [aiSetupOpenRouterMsg, setAISetupOpenRouterMsg] = useState<{ kind: "ok" | "error"; text: string } | null>(null);
  const [aiSetupOpenRouterKeyInfo, setAISetupOpenRouterKeyInfo] = useState<OpenRouterKeyInfo | null>(null);
  const [aiSetupOpenRouterKeyInfoLoading, setAISetupOpenRouterKeyInfoLoading] = useState(false);
  const [aiSetupOpenRouterKeyInfoError, setAISetupOpenRouterKeyInfoError] = useState("");
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const contextReqIdRef = useRef(0);
  const loadedContextSelectionRef = useRef<AIContextSelection | null>(null);
  const lastSelectionRewriteRequestIdRef = useRef<string | null>(null);
  const focusInput = () => inputRef.current?.focus();

  useEffect(() => {
    if (!currentNodeId && historyScope === "scene") {
      setHistoryScope("project");
    }
  }, [currentNodeId, historyScope]);

  useEffect(() => {
    if (!currentNodeId && referenceScope === "scene") {
      setReferenceScope("project");
    }
  }, [currentNodeId, referenceScope]);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [messages, streaming]);

  useLayoutEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    input.style.height = "auto";
    input.style.height = `${input.scrollHeight}px`;
  }, [draft]);

  useEffect(() => {
    if (!selectionRewriteRequest || aiDraft) return;
    if (lastSelectionRewriteRequestIdRef.current === selectionRewriteRequest.id) return;
    lastSelectionRewriteRequestIdRef.current = selectionRewriteRequest.id;
    setDraft(selectionRewritePrompt(selectionRewriteRequest.text, selectionRewriteRequest.kind ?? "rewrite"));
    window.requestAnimationFrame(() => focusInput());
  }, [aiDraft, selectionRewriteRequest]);

  const loadContextPreview = useCallback(async (selection: AIContextSelection, flushEditor = false) => {
    const reqId = ++contextReqIdRef.current;
    setContextLoading(true);
    try {
      if (flushEditor) await beforeSend?.();
      const preview = await companionApi.previewContext(projectId, currentNodeId ?? "", { context: selection });
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
  }, [beforeSend, currentNodeId, projectId]);

  const loadReferences = useCallback(async () => {
    setReferencesLoading(true);
    try {
      const refs = await companionApi.references.list(projectId, currentNodeId ?? null, true);
      setReferences(refs);
      setReferenceNotice("");
    } catch {
      setReferenceNotice(t("companion.reference.loadFailed"));
      setReferences([]);
    } finally {
      setReferencesLoading(false);
    }
  }, [currentNodeId, projectId, t]);

  useEffect(() => {
    if (!showContext) return;
    if (loadedContextSelectionRef.current === contextSelection) return;
    loadedContextSelectionRef.current = contextSelection;
    void loadContextPreview(contextSelection);
  }, [contextSelection, loadContextPreview, showContext]);

  useEffect(() => {
    if (!showContext) return;
    void loadReferences();
  }, [loadReferences, showContext]);

  const toggleContext = () => {
    setShowContext((open) => {
      const next = !open;
      if (next) {
        loadedContextSelectionRef.current = contextSelection;
        void loadContextPreview(contextSelection, true);
        void loadReferences();
      }
      return next;
    });
  };

  const refreshContextAfterReferenceChange = async () => {
    await loadReferences();
    await loadContextPreview(contextSelection);
  };

  const openTextReference = () => {
    setReferenceSource("text");
    setReferencePurpose("content");
    setReferenceTitle("");
    setReferenceText("");
    setReferenceScope(currentNodeId ? "scene" : "project");
    setReferenceDraftOpen(true);
    setReferenceNotice("");
  };

  const openClipboardReference = async () => {
    if (!navigator.clipboard?.readText) {
      setReferenceNotice(t("companion.reference.clipboardUnavailable"));
      return;
    }
    const text = (await navigator.clipboard.readText()).trim();
    if (!text) {
      setReferenceNotice(t("companion.reference.clipboardEmpty"));
      return;
    }
    setReferenceSource("clipboard");
    setReferencePurpose("style");
    setReferenceTitle(t("companion.reference.clipboardTitle"));
    setReferenceText(text);
    setReferenceScope(currentNodeId ? "scene" : "project");
    setReferenceDraftOpen(true);
    setReferenceNotice("");
  };

  const openMarkdownReference = async () => {
    const picked = await openDialog({
      multiple: false,
      filters: [{ name: "Text", extensions: ["md", "markdown", "txt"] }],
    });
    const path = Array.isArray(picked) ? picked[0] : picked;
    if (!path) return;
    const text = (await readTextFile(path)).trim();
    if (!text) {
      setReferenceNotice(t("companion.reference.fileEmpty"));
      return;
    }
    const source: CompanionReferenceSource = /\.(md|markdown)$/i.test(path) ? "markdown" : "file";
    setReferenceSource(source);
    setReferencePurpose("content");
    setReferenceTitle(markdownTitleFromPath(path));
    setReferenceText(text);
    setReferenceScope(currentNodeId ? "scene" : "project");
    setReferenceDraftOpen(true);
    setReferenceNotice("");
  };

  const saveReference = async () => {
    const content = referenceText.trim();
    if (!content || referenceSaving) return;
    setReferenceSaving(true);
    try {
      await companionApi.references.create({
        project_id: projectId,
        ...(referenceScope === "scene" && currentNodeId ? { node_id: currentNodeId } : {}),
        source_type: referenceSource,
        purpose: referencePurpose,
        title: referenceTitle.trim() || undefined,
        content,
      });
      setReferenceDraftOpen(false);
      setReferenceTitle("");
      setReferenceText("");
      await refreshContextAfterReferenceChange();
    } catch {
      setReferenceNotice(t("companion.reference.saveFailed"));
    } finally {
      setReferenceSaving(false);
    }
  };

  const updateReferenceStatus = async (ref: CompanionReference, status: CompanionReferenceStatus) => {
    await companionApi.references.update({ project_id: projectId, id: ref.id, status });
    await refreshContextAfterReferenceChange();
  };

  const summarizeActiveReferences = async () => {
    const targets = references.filter((ref) => ref.status === "active");
    if (targets.length === 0) return;
    await Promise.all(targets.map((ref) => companionApi.references.update({ project_id: projectId, id: ref.id, status: "summarized" })));
    await refreshContextAfterReferenceChange();
  };

  const keepCurrentSceneOnly = () => {
    setContextSelection({
      current_scene: true,
      overview: false,
      synopsis: false,
      nearby_scenes: false,
      related_scenes: false,
      plot: false,
      entities: false,
      relationships: false,
      notes: false,
      project_meta: false,
      style_notes: false,
      facts: false,
      memories: false,
      references: false,
    });
  };

  const deleteReference = async (ref: CompanionReference) => {
    await companionApi.references.delete(projectId, ref.id);
    await refreshContextAfterReferenceChange();
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
  const showPendingPreparation = isStreaming && !thinking && !liveProse;
  const hasTranscript = messages.length > 0 || liveProse.trim().length > 0;
  const contextItemCount = totalContextItems(contextPreview, contextSelection);
  const contextTokenCount = totalContextTokens(contextPreview, contextSelection);
  const budgetLevel = contextBudgetLevel(contextTokenCount);
  const curatedActions = companionActionsForScope(effectiveHistoryScope).slice(0, 3);
  const showCuratedActions = actionTrayTouched || hasTranscript || effectiveHistoryScope === "project";

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

  const pickAction = (action: CompanionActionPreset) => {
    if (action.scope === "work") {
      setHistoryScope("project");
    } else if (currentNodeId) {
      setHistoryScope("scene");
    }
    setActionTrayTouched(true);
    setDraft(t(action.promptKey));
    window.requestAnimationFrame(() => focusInput());
  };

  const openAISetup = (path: "easy" | "subscription" | "direct") => {
    const guideId: GuideID =
      path === "easy" ? "openrouter-safe" : path === "subscription" ? "chatgpt-subscription" : "openai-api";
    const provider: ProviderID =
      path === "easy" ? "openrouter" : path === "subscription" ? "openai-codex" : "openai";
    setAISetupProvider(provider);
    setAISetupGuideId(guideId);
    setAISetupOpen(true);
  };

  const selectAISetupProvider = async (provider: ProviderID) => {
    setAISetupProvider(provider);
    setAISetupGuideId(guideForProvider(provider));
    try {
      const next = await settingsApi.set({ provider });
      dispatchAppEvent("linetta:settings-updated", next);
    } catch {
      // The inline setup sheet is an entry point; Settings remains the detailed
      // surface for showing provider-specific save/test errors.
    }
  };

  const persistAISetupOpenRouter = async (options: { clearAPIKey?: boolean; quiet?: boolean } = {}) => {
    const config: ProviderConfig = {
      model: aiSetupOpenRouterModelDraft.trim() || OPENROUTER_SMART_DEFAULT_MODEL,
    };
    const key = aiSetupOpenRouterKeyDraft.trim();
    if (key !== "") {
      config.api_key = key;
    }
    if (options.clearAPIKey) {
      config.clear_api_key = true;
    }
    const next = await settingsApi.set({
      provider: "openrouter",
      providers: { openrouter: config },
    });
    setAISetupProvider("openrouter");
    setAISetupGuideId("openrouter-safe");
    setAISetupOpenRouterKeyDraft(next.providers?.openrouter?.api_key ?? "");
    setAISetupOpenRouterKeySaved(next.providers?.openrouter?.api_key_set ?? false);
    setAISetupOpenRouterModelDraft(next.providers?.openrouter?.model?.trim() || OPENROUTER_SMART_DEFAULT_MODEL);
    dispatchAppEvent("linetta:settings-updated", next);
    if (!options.quiet) {
      setAISetupOpenRouterMsg({ kind: "ok", text: t("settings.setup.openrouter.saved") });
    }
    return next;
  };

  const refreshAISetupOpenRouterKeyInfo = async () => {
    setAISetupOpenRouterKeyInfoLoading(true);
    setAISetupOpenRouterKeyInfoError("");
    try {
      setAISetupOpenRouterKeyInfo(await openRouterApi.keyInfo());
    } catch (e) {
      setAISetupOpenRouterKeyInfo(null);
      setAISetupOpenRouterKeyInfoError(String(e));
    } finally {
      setAISetupOpenRouterKeyInfoLoading(false);
    }
  };

  const saveAISetupOpenRouter = async () => {
    setAISetupOpenRouterBusy(true);
    setAISetupOpenRouterMsg(null);
    try {
      const next = await persistAISetupOpenRouter();
      if (next.providers?.openrouter?.api_key_set) {
        await refreshAISetupOpenRouterKeyInfo();
      }
    } catch (e) {
      setAISetupOpenRouterMsg({ kind: "error", text: String(e) });
    } finally {
      setAISetupOpenRouterBusy(false);
    }
  };

  const clearAISetupOpenRouterKey = async () => {
    setAISetupOpenRouterBusy(true);
    setAISetupOpenRouterMsg(null);
    try {
      await persistAISetupOpenRouter({ clearAPIKey: true, quiet: true });
      setAISetupOpenRouterKeySaved(false);
      setAISetupOpenRouterKeyInfo(null);
      setAISetupOpenRouterMsg({ kind: "ok", text: t("settings.setup.openrouter.saved") });
    } catch (e) {
      setAISetupOpenRouterMsg({ kind: "error", text: String(e) });
    } finally {
      setAISetupOpenRouterBusy(false);
    }
  };

  const refreshAISetupOpenRouterModels = async () => {
    setAISetupOpenRouterModelsLoading(true);
    setAISetupOpenRouterModelsError("");
    setAISetupOpenRouterMsg(null);
    try {
      await persistAISetupOpenRouter({ quiet: true });
      const res = await providersApi.listModels("openrouter");
      const models = organizeOpenRouterModelOptions(res.models);
      setAISetupOpenRouterModels(models);
      if (!aiSetupOpenRouterModelDraft.trim() && models[0]) {
        setAISetupOpenRouterModelDraft(models[0]);
      }
    } catch (e) {
      setAISetupOpenRouterModelsError(String(e));
    } finally {
      setAISetupOpenRouterModelsLoading(false);
    }
  };

  const testAISetupOpenRouter = async () => {
    setAISetupOpenRouterBusy(true);
    setAISetupOpenRouterMsg(null);
    try {
      await persistAISetupOpenRouter({ quiet: true });
      const res = await providersApi.test("openrouter");
      setAISetupOpenRouterMsg({ kind: "ok", text: t("settings.provider.testOk", { message: res.message }) });
      await refreshAISetupOpenRouterKeyInfo();
    } catch (e) {
      setAISetupOpenRouterMsg({ kind: "error", text: t("settings.provider.testError", { message: String(e) }) });
    } finally {
      setAISetupOpenRouterBusy(false);
    }
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

      {!aiDraft && (
        <div className="companion-scope-tabs" role="group" aria-label={t("companion.scope.label")}>
          <button
            type="button"
            className={effectiveHistoryScope === "scene" ? "is-active" : ""}
            aria-pressed={effectiveHistoryScope === "scene"}
            disabled={!currentNodeId}
            onClick={() => {
              setHistoryScope("scene");
              setActionTrayTouched(true);
            }}
          >
            {t("companion.scope.scene")}
          </button>
          <button
            type="button"
            className={effectiveHistoryScope === "project" ? "is-active" : ""}
            aria-pressed={effectiveHistoryScope === "project"}
            onClick={() => {
              setHistoryScope("project");
              setActionTrayTouched(true);
            }}
          >
            {t("companion.scope.project")}
          </button>
        </div>
      )}

      <div className="panel-scroll cmp-stream" ref={scrollRef}>
        {showHelp && <CompanionHelp t={t} />}
        {aiDraft ? (
          <AIDraftComposer {...aiDraft} />
        ) : messages.length === 0 && (
          <CompanionEmpty t={t} scope={effectiveHistoryScope} onPickAction={pickAction} />
        )}
        {!aiDraft && messages.map((m, i) => {
          const isUser = m.role === "user";
          const messageKey = `${m.role}-${i}`;
          const showSceneMeta = effectiveHistoryScope === "project" && !!m.nodeLabel;
          if (isUser) {
            return (
              <div key={i} className="msg user">
                {showSceneMeta && (
                  <div className="companion-message-meta">
                    <span className="companion-scene-chip">{m.nodeLabel}</span>
                  </div>
                )}
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
          if (m.status === "compacted") {
            return (
              <div key={i} className="companion-compacted-message">
                <Archive size={13} />
                <div>
                  <strong>{t("companion.compactedSummary")}</strong>
                  <Markdown text={m.content} />
                </div>
              </div>
            );
          }
          const hasCard = !!m.proposal || !!m.choices;
          if (m.errored && m.aiSetupIssue) {
            return (
              <div key={i} className="msg bot">
                {showSceneMeta && (
                  <div className="companion-message-meta">
                    <span className="companion-scene-chip">{m.nodeLabel}</span>
                  </div>
                )}
                <span className="msg-who">{t("companion.speaker")}</span>
                <CompanionAISetupCard
                  message={m}
                  t={t}
                  isBusy={isBusy}
                  onOpenSetup={openAISetup}
                  onRetry={() => { void sendWithFreshContext(m.retryText ?? ""); }}
                />
              </div>
            );
          }
          return (
            <div key={i} className="msg bot">
              {(m.content || !hasCard) && (
                <>
                  {showSceneMeta && (
                    <div className="companion-message-meta">
                      <span className="companion-scene-chip">{m.nodeLabel}</span>
                    </div>
                  )}
                  <span className="msg-who">{t("companion.speaker")}</span>
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
                  {m.errored && m.retryText && (
                    <div className="companion-message-actions">
                      <button type="button" className="btn ghost sm" onClick={() => { void sendWithFreshContext(m.retryText ?? ""); }} disabled={isBusy}>
                        {t("companion.retry")}
                      </button>
                    </div>
                  )}
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
        {!aiDraft && isStreaming && (
          <div className="msg bot">
            <span className="msg-who">{t("companion.speaker")}</span>
            <div className="companion-thinking">
              <span className="ai-working-dot" aria-hidden="true" />
              {thinking || (liveProse ? t("companion.writing") : t("companion.preparing"))}
            </div>
            {reasoning && (
              <details className="companion-reasoning">
                <summary>{t("companion.reasoning")}</summary>
                <div className="companion-reasoning-body">{reasoning}</div>
              </details>
            )}
            {showPendingPreparation ? (
              <div className="companion-preparing" aria-label={t("companion.preparing")}>
                {companionPendingSteps(t).map((step) => (
                  <span key={step}>{step}</span>
                ))}
              </div>
            ) : (
              <div className="msg-bubble">{liveProse ? <Markdown text={liveProse} /> : <span className="ai-cursor">&nbsp;</span>}</div>
            )}
          </div>
        )}
      </div>

      {!aiDraft && (
      <div className="cmp-input-wrap">
        {showCuratedActions && (
          <CompanionCuratedActions
            t={t}
            actions={curatedActions}
            onPickAction={pickAction}
            disabled={isBusy}
          />
        )}
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
          <span className="chip companion-tool-chip" title={t("companion.tool.webSearch")}>{t("companion.toolChip.webSearch")}</span>
          <span className="chip companion-tool-chip" title={t("companion.tool.webFetch")}>{t("companion.toolChip.webFetch")}</span>
          <span className="chip companion-tool-chip" title={t("companion.tool.applyOps")}>{t("companion.toolChip.applyOps")}</span>
          <button
            type="button"
            className={`chip ctx companion-context-chip${showContext ? " on" : ""}`}
            onClick={toggleContext}
            aria-label={t("companion.context")}
            aria-pressed={showContext}
            title={t("companion.context")}
          >
            <Layers size={13} />
            ctx {contextLoading ? "…" : (contextTokenCount > 0 ? formatTokenEstimate(contextTokenCount) : "")}
          </button>
        </div>
        {showContext && (
          <section className="companion-context-card" aria-label={t("companion.context")}>
            <div className="companion-context-title">
              <Layers size={15} />
              <span>{t("companion.contextTitle")}</span>
              <span className="companion-context-count">{formatTokenEstimate(contextTokenCount)}</span>
            </div>
            <div className={`companion-context-budget ${budgetLevel}`}>
              <span>{t("companion.contextBudget", { tokens: formatTokenEstimate(contextTokenCount) })}</span>
              <span>{t("companion.contextItems", { count: contextItemCount })}</span>
            </div>
            {budgetLevel !== "normal" && (
              <div className="companion-context-quick-actions">
                <button type="button" className="btn ghost sm" onClick={keepCurrentSceneOnly}>
                  {t("companion.contextQuick.currentScene")}
                </button>
                <button
                  type="button"
                  className="btn ghost sm"
                  onClick={() => { void summarizeActiveReferences(); }}
                  disabled={references.every((ref) => ref.status !== "active")}
                >
                  {t("companion.contextQuick.summarizeRefs")}
                </button>
                <button type="button" className="btn ghost sm" onClick={() => { void compact(); }} disabled={!hasTranscript || isStreaming}>
                  {t("companion.contextQuick.compactChat")}
                </button>
              </div>
            )}
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
                variant="companion"
              />
            )}
            <div className="companion-reference-panel">
              <div className="companion-reference-head">
                <span>{t("companion.reference.title")}</span>
                <div className="companion-reference-actions">
                  <button type="button" className="chip" onClick={openClipboardReference} disabled={isBusy}>
                    <Clipboard size={12} />
                    {t("companion.reference.addClipboard")}
                  </button>
                  <button type="button" className="chip" onClick={openTextReference} disabled={isBusy}>
                    <MessageSquare size={12} />
                    {t("companion.reference.addText")}
                  </button>
                  <button type="button" className="chip" onClick={openMarkdownReference} disabled={isBusy}>
                    <FileText size={12} />
                    {t("companion.reference.addFile")}
                  </button>
                </div>
              </div>
              {referenceNotice && <div className="companion-reference-notice" aria-live="polite">{referenceNotice}</div>}
              {referenceDraftOpen && (
                <div className="companion-reference-editor">
                  <input
                    value={referenceTitle}
                    onChange={(e) => setReferenceTitle(e.target.value)}
                    placeholder={t("companion.reference.titlePlaceholder")}
                    aria-label={t("companion.reference.titleInput")}
                  />
                  <div className="companion-reference-controls">
                    <select
                      value={referencePurpose}
                      onChange={(e) => setReferencePurpose(e.target.value as CompanionReferencePurpose)}
                      aria-label={t("companion.reference.purposeLabel")}
                    >
                      {REFERENCE_PURPOSES.map((purpose) => (
                        <option key={purpose} value={purpose}>{referencePurposeLabel(t, purpose)}</option>
                      ))}
                    </select>
                    <select
                      value={referenceScope}
                      onChange={(e) => setReferenceScope(e.target.value as ReferenceScopeDraft)}
                      aria-label={t("companion.reference.scopeLabel")}
                    >
                      <option value="project">{t("companion.reference.scope.project")}</option>
                      <option value="scene" disabled={!currentNodeId}>{t("companion.reference.scope.scene")}</option>
                    </select>
                  </div>
                  <textarea
                    value={referenceText}
                    onChange={(e) => setReferenceText(e.target.value)}
                    placeholder={t("companion.reference.contentPlaceholder")}
                    aria-label={t("companion.reference.contentInput")}
                    rows={5}
                  />
                  <div className="companion-reference-editor-actions">
                    <button type="button" className="btn ghost sm" onClick={() => setReferenceDraftOpen(false)} disabled={referenceSaving}>
                      {t("common.cancel")}
                    </button>
                    <button type="button" className="btn primary sm" onClick={() => { void saveReference(); }} disabled={!referenceText.trim() || referenceSaving}>
                      {t("companion.reference.save")}
                    </button>
                  </div>
                </div>
              )}
              {referencesLoading ? (
                <div className="companion-reference-empty">
                  <span className="ai-working-dot" aria-hidden="true" />
                  {t("companion.reference.loading")}
                </div>
              ) : references.length === 0 ? (
                <div className="companion-reference-empty">{t("companion.reference.empty")}</div>
              ) : (
                <ul className="companion-reference-list">
                  {references.map((ref) => (
                    <li key={ref.id} className={ref.status === "disabled" ? "is-disabled" : ""}>
                      <div className="companion-reference-row-main">
                        <span className="companion-reference-purpose">{referencePurposeLabel(t, ref.purpose)}</span>
                        <strong title={ref.title}>{ref.title}</strong>
                        <span className="companion-reference-scope">{t(`companion.reference.scope.${referenceScopeOf(ref)}`)}</span>
                        <span className="companion-reference-token">{formatTokenEstimate(ref.token_estimate)}</span>
                      </div>
                      <div className="companion-reference-row-sub">
                        <span>{referenceStatusLabel(t, ref.status)}</span>
                        <span>{t("companion.reference.charCount", {
                          count: ref.char_count.toLocaleString(localeForLanguage(language)),
                        })}</span>
                      </div>
                      <div className="companion-reference-row-actions">
                        {ref.status === "disabled" ? (
                          <button type="button" className="btn ghost sm" onClick={() => { void updateReferenceStatus(ref, "active"); }}>{t("companion.reference.enable")}</button>
                        ) : (
                          <button type="button" className="btn ghost sm" onClick={() => { void updateReferenceStatus(ref, "disabled"); }}>{t("companion.reference.disable")}</button>
                        )}
                        {ref.status === "summarized" ? (
                          <button type="button" className="btn ghost sm" onClick={() => { void updateReferenceStatus(ref, "active"); }}>{t("companion.reference.useOriginal")}</button>
                        ) : (
                          <button type="button" className="btn ghost sm" onClick={() => { void updateReferenceStatus(ref, "summarized"); }}>{t("companion.reference.useSummary")}</button>
                        )}
                        <button type="button" className="btn ghost sm danger" onClick={() => { void deleteReference(ref); }}>{t("common.delete")}</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </section>
        )}
      </div>
      )}
      {aiSetupOpen && (
        <div className="modal-backdrop center companion-ai-setup-backdrop" onMouseDown={(e) => e.stopPropagation()}>
          <div className="modal companion-ai-setup-modal" role="dialog" aria-modal="true" aria-labelledby="ai-setup-start-title">
            <AISetupStart
              variant="modal"
              currentProvider={aiSetupProvider}
              currentProviderLabel={t(`settings.provider.${aiSetupProvider === "openai-codex" ? "openaiCodex" : aiSetupProvider === "anthropic" ? "anthropic" : aiSetupProvider === "openrouter" ? "openrouter" : aiSetupProvider === "gemini-native" ? "gemini" : aiSetupProvider === "claude-code-cli" ? "claudeCli" : "openai"}.label`)}
              credentialState={t("settings.provider.stateNeedsConnection")}
              unavailableProviders={[]}
              selectedGuideId={aiSetupGuideId}
              onGuideIdChange={setAISetupGuideId}
              onSelectProvider={(provider) => { void selectAISetupProvider(provider); }}
              openRouterKeyInfo={aiSetupOpenRouterKeyInfo}
              openRouterKeyInfoLoading={aiSetupOpenRouterKeyInfoLoading}
              openRouterKeyInfoError={aiSetupOpenRouterKeyInfoError}
              onRefreshOpenRouterKeyInfo={() => { void refreshAISetupOpenRouterKeyInfo(); }}
              openRouterAPIKeyDraft={aiSetupOpenRouterKeyDraft}
              openRouterAPIKeySaved={aiSetupOpenRouterKeySaved}
              openRouterModelDraft={aiSetupOpenRouterModelDraft}
              openRouterModelOptions={aiSetupOpenRouterModels}
              openRouterModelsLoading={aiSetupOpenRouterModelsLoading}
              openRouterModelsError={aiSetupOpenRouterModelsError}
              openRouterSetupBusy={aiSetupOpenRouterBusy}
              openRouterTestMessage={aiSetupOpenRouterMsg}
              onOpenRouterAPIKeyChange={(value) => {
                setAISetupOpenRouterKeyDraft(value);
                setAISetupOpenRouterMsg(null);
              }}
              onOpenRouterModelChange={(value) => {
                setAISetupOpenRouterModelDraft(value);
                setAISetupOpenRouterMsg(null);
              }}
              onSaveOpenRouter={() => { void saveAISetupOpenRouter(); }}
              onClearOpenRouterAPIKey={() => { void clearAISetupOpenRouterKey(); }}
              onRefreshOpenRouterModels={() => { void refreshAISetupOpenRouterModels(); }}
              onTestOpenRouter={() => { void testAISetupOpenRouter(); }}
              onClose={() => {
                setAISetupOpen(false);
                window.requestAnimationFrame(() => focusInput());
              }}
            />
          </div>
        </div>
      )}
    </aside>
  );
}
