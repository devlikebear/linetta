import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { Search, Command as CommandIcon, Maximize2, ArrowLeft, BookOpen, Library, Replace, Menu, Keyboard } from "lucide-react";
import { nodes, projects, snapshots, entities as entitiesApi, mentions as mentionsApi, threads as threadsApi, beats as beatsApi, settings as settingsApi, exportApi, notes as notesApi, gitSync, stats as statsApi, diagnostics as diagnosticsApi } from "../lib/rpc";
import { McpToggle } from "../components/McpToggle";
import { NoteMarkerExtension } from "../components/editor/NoteMarkerExtension";
import { NotePopover } from "../components/NotePopover";
import type { NodeRow, Project, Entity, SearchResult, Settings as SettingsRow, NodeStatus } from "../lib/types";
import { buildMentionExtension, type MentionPickerState } from "../components/editor/MentionExtension";
import { MentionPicker } from "../components/editor/MentionPicker";
import { EntitySheet } from "../components/EntitySheet";
import { ThreadSheet } from "../components/ThreadSheet";
import { VersionSheet } from "../components/VersionSheet";
import { exportDestinationMessage, saveExportedMarkdown } from "../lib/exportSave";
import { TiptapEditor, type TiptapHandle, type TiptapSelectionMenuPayload } from "../components/editor/Tiptap";
import { autoMentionDoc, countAutoMentionCandidates } from "../lib/editor/autoMention";
import { ZenMode } from "../components/ZenMode";
import { ContextPanel, type SaveStatus } from "../components/ContextPanel";
import { CanonPanel } from "../components/CanonPanel";
import { FactBookPanel } from "../components/FactBookPanel";
import { AgentPanel } from "../components/agent/AgentPanel";
import { ContextualEditPanel } from "../components/contextual/ContextualEditPanel";
import { OutlinePanel } from "../components/OutlinePanel";
import { InlineEditableText } from "../components/InlineEditableText";
import { CommandPalette, type Command } from "../components/CommandPalette";
import { ShortcutsModal } from "../components/ShortcutsModal";
import { SearchModal } from "../components/SearchModal";
import { OnboardingTour, type OnboardingTourStep } from "../components/onboarding/OnboardingTour";
import {
  CURRENT_ONBOARDING_TOUR_VERSION,
  MANUAL_PHASE_STORAGE_KEY,
  WORKSPACE_PENDING_STORAGE_KEY,
  clearStoredPhase,
  readStoredPhase,
  shouldAutoStartOnboarding,
} from "../components/onboarding/onboardingState";
import { useSizeClass } from "../hooks/useSizeClass";
import { reconcileInspector } from "../hooks/inspector";
import type { InspectorState } from "../hooks/inspector";
import { useKeyedDebouncedCallback } from "../hooks/useDebouncedCallback";
import { useIdleTimer } from "../hooks/useIdleTimer";
import { useThrottledCallback } from "../hooks/useThrottledCallback";
import { subscribeAppEvent, type LinettaEventMap } from "../lib/appEvents";
import { useToast } from "../components/ToastProvider";
import { displayNodeLabel, localeForLanguage, useI18n } from "../lib/i18n";
import { outlineNumberLabel, outlinePresetById, repairOutlineTree, type OutlinePresetId } from "../lib/outlineRepair";
import { findEpisodeNode } from "../lib/outlineEpisode";
import { planChapterCreation, type CreateNodeStep } from "../lib/outlineCreate";
import { normalizePlatformProfile, transformPlatformText } from "../lib/platformProfiles";
import { SceneSaveQueue } from "../lib/sceneSaveQueue";
import { useMcpChanges } from "../hooks/useMcpChanges";
import {
  buildTree,
  countEpisodeStatus,
  findFirstLeaf,
  flatten,
  leafNeighbors,
  sumLeafChars,
  type TreeNode,
} from "../hooks/useFirstLeaf";

const SAVE_DEBOUNCE_MS = 800;
// Longer than the save debounce: a name is only worth counting once the
// writer has stopped mid-sentence, and the scan walks the whole document.
const AUTO_MENTION_SCAN_MS = 1500;
const IDLE_CHECKPOINT_MS = 120_000;
const LAST_OPENED_THROTTLE_MS = 5000;
function seedRailCollapsed(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  // Collapsed on touch/mobile tiers so the first workspace view is the editor.
  const compactish = window.matchMedia("(max-width: 860px)").matches;
  const ipadTouch = window.matchMedia(
    "(min-width: 701px) and (max-width: 1366px) and (min-height: 600px) and (any-pointer: coarse)",
  ).matches;
  return compactish || ipadTouch;
}



interface LoadState {
  project: Project;
  node: NodeRow;
  initialDoc: object;
  tree: TreeNode[];
}

type DialogState =
  | { kind: "prompt"; title: string; initial: string; resolve: (v: string | null) => void }
  | { kind: "confirm"; title: string; resolve: (v: boolean) => void };

type SelectionMenuState = TiptapSelectionMenuPayload & {
  x: number;
  y: number;
};

function snapshotOutlineTree(tree: TreeNode[]): NodeRow[] {
  return flatten(tree).map(({ children: _children, ...node }) => ({ ...node }));
}

export function Workspace() {
  const { projectId } = useParams();
  const sizeClass = useSizeClass();
  const navigate = useNavigate();
  const location = useLocation();
  const [load, setLoad] = useState<LoadState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { showToast } = useToast();
  const { language, t } = useI18n();
  const locale = localeForLanguage(language);
  const [charCount, setCharCount] = useState(0);
  const [todayChars, setTodayChars] = useState<number | null>(null);
  const [typewriter, setTypewriter] = useState(false);
  const [focus, setFocus] = useState(false);
  const [railCollapsed, setRailCollapsed] = useState(() => seedRailCollapsed());
  const [saveStatus, setSaveStatus] = useState<SaveStatus>({ kind: "idle" });
  // Whether the buffer holds edits the engine has not seen. An external
  // agent must never replace those, so this gates the MCP scene refresh.
  const [editorDirty, setEditorDirty] = useState(false);
  const saveCompletedAt = saveStatus.kind === "saved" ? saveStatus.at : null;
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [versionSheetNodeId, setVersionSheetNodeId] = useState<string | null>(null);
  const [zenOpen, setZenOpen] = useState(false);
  const zenOpenRef = useRef(false);
  useEffect(() => {
    zenOpenRef.current = zenOpen;
  }, [zenOpen]);
  const [dialog, setDialog] = useState<DialogState | null>(null);
  const [mentionState, setMentionState] = useState<MentionPickerState | null>(null);
  const [entitySheetId, setEntitySheetId] = useState<string | null>(null);
  const [threadSheetId, setThreadSheetId] = useState<string | null>(null);
  const [factBookOpen, setFactBookOpen] = useState(false);
  const [contextualEditOpen, setContextualEditOpen] = useState(false);
  const [canonOpen, setCanonOpen] = useState(false);
  const [agentOpen, setAgentOpen] = useState(false);
  // Bumped when an external agent changes the work, so the story world list
  // reflects the three characters it just said it created (#28).
  const [canonRefreshKey, setCanonRefreshKey] = useState(0);
  const prevInspectorRef = useRef<InspectorState>({
    factBook: false,
    contextual: false,
    canon: false,
    agent: false,
  });
  useEffect(() => {
    const next: InspectorState = {
      factBook: factBookOpen,
      contextual: contextualEditOpen,
      canon: canonOpen,
      agent: agentOpen,
    };
    const corrected = reconcileInspector(prevInspectorRef.current, next, sizeClass);
    if (corrected.factBook !== next.factBook) setFactBookOpen(corrected.factBook);
    if (corrected.contextual !== next.contextual) setContextualEditOpen(corrected.contextual);
    if (corrected.canon !== next.canon) setCanonOpen(corrected.canon);
    if (corrected.agent !== next.agent) setAgentOpen(corrected.agent);
    prevInspectorRef.current = corrected;
  }, [sizeClass, factBookOpen, contextualEditOpen, canonOpen, agentOpen]);
  const [contextualSeed, setContextualSeed] = useState<{ entityId?: string; text?: string; autoCheck?: boolean } | null>(null);
  const [outlineUndoSnapshot, setOutlineUndoSnapshot] = useState<NodeRow[] | null>(null);
  const [outlineRenameRequest, setOutlineRenameRequest] = useState<{ id: string; nonce: number } | null>(null);
  const outlinePreset = useMemo(() => outlinePresetById(load?.project.outline_preset), [load?.project.outline_preset]);
  const outlinePresetId = outlinePreset.id;
  const [factBookSelectedClaimRequest, setFactBookSelectedClaimRequest] = useState<{ id: string; claim: string } | null>(null);
  const [selectionMenu, setSelectionMenu] = useState<SelectionMenuState | null>(null);
  const [settingsRow, setSettingsRow] = useState<SettingsRow | null>(null);
  const [gitSyncAvailable, setGitSyncAvailable] = useState(true);
  const [tourOpen, setTourOpen] = useState(false);
  const [mentioned, setMentioned] = useState<Entity[]>([]);
  const [autoMentionBusy, setAutoMentionBusy] = useState(false);
  // Registered names sitting in the prose without a mention link. Counted, not
  // applied: linking rewrites the manuscript and can pick the wrong record for
  // a homonym, so the writer decides (#32).
  const [autoMentionFound, setAutoMentionFound] = useState(0);
  const [autoMentionScanKey, setAutoMentionScanKey] = useState(0);
  const factBookSelectionSeqRef = useRef(0);
  const loadRef = useRef<LoadState | null>(null);
  const sceneSaveQueueRef = useRef<SceneSaveQueue<NodeRow> | null>(null);
  if (!sceneSaveQueueRef.current) {
    sceneSaveQueueRef.current = new SceneSaveQueue((nodeId, doc, expectedVersion) =>
      nodes.updateContent(nodeId, doc, expectedVersion));
  }
  const sceneSaveQueue = sceneSaveQueueRef.current;
  useEffect(() => {
    loadRef.current = load;
  }, [load]);
  useEffect(() => {
    if (load) sceneSaveQueue.seed(load.node.id, load.node.content_version ?? 0);
  }, [load, sceneSaveQueue]);
  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const compact = window.matchMedia("(max-width: 860px)");
    const ipadTouch = window.matchMedia(
      "(min-width: 701px) and (max-width: 1366px) and (min-height: 600px) and (any-pointer: coarse)",
    );
    const onChange = () => {
      if (compact.matches || ipadTouch.matches) setRailCollapsed(true);
    };
    compact.addEventListener("change", onChange);
    ipadTouch.addEventListener("change", onChange);
    onChange();
    return () => {
      compact.removeEventListener("change", onChange);
      ipadTouch.removeEventListener("change", onChange);
    };
  }, []);
  useEffect(() => {
    const vv = typeof window !== "undefined" ? window.visualViewport : null;
    if (!vv) return;
    const root = document.documentElement;
    const update = () => {
      // How much of the layout viewport the soft keyboard covers at the bottom.
      const inset = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
      root.style.setProperty("--kbd-inset", `${Math.round(inset)}px`);
    };
    vv.addEventListener("resize", update);
    vv.addEventListener("scroll", update);
    update();
    return () => {
      vv.removeEventListener("resize", update);
      vv.removeEventListener("scroll", update);
      root.style.removeProperty("--kbd-inset");
    };
  }, []);
  const refreshTodayChars = useCallback(async (targetProjectId?: string) => {
    const id = targetProjectId ?? loadRef.current?.project.id;
    if (!id) return;
    try {
      const today = await statsApi.today(id);
      setTodayChars(today.chars_added);
    } catch {
      /* benign; the sidebar can omit today's number */
    }
  }, []);
  useEffect(() => {
    if (!load?.project.id) {
      setTodayChars(null);
      return;
    }
    setTodayChars(null);
    void refreshTodayChars(load.project.id);
  }, [load?.project.id, refreshTodayChars]);
  useEffect(() => {
    if (saveCompletedAt === null || !load?.project.id) return;
    void refreshTodayChars(load.project.id);
  }, [saveCompletedAt, load?.project.id, refreshTodayChars]);
  useEffect(() => { setOutlineUndoSnapshot(null); }, [projectId]);
  const editorRef = useRef<TiptapHandle>(null);
  const savedSelectionRef = useRef<{ from: number; to: number } | null>(null);
  const zenEditorRef = useRef<TiptapHandle | null>(null);
  const [notePopover, setNotePopover] = useState<{
    noteId: string;
    targetEl: HTMLElement;
    mode: "read" | "edit";
  } | null>(null);

  useEffect(() => {
    const onHover = (detail: LinettaEventMap["linetta:note-hover"]) => {
      setNotePopover((cur) => {
        if (cur && cur.mode === "edit" && cur.noteId === detail.noteId) return cur;
        return { noteId: detail.noteId, targetEl: detail.target, mode: "read" };
      });
    };
    const onHoverEnd = (detail: LinettaEventMap["linetta:note-hover-end"]) => {
      setNotePopover((cur) => {
        if (!cur) return null;
        if (cur.mode === "edit") return cur;
        if (cur.noteId !== detail.noteId) return cur;
        return null;
      });
    };
    const onClick = (detail: LinettaEventMap["linetta:note-click"]) => {
      setNotePopover({ noteId: detail.noteId, targetEl: detail.target, mode: "edit" });
    };
    const unsubscribeHover = subscribeAppEvent("linetta:note-hover", onHover);
    const unsubscribeHoverEnd = subscribeAppEvent("linetta:note-hover-end", onHoverEnd);
    const unsubscribeClick = subscribeAppEvent("linetta:note-click", onClick);
    return () => {
      unsubscribeHover();
      unsubscribeHoverEnd();
      unsubscribeClick();
    };
  }, []);

  const enterZen = useCallback(() => {
    savedSelectionRef.current = editorRef.current?.getSelection() ?? null;
    setZenOpen(true);
  }, []);

  const exitZen = useCallback(() => {
    savedSelectionRef.current =
      zenEditorRef.current?.getSelection() ?? savedSelectionRef.current;
    // ZEN의 Tiptap은 Edit의 Tiptap과 별도 인스턴스라 ZEN에서 친 글자는
    // Edit editor의 메모리에 반영되지 않는다. ZEN의 최신 doc을 가져와
    // load.initialDoc을 갱신하면 Edit editor가 새 길이로 remount되어 보존된다.
    const zenDoc = zenEditorRef.current?.getDoc();
    if (zenDoc) {
      setLoad((prev) => (prev ? { ...prev, initialDoc: zenDoc } : prev));
    }
    setZenOpen(false);
  }, []);

  const handleOutlinePresetChange = useCallback(
    async (nextPresetId: OutlinePresetId) => {
      const current = loadRef.current;
      if (!current) return;
      try {
        const updated = await projects.update({ id: current.project.id, outline_preset: nextPresetId });
        setLoad((prev) => (prev && prev.project.id === updated.id ? { ...prev, project: updated } : prev));
        setOutlineUndoSnapshot(null);
      } catch (e) {
        showToast(t("workspace.toast.outlinePresetSaveFailed", { error: String(e) }));
      }
    },
    [showToast, t],
  );

  // Restore the saved selection in the Edit-mode editor once ZEN closes.
  useEffect(() => {
    if (!zenOpen && savedSelectionRef.current) {
      const id = window.requestAnimationFrame(() => {
        const sel = savedSelectionRef.current;
        if (sel) editorRef.current?.setSelection(sel);
      });
      return () => window.cancelAnimationFrame(id);
    }
    return undefined;
  }, [zenOpen]);

  const focusEditor = useCallback(() => {
    window.setTimeout(() => editorRef.current?.focus(), 0);
  }, []);

  useEffect(() => {
    if (!selectionMenu) return undefined;
    const close = () => setSelectionMenu(null);
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    window.addEventListener("click", close);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("click", close);
      window.removeEventListener("keydown", onKey);
    };
  }, [selectionMenu]);

  const openEditorSelectionMenu = useCallback((event: ReactMouseEvent, payload: TiptapSelectionMenuPayload) => {
    // A bare cursor had only one action, "continue writing here", and that was
    // the companion. Without it there is nothing to offer, so the menu now
    // opens on a selection only.
    if (payload.kind !== "selection") return;
    setSelectionMenu({
      ...payload,
      x: event.clientX,
      y: event.clientY,
    });
  }, []);

  // Apply typewriter + focus defaults from settings exactly once on mount.
  useEffect(() => {
    let cancelled = false;
    settingsApi.get()
      .then((s) => {
        if (cancelled) return;
        setSettingsRow(s);
        setTypewriter(s.typewriter_default);
        setFocus(s.focus_default);
      })
      .catch(() => { /* benign */ });
    diagnosticsApi.get()
      .then((d) => { if (!cancelled) setGitSyncAvailable(d.git_sync_available ?? true); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  const mentionExtension = useMemo(() => {
    if (!projectId) return null;
    return buildMentionExtension({
      search: async (query) => {
        const results = await entitiesApi.search(projectId, query);
        return results.map((e) => ({ id: e.id, name: e.name, role: e.role }));
      },
      onStateChange: setMentionState,
    });
  }, [projectId]);

  const refreshMentioned = useCallback(async (nodeId: string) => {
    try { setMentioned(await mentionsApi.listForNode(nodeId)); }
    catch { /* benign */ }
  }, []);

  // --- Dialog helpers (replace window.prompt / window.confirm, which Tauri 2 blocks). ---
  const promptDialog = useCallback(
    (title: string, initial = ""): Promise<string | null> =>
      new Promise((resolve) => setDialog({ kind: "prompt", title, initial, resolve })),
    [],
  );
  const confirmDialog = useCallback(
    (title: string): Promise<boolean> =>
      new Promise((resolve) => setDialog({ kind: "confirm", title, resolve })),
    [],
  );

  const fetchTree = useCallback(
    async (pId: string, nId: string): Promise<LoadState> => {
      const p = await projects.get(pId);
      const flat = await nodes.listTree(pId);
      const tree = buildTree(flat);
      const node = flat.find((x) => x.id === nId);
      if (!node) {
        const firstLeaf = tree.length > 0 ? findFirstLeaf(tree[0]) : null;
        if (!firstLeaf) throw new Error("project has no leaf");
        const n = await nodes.get(firstLeaf.id);
        const initialDoc = JSON.parse(n.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`);
        return { project: p, node: n, initialDoc, tree };
      }
      const initialDoc = JSON.parse(node.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`);
      return { project: p, node, initialDoc, tree };
    },
    [],
  );

  // Refresh the entire tree AND jump to the given node id, all in one setLoad —
  // avoids the stale-closure race where a subsequent navigateToNode overwrites
  // the freshly fetched tree.
  const refreshTreeAndNavigateTo = useCallback(
    async (targetNodeId: string) => {
      if (!projectId) return;
      const fresh = await fetchTree(projectId, targetNodeId);
      setLoad(fresh);
      setCharCount(fresh.node.word_count);
      nodes.setLastOpened(projectId, targetNodeId).catch(() => { /* benign */ });
    },
    [projectId, fetchTree],
  );

  // Refresh the tree only (keep the active node where it is). Used by Move/Rename.
  // An agent's change to structure. The open scene's buffer is deliberately
  // left alone here — replacing it is onSceneChanged's job, and only when the
  // writer has nothing unsaved.
  const refreshOutlineFromEngine = useCallback(async () => {
    if (!projectId || !loadRef.current) return;
    const [p, flat] = await Promise.all([projects.get(projectId), nodes.listTree(projectId)]);
    setLoad((prev) => (prev ? { ...prev, project: p, tree: buildTree(flat) } : prev));
  }, [projectId]);

  // Pull an agent's prose into the editor. The Tiptap key includes
  // content_version, so a new version remounts the editor on the new doc, and
  // the save-queue seed effect re-seeds against it.
  const reloadSceneFromEngine = useCallback(async (nodeId: string) => {
    const n = await nodes.get(nodeId);
    const initialDoc = JSON.parse(n.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`);
    setLoad((prev) => (prev && prev.node.id === n.id ? { ...prev, node: n, initialDoc } : prev));
    setCharCount(n.word_count);
  }, []);

  const { conflictNodeId, conflictSource, dismissConflict } = useMcpChanges({
    projectId: projectId ?? null,
    openNodeId: load?.node.id ?? null,
    editorDirty,
    onOutlineChanged: () => {
      void refreshOutlineFromEngine();
      // linetta_apply_story_ops carries create_entity and create_relationship,
      // so the same signal that refreshes the outline refreshes the cast.
      setCanonRefreshKey((k) => k + 1);
    },
    onSceneChanged: (nodeId) => { void reloadSceneFromEngine(nodeId); },
  });

  const refreshTreeKeepNode = useCallback(
    async (currentNodeId: string) => {
      if (!projectId) return;
      const fresh = await fetchTree(projectId, currentNodeId);
      setLoad(fresh);
      setCharCount(fresh.node.word_count);
    },
    [projectId, fetchTree],
  );

  const handleRenameNode = useCallback(
    async (target: TreeNode, title: string) => {
      try {
        await nodes.rename(target.id, target.label, title.trim());
        await refreshTreeKeepNode(loadRef.current?.node.id ?? target.id);
        showToast(t("workspace.toast.renameSuccess"));
      } catch (e) {
        showToast(t("workspace.toast.renameFailed", { error: String(e) }));
      }
    },
    [refreshTreeKeepNode, showToast, t],
  );

  const requestInlineRenameNode = useCallback((target: TreeNode) => {
    setOutlineRenameRequest({ id: target.id, nonce: Date.now() });
  }, []);

  const handleCreateSceneFromOutline = useCallback(
    async (anchor: TreeNode) => {
      const current = loadRef.current;
      if (!current) return;
      try {
        const allNodes = flatten(current.tree);
        let created: NodeRow;
        if (anchor.kind === "container") {
          const childLeaves = anchor.children.filter((n) => n.kind === "leaf");
          created = await nodes.createChild(anchor.id, "leaf", outlineNumberLabel(outlinePreset, "scene", childLeaves.length + 1, t), "");
        } else {
          const siblings = allNodes.filter((n) => (n.parent_id ?? null) === (anchor.parent_id ?? null));
          const leafCount = siblings.filter((n) => n.kind === "leaf").length;
          created = await nodes.createSibling(anchor.id, "leaf", outlineNumberLabel(outlinePreset, "scene", leafCount + 1, t), "");
        }
        await refreshTreeAndNavigateTo(created.id);
      } catch (e) {
        showToast(t("workspace.toast.createSceneFailed", { error: String(e) }));
      }
    },
    [outlinePreset, refreshTreeAndNavigateTo, showToast, t],
  );

  const handleCreatePartFromOutline = useCallback(
    async (anchor: TreeNode) => {
      const current = loadRef.current;
      if (!current) return;
      try {
        const allNodes = flatten(current.tree);
        let reference = anchor;
        while (reference.parent_id) {
          const parent = allNodes.find((n) => n.id === reference.parent_id);
          if (!parent) break;
          reference = parent;
        }
        const partCount = current.tree.filter((n) => n.kind === "container").length;
        const part = await nodes.createSibling(reference.id, "container", outlineNumberLabel(outlinePreset, "part", partCount + 1, t), "");
        const chapter = await nodes.createChild(part.id, "container", outlineNumberLabel(outlinePreset, "chapter", 1, t), "");
        const scene = await nodes.createChild(chapter.id, "leaf", outlineNumberLabel(outlinePreset, "scene", 1, t), "");
        await refreshTreeAndNavigateTo(scene.id);
      } catch (e) {
        showToast(t("workspace.toast.createPartFailed", { error: String(e) }));
      }
    },
    [outlinePreset, refreshTreeAndNavigateTo, showToast, t],
  );

  const handleCreateChapterFromOutline = useCallback(
    async (anchor: TreeNode) => {
      const current = loadRef.current;
      if (!current) return;
      try {
        const plan = planChapterCreation(current.tree, anchor, outlinePreset, t);
        const createNode = async (step: CreateNodeStep) => {
          if (step.placement === "child") {
            return nodes.createChild(step.parentId, step.kind, step.label, step.title);
          }
          return nodes.createSibling(step.referenceId, step.kind, step.label, step.title);
        };
        const chapter = await createNode(plan.chapter);
        if (!plan.seedScene) {
          await refreshTreeAndNavigateTo(chapter.id);
          return;
        }
        const seeded = await nodes.createChild(chapter.id, "leaf", plan.seedSceneLabel, "");
        await refreshTreeAndNavigateTo(seeded.id);
      } catch (e) {
        showToast(t("workspace.toast.createChapterFailed", { error: String(e) }));
      }
    },
    [outlinePreset, refreshTreeAndNavigateTo, showToast, t],
  );

  const handleMoveSceneFromOutline = useCallback(
    async (target: TreeNode, direction: "up" | "down") => {
      const current = loadRef.current;
      if (!current) return;
      try {
        if (direction === "up") {
          await nodes.moveUp(target.id);
        } else {
          await nodes.moveDown(target.id);
        }
        await refreshTreeKeepNode(current.node.id);
      } catch (e) {
        showToast(t("workspace.toast.moveSceneFailed", { error: String(e) }));
      }
    },
    [refreshTreeKeepNode, showToast, t],
  );

  const handleMoveNodeTo = useCallback(
    async (target: TreeNode, parentId: string | null, ordinal: number) => {
      try {
        await nodes.moveTo(target.id, parentId, ordinal);
        await refreshTreeKeepNode(loadRef.current?.node.id ?? target.id);
      } catch (e) {
        showToast(t("workspace.toast.moveSceneFailed", { error: String(e) }));
      }
    },
    [refreshTreeKeepNode, showToast, t],
  );

  const handleSetNodeStatus = useCallback(
    async (target: TreeNode, status: NodeStatus) => {
      const current = loadRef.current;
      if (!current) return;
      try {
        await nodes.setStatus(target.id, status);
        await refreshTreeKeepNode(current.node.id);
      } catch (e) {
        showToast(t("workspace.toast.statusChangeFailed", { error: String(e) }));
      }
    },
    [refreshTreeKeepNode, showToast, t],
  );

  const handleDeleteSceneFromOutline = useCallback(
    async (target: TreeNode) => {
      const current = loadRef.current;
      if (!current) return;
      const targetLabel = displayNodeLabel(language, target.label);
      const ok = await confirmDialog(
        target.kind === "leaf"
          ? t("workspace.confirm.deleteScene", { label: targetLabel })
          : t("workspace.confirm.deleteNode", { label: target.title ? `${targetLabel} · ${target.title}` : targetLabel }),
      );
      if (!ok) return;
      try {
        const deletedIDs = new Set(flatten([target]).map((n) => n.id));
        const leaves = flatten(current.tree).filter((n) => n.kind === "leaf");
        const deletedLeafIndexes = leaves
          .map((leaf, index) => (deletedIDs.has(leaf.id) ? index : -1))
          .filter((index) => index >= 0);
        const firstDeleted = deletedLeafIndexes.length > 0 ? Math.min(...deletedLeafIndexes) : -1;
        const lastDeleted = deletedLeafIndexes.length > 0 ? Math.max(...deletedLeafIndexes) : -1;
        const fallback = lastDeleted >= 0
          ? (leaves.slice(lastDeleted + 1).find((leaf) => !deletedIDs.has(leaf.id)) ??
            [...leaves.slice(0, firstDeleted)].reverse().find((leaf) => !deletedIDs.has(leaf.id)) ??
            null)
          : null;
        await nodes.delete(target.id);
        if (deletedIDs.has(current.node.id)) {
          if (fallback) {
            await refreshTreeAndNavigateTo(fallback.id);
          } else {
            navigate("/");
          }
        } else {
          await refreshTreeKeepNode(current.node.id);
        }
        showToast(t("workspace.toast.deleteSceneSuccess"));
      } catch (e) {
        showToast(t("workspace.toast.deleteSceneFailed", { error: String(e) }));
      }
    },
    [confirmDialog, language, navigate, refreshTreeAndNavigateTo, refreshTreeKeepNode, showToast, t],
  );

  const handleRepairOutline = useCallback(
    async () => {
      const current = loadRef.current;
      if (!current) return;
      const ok = await confirmDialog(t("workspace.confirm.repairOutline"));
      if (!ok) return;
      try {
        const snapshot = snapshotOutlineTree(current.tree);
        await repairOutlineTree(current.tree, nodes, t, outlinePreset);
        setOutlineUndoSnapshot(snapshot);
        await refreshTreeKeepNode(current.node.id);
        showToast(t("workspace.toast.repairOutlineSuccess"));
      } catch (e) {
        showToast(t("workspace.toast.repairOutlineFailed", { error: String(e) }));
      }
    },
    [confirmDialog, outlinePreset, refreshTreeKeepNode, showToast, t],
  );

  const handleUndoRepairOutline = useCallback(
    async () => {
      const current = loadRef.current;
      if (!current || !projectId || !outlineUndoSnapshot) return;
      const ok = await confirmDialog(t("workspace.confirm.undoRepairOutline"));
      if (!ok) return;
      try {
        await nodes.restoreOutline(projectId, outlineUndoSnapshot);
        setOutlineUndoSnapshot(null);
        await refreshTreeKeepNode(current.node.id);
        showToast(t("workspace.toast.undoRepairOutlineSuccess"));
      } catch (e) {
        showToast(t("workspace.toast.undoRepairOutlineFailed", { error: String(e) }));
      }
    },
    [confirmDialog, outlineUndoSnapshot, projectId, refreshTreeKeepNode, showToast, t],
  );

  // Navigate without re-fetching the tree (used by outline click + leaf neighbor cmds).
  const navigateToNode = useCallback(
    async (target: TreeNode | NodeRow) => {
      if (!projectId) return;
      const leaf = "children" in target ? findFirstLeaf(target as TreeNode) : (target as NodeRow);
      if (!leaf) {
        showToast(t("workspace.toast.noSceneToNavigate"));
        return;
      }
      const n = await nodes.get(leaf.id);
      const initialDoc = JSON.parse(n.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`);
      // Functional setLoad so we don't clobber the latest tree.
      setLoad((prev) => (prev ? { ...prev, node: n, initialDoc } : prev));
      setCharCount(n.word_count);
      nodes.setLastOpened(projectId, n.id).catch(() => { /* benign */ });
    },
    [projectId, showToast, t],
  );

  // Initial load.
  useEffect(() => {
    if (!projectId) return;
    let cancelled = false;
    (async () => {
      try {
        const p = await projects.get(projectId);
        const jumpTo = (location.state as { jumpToNodeId?: string } | null)?.jumpToNodeId;
        const target = jumpTo ?? p.last_opened_node_id;
        if (!target) throw new Error("project has no opened node");
        const next = await fetchTree(projectId, target);
        if (!cancelled) {
          setLoad(next);
          setCharCount(next.node.word_count);
          if (jumpTo) navigate(location.pathname, { replace: true, state: null });
        }
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => { cancelled = true; };
  }, [fetchTree, location.pathname, location.state, navigate, projectId]);

  const activeMentionNodeId = load?.node.id;
  // Refresh mentioned entities when the active node changes.
  useEffect(() => {
    if (activeMentionNodeId) refreshMentioned(activeMentionNodeId);
  }, [activeMentionNodeId, refreshMentioned]);

  // Listen for "new entity" event dispatched by MentionExtension.
  useEffect(() => {
    if (!projectId) return;
    const handler = async (detail: LinettaEventMap["linetta:mention-pick-new"]) => {
      try {
        const created = await entitiesApi.create({ project_id: projectId, kind: "character", name: detail.query });
        detail.editor.chain().focus().deleteRange(detail.range).insertContent([
          { type: "mention", attrs: { id: created.id, label: created.name } },
          { type: "text", text: " " },
        ]).run();
        setContextualEditOpen(false);
        setEntitySheetId(created.id);
      } catch (err) {
        showToast(t("workspace.toast.entityCreateFailed", { error: String(err) }));
      }
    };
    return subscribeAppEvent("linetta:mention-pick-new", handler);
  }, [projectId, showToast, t]);

  // Global Cmd+R reload + Cmd+P palette toggle + Cmd+F search + Cmd+Shift+F
  // contextual edit + Cmd+J agent panel.
  //
  // Cmd+I (AI draft) is gone with the companion and is deliberately left
  // unbound rather than reassigned. The guard that used to swallow it while
  // the AI modal was open went with it too: that guard tested that modal
  // specifically, not modals in general. Cmd+J used to be unbound for the
  // same reason; it is not any more, now that it opens the agent panel (#95)
  // instead of staying reserved for a companion that is not coming back.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const isMac = navigator.platform.toLowerCase().includes("mac");
      const mod = isMac ? e.metaKey : e.ctrlKey;
      if (!mod) return;
      if (e.key.toLowerCase() === "r") {
        e.preventDefault();
        window.location.reload();
      } else if (e.key.toLowerCase() === "p") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      } else if (e.key.toLowerCase() === "f") {
        e.preventDefault();
        if (e.shiftKey) {
          setSearchOpen(false);
          setFactBookOpen(false);
          setEntitySheetId(null);
          setThreadSheetId(null);
          setContextualEditOpen((v) => !v);
          return;
        }
        setSearchOpen(true);
      } else if (e.key.toLowerCase() === "j") {
        e.preventDefault();
        toggleAgent();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
    // Registered once on mount, same as the rest of this handler. toggleAgent
    // is declared further down with useCallback(..., []), so its identity is
    // stable and never changes — listing it would not change when this
    // effect re-runs, only invite a forward-reference ReferenceError, since
    // deps are evaluated eagerly at this line while toggleAgent's own const
    // has not been reached yet.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const saveNow = useCallback(
    async (nodeId: string, doc: object) => {
      const isActive = () => loadRef.current?.node.id === nodeId;
      if (isActive()) setSaveStatus({ kind: "saving" });
      try {
        await sceneSaveQueue.save(nodeId, JSON.stringify(doc));
        if (isActive()) {
          setSaveStatus({ kind: "saved", at: Date.now() });
          setEditorDirty(false);
        }
        refreshMentioned(nodeId);
      } catch (e) {
        if (isActive()) {
          setSaveStatus({ kind: "error", message: String(e) });
          setError(String(e));
        }
      }
    },
    [refreshMentioned, sceneSaveQueue],
  );
  const debouncedSave = useKeyedDebouncedCallback(saveNow, SAVE_DEBOUNCE_MS);
  const idleDirtyRef = useRef(false);
  const handleIdleCheckpoint = useCallback(async () => {
    if (!idleDirtyRef.current) return;
    const currentLoad = loadRef.current;
    const doc = zenOpenRef.current
      ? zenEditorRef.current?.getDoc()
      : editorRef.current?.getDoc();
    if (!currentLoad || !doc) return;
    idleDirtyRef.current = false;
    try {
      await snapshots.createAuto(currentLoad.node.id, JSON.stringify(doc));
    } catch {
      /* benign: idle checkpoints are best-effort */
    }
  }, []);
  const { markActivity, cancel: cancelIdleCheckpoint } = useIdleTimer(
    IDLE_CHECKPOINT_MS,
    handleIdleCheckpoint,
  );
  useEffect(() => {
    idleDirtyRef.current = false;
    setEditorDirty(false);
    cancelIdleCheckpoint();
  }, [load?.node.id, cancelIdleCheckpoint]);
  const throttledLastOpened = useThrottledCallback(
    useCallback(() => {
      if (!load) return;
      nodes.setLastOpened(load.project.id, load.node.id).catch(() => { /* benign */ });
    }, [load]),
    LAST_OPENED_THROTTLE_MS,
  );

  useEffect(() => {
    if (!load) return;
    throttledLastOpened();
  }, [load, throttledLastOpened]);

  const handleSearchSelect = useCallback(
    async (result: SearchResult) => {
      if (load?.project.id === result.project_id) {
        await navigateToNode({ id: result.node_id } as NodeRow);
        return;
      }
      navigate(`/workspace/${result.project_id}`, { state: { jumpToNodeId: result.node_id } });
    },
    [load?.project.id, navigate, navigateToNode],
  );

  const handleManualSave = useCallback(
    async (doc: object) => {
      if (!load) return;
      debouncedSave.cancel(load.node.id);
      setSaveStatus({ kind: "saving" });
      try {
        await sceneSaveQueue.save(load.node.id, JSON.stringify(doc));
        await snapshots.createManual(load.node.id, JSON.stringify(doc));
        setSaveStatus({ kind: "saved", at: Date.now() });
        showToast(t("workspace.toast.snapshotSaved"));
      } catch (e) {
        setSaveStatus({ kind: "error", message: String(e) });
        setError(String(e));
      }
    },
    [debouncedSave, load, sceneSaveQueue, showToast, t],
  );

  // Re-count after the writer stops typing, and after an agent or a scene
  // change replaces the buffer. Debounced because it walks the whole doc.
  useEffect(() => {
    if (!load) return undefined;
    let cancelled = false;
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const doc = editorRef.current?.getDoc();
          if (!doc) return;
          const allEntities = await entitiesApi.list(load.project.id);
          if (!cancelled) setAutoMentionFound(countAutoMentionCandidates(doc, allEntities));
        } catch {
          /* benign: the count is a hint, not a feature the writer waits on */
        }
      })();
    }, AUTO_MENTION_SCAN_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [load, autoMentionScanKey]);

  const handleAutoMentionScene = useCallback(async () => {
    if (!load) return;
    const editor = editorRef.current?.editor;
    const doc = editorRef.current?.getDoc();
    if (!editor || !doc) return;
    setAutoMentionBusy(true);
    try {
      const allEntities = await entitiesApi.list(load.project.id);
      const result = autoMentionDoc(doc, allEntities);
      if (result.applied === 0) {
        setAutoMentionFound(0);
        showToast(t("workspace.toast.autoMentionNone"));
        return;
      }
      setSaveStatus({ kind: "saving" });
      debouncedSave.cancel(load.node.id);
      editor.commands.setContent(result.doc);
      const updated = await sceneSaveQueue.save(load.node.id, JSON.stringify(result.doc));
      setLoad((prev) => (prev ? { ...prev, node: updated, initialDoc: result.doc } : prev));
      setCharCount(updated.word_count);
      await refreshMentioned(load.node.id);
      setSaveStatus({ kind: "saved", at: Date.now() });
      setAutoMentionFound(0);
      showToast(t("workspace.toast.autoMentionApplied", { count: result.applied }));
    } catch (e) {
      setSaveStatus({ kind: "error", message: String(e) });
      showToast(t("workspace.toast.scanSceneFailed", { error: String(e) }));
    } finally {
      setAutoMentionBusy(false);
    }
  }, [debouncedSave, load, refreshMentioned, sceneSaveQueue, showToast, t]);

  const handleSceneTitleCommit = useCallback(
    async (title: string) => {
      if (!load) return;
      const nextTitle = title === load.node.label ? "" : title;
      try {
        await nodes.rename(load.node.id, load.node.label, nextTitle);
        await refreshTreeKeepNode(load.node.id);
      } catch (e) {
        showToast(t("workspace.toast.sceneTitleSaveFailed", { error: String(e) }));
        throw e;
      }
    },
    [load, refreshTreeKeepNode, showToast, t],
  );

  const handleProjectTitleCommit = useCallback(
    async (title: string) => {
      if (!load) return;
      try {
        const updated = await projects.update({ id: load.project.id, title });
        setLoad((prev) => (prev ? { ...prev, project: updated } : prev));
      } catch (e) {
        showToast(t("workspace.toast.novelTitleSaveFailed", { error: String(e) }));
        throw e;
      }
    },
    [load, showToast, t],
  );


  const toggleFactBook = useCallback(() => {
    setFactBookSelectedClaimRequest(null);
    setFactBookOpen((v) => {
      const next = !v;
      if (next) {
            setContextualEditOpen(false);
        setCanonOpen(false);
        setAgentOpen(false);
        setEntitySheetId(null);
        setThreadSheetId(null);
          }
      return next;
    });
  }, []);

  const toggleContextualEdit = useCallback(() => {
    setContextualEditOpen((v) => {
      const next = !v;
      if (next) {
        setContextualSeed(null);
        setFactBookSelectedClaimRequest(null);
        setFactBookOpen(false);
        setCanonOpen(false);
        setAgentOpen(false);
            setEntitySheetId(null);
        setThreadSheetId(null);
          }
      return next;
    });
  }, []);

  const toggleCanon = useCallback(() => {
    setCanonOpen((v) => {
      const next = !v;
      if (next) {
        setFactBookSelectedClaimRequest(null);
        setFactBookOpen(false);
        setContextualEditOpen(false);
        setAgentOpen(false);
        setEntitySheetId(null);
        setThreadSheetId(null);
      }
      return next;
    });
  }, []);

  const toggleAgent = useCallback(() => {
    setAgentOpen((v) => {
      const next = !v;
      if (next) {
        setFactBookSelectedClaimRequest(null);
        setFactBookOpen(false);
        setContextualEditOpen(false);
        setCanonOpen(false);
        setEntitySheetId(null);
        setThreadSheetId(null);
      }
      return next;
    });
  }, []);

  const runSelectionFactCheck = useCallback(() => {
    if (!selectionMenu || selectionMenu.kind !== "selection") return;
    editorRef.current?.setSelection({ from: selectionMenu.from, to: selectionMenu.to });
    setSelectionMenu(null);
    setContextualEditOpen(false);
    setEntitySheetId(null);
    setThreadSheetId(null);
    setFactBookOpen(true);
    setFactBookSelectedClaimRequest({
      id: `selection-${++factBookSelectionSeqRef.current}`,
      claim: selectionMenu.text,
    });
  }, [selectionMenu]);


  const copyNodeText = useCallback(async (node: Pick<TreeNode, "id">) => {
    try {
      const payload = await exportApi.nodeText(node.id);
      const copyProfile = normalizePlatformProfile(settingsRow?.copy_profile);
      const text = transformPlatformText(payload.text, copyProfile);
      if (!navigator.clipboard?.writeText) {
        throw new Error("clipboard unavailable");
      }
      await navigator.clipboard.writeText(text);
      showToast(t("workspace.toast.copyTextSuccessWithProfile", {
        count: Array.from(text).length.toLocaleString(locale),
        profile: t(`platformProfile.${copyProfile}`),
      }));
    } catch (e) {
      showToast(t("workspace.toast.copyTextFailed", { error: String(e) }));
    }
  }, [locale, settingsRow?.copy_profile, showToast, t]);

  // --- Commands ---

  // Cmd+P palette uses a fixed 7-section vocabulary. Every `cmds.push({ section })`
  // call below MUST use one of these exact strings — adding an 8th label fractures
  // the palette's mental model and reverts the Phase-15 cleanup. The labels are:
  //   이동 · 보기 · 노드 · 엔티티 · 프로젝트 · 내보내기 · 도움말
  // "AI" was the eighth; its only command was the companion toggle.
  // Hints should be either a keyboard shortcut (e.g. "Cmd+S", "ESC") or a brief
  // status note (e.g. "복원", "ESC로 종료"). Omit `hint` rather than putting noise.
  const commands: Command[] = useMemo(() => {
    if (!load) return [];
    const { prev, next } = leafNeighbors(load.tree, load.node.id);
    const allNodes = flatten(load.tree);
    const siblingsOfCurrent = allNodes.filter(
      (n) => (n.parent_id ?? null) === (load.node.parent_id ?? null),
    );
    const leafSiblings = siblingsOfCurrent.filter((n) => n.kind === "leaf");
    const nextSceneLabel = outlineNumberLabel(outlinePreset, "scene", leafSiblings.length + 1, t);
    const currentTreeNode = allNodes.find((n) => n.id === load.node.id) ?? ({ ...load.node, children: [] } as TreeNode);
    const parentContainer = currentTreeNode.parent_id
      ? allNodes.find((n) => n.id === currentTreeNode.parent_id && n.kind === "container")
      : undefined;
    const copyEpisodeTarget = parentContainer ?? currentTreeNode;
    const chapterReference = currentTreeNode.kind === "leaf" && parentContainer ? parentContainer : currentTreeNode;
    const chapterSiblings = allNodes.filter(
      (n) => (n.parent_id ?? null) === (chapterReference.parent_id ?? null) && n.kind === "container",
    );
    const nextChapterLabel = outlineNumberLabel(outlinePreset, "chapter", chapterSiblings.length + 1, t);
    const sectionNavigation = t("workspace.command.section.navigation");
    const sectionNode = t("workspace.command.section.node");
    const sectionView = t("workspace.command.section.view");
    const sectionProject = t("workspace.command.section.project");
    const sectionExport = t("workspace.command.section.export");
    const sectionHelp = t("workspace.command.section.help");
    const noTarget = t("workspace.command.none");

    const cmds: Command[] = [];
    cmds.push({
      id: "go-prev",
      section: sectionNavigation,
      label: t("workspace.command.prevScene"),
      hint: prev ? displayNodeLabel(language, prev.label) : noTarget,
      disabled: !prev,
      run: async () => { if (prev) await navigateToNode(prev); },
    });
    cmds.push({
      id: "go-next",
      section: sectionNavigation,
      label: t("workspace.command.nextScene"),
      hint: next ? displayNodeLabel(language, next.label) : noTarget,
      disabled: !next,
      run: async () => { if (next) await navigateToNode(next); },
    });
    cmds.push({
      id: "global-search",
      section: sectionNavigation,
      label: t("workspace.command.globalSearch"),
      hint: "Cmd+F",
      run: () => setSearchOpen(true),
    });
    for (const leaf of allNodes.filter((n) => n.kind === "leaf").slice(0, 20)) {
      const leafLabel = displayNodeLabel(language, leaf.label);
      cmds.push({
        id: `go-${leaf.id}`,
        section: sectionNavigation,
        label: t("workspace.command.goToScene", { label: leafLabel }),
        hint: leaf.title || undefined,
        disabled: leaf.id === load.node.id,
        run: async () => navigateToNode(leaf),
      });
    }

    cmds.push({
      id: "new-scene",
      section: sectionNode,
      label: t("workspace.command.newSceneBeside", { label: nextSceneLabel }),
      run: async () => { await handleCreateSceneFromOutline(currentTreeNode); },
    });
    cmds.push({
      id: "new-chapter",
      section: sectionNode,
      label: t("workspace.command.newChapterBeside", { label: nextChapterLabel }),
      run: async () => { await handleCreateChapterFromOutline(currentTreeNode); },
    });
    cmds.push({
      id: "mark-thread",
      section: sectionNode,
      label: t("workspace.markSceneAsThread"),
      run: async () => {
        const name = await promptDialog(t("workspace.prompt.storylineName"), load.node.title || load.node.label);
        if (name === null) return;
        const trimmed = name.trim();
        if (!trimmed) return;
        try {
          const t = await threadsApi.create({ project_id: load.project.id, name: trimmed, color: "#666" });
          // 곧바로 현재 씬에 바인딩된 첫 마디를 만들어두지 않으면, 사용자가
          // ThreadSheet 안에서 직접 "+ 새 마디 추가"로 만든 마디는 unbound라
          // 이 씬의 활성 Thread 패널에 표시되지 않는다. seed 책임을 시트에서
          // 명령으로 옮겨 이 함정을 제거한다.
          await beatsApi.create({ thread_id: t.id, node_id: load.node.id, label: "" });
          setThreadSheetId(t.id);
        } catch (e) {
          showToast(t("workspace.toast.storylineCreateFailed", { error: String(e) }));
        }
      },
    });
    cmds.push({
      id: "add-note",
      section: sectionNode,
      label: t("workspace.command.addMarginNote"),
      run: async () => {
        const body = await promptDialog(t("workspace.prompt.noteBody"), "");
        if (body === null) return;
        const trimmed = body.trim();
        if (!trimmed) return;
        const sel = editorRef.current?.getSelection();
        const anchor = sel?.from ?? 0;
        try {
          const created = await notesApi.create({
            node_id: load.node.id,
            anchor,
            body: trimmed,
          });
          if (sel) editorRef.current?.setSelection(sel);
          editorRef.current?.addNoteMarker(created.id);
        } catch (e) {
          showToast(t("workspace.toast.noteCreateFailed", { error: String(e) }));
        }
      },
    });
    cmds.push({
      id: "rename",
      section: sectionNode,
      label: t("workspace.command.renameNode"),
      run: () => { requestInlineRenameNode(currentTreeNode); },
    });
    cmds.push({
      id: "delete",
      section: sectionNode,
      label: t("workspace.delete"),
      run: async () => { await handleDeleteSceneFromOutline(currentTreeNode); },
    });
    cmds.push({
      id: "move-up",
      section: sectionNode,
      label: t("workspace.command.moveSceneUp"),
      run: async () => { await handleMoveSceneFromOutline(currentTreeNode, "up"); },
    });
    cmds.push({
      id: "move-down",
      section: sectionNode,
      label: t("workspace.command.moveSceneDown"),
      run: async () => { await handleMoveSceneFromOutline(currentTreeNode, "down"); },
    });
    cmds.push({
      id: "view-outline",
      section: sectionView,
      label: railCollapsed ? t("workspace.outlineExpand") : t("workspace.outlineCollapse"),
      run: () => setRailCollapsed((v) => !v),
    });
    cmds.push({
      id: "view-threads",
      section: sectionView,
      label: t("workspace.command.flowThreadView"),
      run: () => navigate(`/workspace/${load.project.id}/threads`),
    });
    cmds.push({
      id: "version-restore",
      section: sectionProject,
      label: t("workspace.command.sceneVersions"),
      hint: t("workspace.command.sceneVersionsHint"),
      keywords: ["history", "rollback", "diff", "version", "restore", "히스토리", "버전", "복원", "비교"],
      run: () => setVersionSheetNodeId(load.node.id),
    });
    cmds.push({
      id: "export-project",
      section: sectionExport,
      label: t("workspace.command.exportProject"),
      run: async () => {
        try {
          const payload = await exportApi.project(load.project.id);
          const path = await saveExportedMarkdown(payload);
          if (path) showToast(exportDestinationMessage(t, path));
        } catch (e) {
          showToast(t("workspace.toast.exportFailed", { error: String(e) }));
        }
      },
    });
    cmds.push({
      id: "export-node",
      section: sectionExport,
      label: t("workspace.command.exportScene"),
      run: async () => {
        try {
          const payload = await exportApi.node(load.node.id);
          const path = await saveExportedMarkdown(payload);
          if (path) showToast(exportDestinationMessage(t, path));
        } catch (e) {
          showToast(t("workspace.toast.exportFailed", { error: String(e) }));
        }
      },
    });
    if (outlinePreset.id === "webnovel") {
      cmds.push({
        id: "copy-episode-text",
        section: sectionExport,
        label: t("workspace.command.copyEpisodeText"),
        run: async () => { await copyNodeText(copyEpisodeTarget); },
      });
    }
    cmds.push({
      id: "copy-scene-text",
      section: sectionExport,
      label: t("workspace.command.copySceneText"),
      run: async () => { await copyNodeText(currentTreeNode); },
    });
    cmds.push({
      id: "go-settings",
      section: sectionProject,
      label: t("workspace.command.openSettings"),
      run: () => navigate("/settings"),
    });
    if (gitSyncAvailable) {
      cmds.push({
        id: "git-sync-now",
        section: sectionProject,
        label: t("workspace.command.syncNow"),
        run: async () => {
          try {
            const res = await gitSync.run();
            if (res.skipped) {
              showToast(t("workspace.toast.syncDisabled"));
              return;
            }
            if (res.error) {
              showToast(t("workspace.toast.syncError", { error: res.error }));
              return;
            }
            if (!res.committed) {
              showToast(t("workspace.toast.syncNoChanges", { count: res.files_written }));
              return;
            }
            if (res.pushed) {
              showToast(t("workspace.toast.syncPushed", { count: res.files_written }));
            } else {
              showToast(t("workspace.toast.syncCommittedPushFailed", { count: res.files_written }));
            }
          } catch (e) {
            showToast(t("workspace.toast.syncFailed", { error: String(e) }));
          }
        },
      });
    }
    cmds.push({
      id: "enter-zen",
      section: sectionView,
      label: t("workspace.command.openZen"),
      hint: t("workspace.command.exitEsc"),
      run: enterZen,
    });
    cmds.push({
      id: "toggle-focus",
      section: sectionView,
      label: focus ? t("workspace.command.toggleFocusOff") : t("workspace.command.toggleFocusOn"),
      run: () => setFocus((v) => !v),
    });
    cmds.push({
      id: "toggle-contextual-edit",
      section: sectionView,
      label: t("workspace.command.contextualEdit"),
      hint: "Cmd+Shift+F",
      run: toggleContextualEdit,
    });
    cmds.push({
      id: "toggle-canon",
      section: sectionProject,
      label: t("canon.title"),
      run: toggleCanon,
    });
    cmds.push({
      id: "toggle-fact-book",
      section: sectionProject,
      label: t("factBook.title"),
      run: toggleFactBook,
    });
    cmds.push({
      id: "show-shortcuts",
      section: sectionHelp,
      label: t("workspace.command.shortcutHelp"),
      run: () => setShortcutsOpen(true),
    });
    return cmds;
  }, [load, navigateToNode, navigate, promptDialog, enterZen, focus, railCollapsed, outlinePreset, handleCreateSceneFromOutline, handleCreateChapterFromOutline, requestInlineRenameNode, handleMoveSceneFromOutline, handleDeleteSceneFromOutline, copyNodeText, showToast, language, t, toggleFactBook, toggleContextualEdit, toggleCanon, gitSyncAvailable]);

  // Breadcrumb chain: ancestor container labels + the current scene label.
  const crumbChain = useMemo(() => {
    if (!load) return [] as string[];
    const byId = new Map(flatten(load.tree).map((n) => [n.id, n] as const));
    const chain: string[] = [];
    let cur: TreeNode | undefined = byId.get(load.node.id);
    while (cur) {
      chain.unshift(displayNodeLabel(language, cur.label));
      cur = cur.parent_id ? byId.get(cur.parent_id) : undefined;
    }
    return chain;
  }, [language, load]);

  // Scene-marker text shown above the editor title (uppercase mono path).
  const sceneMarker = useMemo(
    () => crumbChain.join(" · "),
    [crumbChain],
  );
  const isWebnovelProject = outlinePresetId === "webnovel";
  const episodeCharTarget = load?.project.episode_char_target ?? 5000;
  const currentEpisodeCharCount = useMemo(() => {
    if (!load || !isWebnovelProject) return charCount;
    const current = flatten(load.tree).find((n) => n.id === load.node.id);
    const episode = findEpisodeNode(load.tree, load.node.id, outlinePreset);
    if (!current || !episode) return charCount;
    return Math.max(0, sumLeafChars(episode) - current.word_count + charCount);
  }, [charCount, isWebnovelProject, load, outlinePreset]);
  const episodeStock = useMemo(
    () => (load && isWebnovelProject ? countEpisodeStatus(load.tree) : null),
    [isWebnovelProject, load],
  );

  const tourSteps: OnboardingTourStep[] = [
    {
      target: "workspace-outline",
      title: t("onboarding.workspace.outline.title"),
      body: t("onboarding.workspace.outline.body"),
    },
    {
      target: "workspace-editor",
      title: t("onboarding.workspace.editor.title"),
      body: t("onboarding.workspace.editor.body"),
    },
    {
      target: "workspace-context",
      title: t("onboarding.workspace.context.title"),
      body: t("onboarding.workspace.context.body"),
    },
    {
      target: "workspace-zen",
      title: t("onboarding.workspace.zen.title"),
      body: t("onboarding.workspace.zen.body"),
    },
    {
      target: "workspace-navigation",
      title: t("onboarding.workspace.navigation.title"),
      body: t("onboarding.workspace.navigation.body"),
    },
  ];

  const finishWorkspaceTour = useCallback(async () => {
    clearStoredPhase(WORKSPACE_PENDING_STORAGE_KEY);
    clearStoredPhase(MANUAL_PHASE_STORAGE_KEY);
    setSettingsRow((current) => current
      ? { ...current, onboarding_tour_seen_version: CURRENT_ONBOARDING_TOUR_VERSION }
      : current);
    setTourOpen(false);
    try {
      const next = await settingsApi.set({ onboarding_tour_seen_version: CURRENT_ONBOARDING_TOUR_VERSION });
      setSettingsRow(next);
    } catch (err) {
      showToast(t("workspace.toast.contextLoadFailed", { error: String(err) }));
    }
  }, [showToast, t]);

  useEffect(() => {
    const blocked = !load ||
      tourOpen ||
      contextualEditOpen ||
      entitySheetId !== null ||
      threadSheetId !== null ||
      versionSheetNodeId !== null ||
      zenOpen ||
      paletteOpen ||
      searchOpen ||
      shortcutsOpen ||
      dialog !== null ||
      notePopover !== null;
    if (blocked || !settingsRow) return;
    const manual = readStoredPhase(MANUAL_PHASE_STORAGE_KEY);
    const pending = readStoredPhase(WORKSPACE_PENDING_STORAGE_KEY);
    if (manual === "workspace" || pending === "workspace" || shouldAutoStartOnboarding(settingsRow)) {
      setTourOpen(true);
    }
  }, [
    contextualEditOpen,
    dialog,
    entitySheetId,
    load,
    notePopover,
    paletteOpen,
    searchOpen,
    settingsRow,
    shortcutsOpen,
    threadSheetId,
    tourOpen,
    versionSheetNodeId,
    zenOpen,
  ]);

  if (error) {
    return (
      <main className="shell">
        <p><Link to="/">← {t("settings.backToLibrary")}</Link></p>
        <p className="error">{error}</p>
      </main>
    );
  }
  if (!load) {
    return (
      <main className="shell">
        <p className="hint">{t("common.loading")}</p>
      </main>
    );
  }

  const currentNodeLabel = displayNodeLabel(language, load.node.label);
  const currentSceneTitle = load.node.title || currentNodeLabel;
  const handleOutlineSelect = (n: TreeNode) => {
    navigateToNode(n);
    if (window.matchMedia("(max-width: 860px)").matches || sizeClass === "ipad") setRailCollapsed(true);
  };

  return (
    <main className="workspace">
      <header className="ws-top">
        <Link to="/" className="ws-crumb">
          <span className="home"><ArrowLeft size={16} /></span>
          <span className="ws-crumb-path">
            <b>{load.project.title}</b>
            {crumbChain.map((label, i) => (
              <span key={i} className="label-inline">
                <span className="sep">›</span>{label}
              </span>
            ))}
            {load.node.title && (
              <>
                <span className="sep">—</span>
                <span className="title">{load.node.title}</span>
              </>
            )}
          </span>
        </Link>
        <div className="ws-top-actions" data-tour="workspace-navigation">
          <McpToggle projectId={load.project.id} projectTitle={load.project.title} />
          <button
            type="button"
            className="ws-tool icon-only mobile-outline-toggle"
            title={t("workspace.outline")}
            onClick={() => setRailCollapsed((v) => !v)}
          >
            <Menu size={16} />
          </button>
          <button
            type="button"
            className="ws-tool icon-only"
            title={t("workspace.searchShortcut")}
            onClick={() => setSearchOpen(true)}
          >
            <Search size={16} />
          </button>
          <button
            type="button"
            className="ws-tool icon-only"
            title={t("workspace.commandPaletteShortcut")}
            onClick={() => setPaletteOpen((v) => !v)}
          >
            <CommandIcon size={16} />
          </button>
          {sizeClass === "ipad" && (
            <button
              type="button"
              className="ws-tool icon-only ipad-shortcuts-toggle"
              aria-label={t("shortcuts.helpLabel")}
              onClick={() => setShortcutsOpen(true)}
            >
              <Keyboard size={16} />
            </button>
          )}
          <div className="ws-sep" />
          <button
            type="button"
            className={`ws-tool${contextualEditOpen ? " is-active" : ""}`}
            onClick={toggleContextualEdit}
            title={t("workspace.command.contextualEdit")}
          >
            <Replace size={15} /> {t("contextual.title")}
          </button>
          <button
            type="button"
            className={`ws-tool${canonOpen ? " is-active" : ""}`}
            onClick={toggleCanon}
            title={t("canon.title")}
          >
            <Library size={15} /> {t("canon.title")}
          </button>
          <button
            type="button"
            className={`ws-tool${factBookOpen ? " is-active" : ""}`}
            onClick={toggleFactBook}
            title={t("factBook.title")}
          >
            <BookOpen size={15} /> {t("factBook.title")}
          </button>
          <div className="ws-sep" />
          <button type="button" className="ws-tool" onClick={enterZen} data-tour="workspace-zen">
            <Maximize2 size={15} /> ZEN
          </button>
        </div>
      </header>

      <div className={`ws-body${railCollapsed ? " rail-collapsed" : ""}${
        (factBookOpen || contextualEditOpen || canonOpen || agentOpen) ? " right-wide" : ""
      }${versionSheetNodeId ? " right-history" : ""}`}>
        {!railCollapsed && (
          <button
            type="button"
            className="mobile-rail-backdrop"
            aria-label={t("workspace.outlineCollapse")}
            onClick={() => setRailCollapsed(true)}
          />
        )}
        <OutlinePanel
          tree={load.tree}
          currentId={load.node.id}
          collapsed={railCollapsed}
          onToggleCollapse={() => setRailCollapsed((v) => !v)}
          onSelect={handleOutlineSelect}
          onRename={handleRenameNode}
          renameRequest={outlineRenameRequest}
          onCreateScene={handleCreateSceneFromOutline}
          onCreatePart={handleCreatePartFromOutline}
          onCreateChapter={handleCreateChapterFromOutline}
          onCopyText={copyNodeText}
          onMoveNodeUp={(node) => handleMoveSceneFromOutline(node, "up")}
          onMoveNodeDown={(node) => handleMoveSceneFromOutline(node, "down")}
          onMoveNode={handleMoveNodeTo}
          onDeleteNode={handleDeleteSceneFromOutline}
          onSetStatus={handleSetNodeStatus}
          onRepairOutline={handleRepairOutline}
          onUndoRepairOutline={handleUndoRepairOutline}
          canUndoRepair={Boolean(outlineUndoSnapshot)}
          outlinePresetId={outlinePresetId}
          episodeCharTarget={episodeCharTarget}
          onOutlinePresetChange={handleOutlinePresetChange}
          tourTarget="workspace-outline"
        />
        <section className={`ws-editor${focus ? " focus-mode" : ""}`} data-tour="workspace-editor">
          <div className="editor-col">
            {conflictNodeId && (
              /* An agent rewrote the scene the writer is editing. Their
                 unsaved sentence outranks the agent's version, so nothing is
                 replaced until they say so. */
              <div className="mcp-conflict" role="status" data-testid="mcp-conflict">
                <span>
                  {conflictSource === "agent"
                    ? t("workspace.mcp.conflict.agentBody")
                    : t("workspace.mcp.conflict.body")}
                </span>
                <button
                  type="button"
                  className="btn ghost sm"
                  onClick={() => { void reloadSceneFromEngine(conflictNodeId); dismissConflict(); }}
                  data-testid="mcp-conflict-load"
                >
                  {t("workspace.mcp.conflict.load")}
                </button>
                <button type="button" className="btn ghost sm" onClick={dismissConflict}>
                  {t("workspace.mcp.conflict.keep")}
                </button>
              </div>
            )}
            <div className="scene-marker">
              <span>{sceneMarker}</span>
              <span className="rule" />
            </div>
            <h1 className="scene-title">
              <InlineEditableText
                value={currentSceneTitle}
                ariaLabel={t("workspace.sceneName")}
                className="scene-title-input"
                onCommit={handleSceneTitleCommit}
              />
            </h1>
            {load.node.title && <div className="scene-sub">{currentNodeLabel}</div>}
            <TiptapEditor
              key={`${load.node.id}:${load.node.content_version ?? load.node.updated_at}`}
              ref={editorRef}
              initialDoc={load.initialDoc}
              onChange={(doc) => {
                debouncedSave(load.node.id, doc);
                idleDirtyRef.current = true;
                setEditorDirty(true);
                setAutoMentionScanKey((k) => k + 1);
                markActivity();
                throttledLastOpened();
              }}
              onCharCount={setCharCount}
              typewriter={typewriter}
              focus={focus}
              onManualSave={handleManualSave}
              extensions={[
                ...(mentionExtension ? [mentionExtension] : []),
                NoteMarkerExtension,
              ]}
              onMentionDoubleClick={(id) => {
                setContextualEditOpen(false);
                setEntitySheetId(id);
              }}
              onSelectionContextMenu={openEditorSelectionMenu}
            />
            {selectionMenu && (
              <div
                role="menu"
                aria-label={t("workspace.selectionMenu.label")}
                className="editor-selection-menu"
                style={{ left: selectionMenu.x, top: selectionMenu.y }}
                onMouseDown={(e) => e.stopPropagation()}
                onClick={(e) => e.stopPropagation()}
              >
                <button type="button" role="menuitem" onClick={runSelectionFactCheck}>
                  <Search size={13} /> {t("workspace.selectionMenu.factCheck")}
                </button>
              </div>
            )}
          </div>
          <div className="editor-foot">
            <span>{currentSceneTitle}</span>
            <span>·</span>
            {isWebnovelProject ? (
              <>
                <span>
                  {t("workspace.episodeCharCount", {
                    count: currentEpisodeCharCount.toLocaleString(locale),
                    target: episodeCharTarget.toLocaleString(locale),
                  })}
                </span>
                <span>·</span>
                <span>{t("workspace.charCountWithSpaces")}</span>
              </>
            ) : (
              <span>{t("workspace.charCount", { count: charCount.toLocaleString(locale) })}</span>
            )}
          </div>
        </section>
        {versionSheetNodeId && load ? (
          <VersionSheet
            nodeId={versionSheetNodeId}
            onClose={() => {
              setVersionSheetNodeId(null);
              focusEditor();
            }}
            onRestored={(updatedNode) => {
              debouncedSave.cancel(updatedNode.id);
              sceneSaveQueue.seed(updatedNode.id, updatedNode.content_version ?? 0);
              const docStr = updatedNode.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`;
              setLoad((prev) => prev ? { ...prev, node: updatedNode, initialDoc: JSON.parse(docStr) } : prev);
              setCharCount(updatedNode.word_count);
              showToast(t("workspace.toast.versionRestored"));
            }}
          />
        ) : factBookOpen && load ? (
          <FactBookPanel
            projectId={load.project.id}
            nodeId={load.node.id}
            selectedClaimRequest={factBookSelectedClaimRequest}
            onImpactCheck={(text) => {
              setContextualSeed({ text, autoCheck: true });
              setFactBookOpen(false);
                        setEntitySheetId(null);
              setThreadSheetId(null);
                        setContextualEditOpen(true);
            }}
            onClose={() => {
              setFactBookOpen(false);
              setFactBookSelectedClaimRequest(null);
              focusEditor();
            }}
            onChanged={() => {
              refreshTreeKeepNode(load.node.id);
            }}
          />
        ) : contextualEditOpen && load ? (
          <ContextualEditPanel
            open={contextualEditOpen}
            projectId={load.project.id}
            currentNodeId={load.node.id}
            editorRef={editorRef}
            initialEntityId={contextualSeed?.entityId ?? null}
            initialText={contextualSeed?.text ?? null}
            autoCheckInitialText={Boolean(contextualSeed?.autoCheck)}
            onNavigateNode={(nodeId) => {
              void navigateToNode({ id: nodeId } as NodeRow);
            }}
            onBatchApplied={() => {
              void refreshTreeKeepNode(load.node.id);
              setSaveStatus({ kind: "saved", at: Date.now() });
            }}
            onClose={() => {
              setContextualEditOpen(false);
              setContextualSeed(null);
              focusEditor();
            }}
          />
        ) : entitySheetId ? (
          <EntitySheet
            entityId={entitySheetId}
            onClose={() => {
              setEntitySheetId(null);
              if (load) refreshMentioned(load.node.id);
              focusEditor();
            }}
            onSaved={() => {
              if (load) refreshMentioned(load.node.id);
            }}
            onContextChange={(entityId) => {
              setContextualSeed({ entityId });
              setEntitySheetId(null);
              setFactBookOpen(false);
                        setThreadSheetId(null);
                        setContextualEditOpen(true);
            }}
            onNavigate={(nodeId) => {
              setEntitySheetId(null);
              navigateToNode({ id: nodeId } as NodeRow);
            }}
          />
        ) : threadSheetId ? (
          <ThreadSheet
            threadId={threadSheetId}
            onClose={() => {
              setThreadSheetId(null);
              focusEditor();
            }}
            onSaved={() => {
              /* PlotPanel self-reloads */
            }}
          />
        ) : canonOpen && load ? (
          // Below the sheets on purpose: opening a record from the list hands
          // the slot to EntitySheet, and closing the sheet lands back here
          // instead of dumping the writer in the editor.
          <CanonPanel
            projectId={load.project.id}
            refreshKey={canonRefreshKey}
            onOpenEntity={(entityId) => setEntitySheetId(entityId)}
            onClose={() => {
              setCanonOpen(false);
              focusEditor();
            }}
          />
        ) : agentOpen && load ? (
          <AgentPanel
            onClose={() => {
              setAgentOpen(false);
              focusEditor();
            }}
          />
        ) : sizeClass === "desktop" ? (
          <ContextPanel
            project={load.project}
            node={load.node}
            charCount={charCount}
            todayChars={todayChars}
            episodeStock={episodeStock}
            statsRefreshKey={saveCompletedAt}
            typewriter={typewriter}
            onToggleTypewriter={() => setTypewriter((v) => !v)}
            saveStatus={saveStatus}
            mentionedEntities={mentioned}
            onMentionClick={(id) => setEntitySheetId(id)}
            onAutoMention={handleAutoMentionScene}
            autoMentionBusy={autoMentionBusy}
            autoMentionFound={autoMentionFound}
            onOpenThread={setThreadSheetId}
            onProjectTitleChange={handleProjectTitleCommit}
            onProjectChanged={(p) =>
              setLoad((prev) => (prev ? { ...prev, project: p } : prev))
            }
            tourTarget="workspace-context"
          />
        ) : null}
      </div>

      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        commands={commands}
      />

      <SearchModal
        open={searchOpen}
        onClose={() => setSearchOpen(false)}
        onSelect={handleSearchSelect}
      />

      <MentionPicker state={mentionState} />

      {dialog && (
        <DialogModal
          dialog={dialog}
          onClose={() => {
            setDialog(null);
            focusEditor();
          }}
        />
      )}

      {zenOpen && (
        <ZenMode
          initialDoc={load.initialDoc}
          initialSelection={savedSelectionRef.current}
          charCount={charCount}
          episodeCharCount={currentEpisodeCharCount}
          sceneLabel={currentNodeLabel}
          target={isWebnovelProject ? episodeCharTarget : 0}
          onChange={(doc) => {
            debouncedSave(load.node.id, doc);
            idleDirtyRef.current = true;
            markActivity();
          }}
          onCharCount={setCharCount}
          onManualSave={handleManualSave}
          onMountEditor={(h) => { zenEditorRef.current = h; }}
          onExit={exitZen}
        />
      )}

      {notePopover && (
        <NotePopover
          noteId={notePopover.noteId}
          targetEl={notePopover.targetEl}
          mode={notePopover.mode}
          onClose={() => setNotePopover(null)}
          onDeleted={(id) => { editorRef.current?.removeNoteMarker(id); }}
        />
      )}

      <ShortcutsModal open={shortcutsOpen} onClose={() => setShortcutsOpen(false)} />
      <OnboardingTour
        open={tourOpen}
        steps={tourSteps}
        onFinish={finishWorkspaceTour}
        onSkip={finishWorkspaceTour}
      />
    </main>
  );
}

function DialogModal({ dialog, onClose }: { dialog: DialogState; onClose: () => void }) {
  const { t } = useI18n();
  const [value, setValue] = useState(dialog.kind === "prompt" ? dialog.initial : "");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (dialog.kind === "prompt") {
      setValue(dialog.initial);
      window.setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [dialog]);

  const handleConfirm = () => {
    if (dialog.kind === "prompt") dialog.resolve(value);
    else dialog.resolve(true);
    onClose();
  };

  const handleCancel = () => {
    if (dialog.kind === "prompt") dialog.resolve(null);
    else dialog.resolve(false);
    onClose();
  };

  return (
    <div className="dialog-backdrop" onClick={handleCancel}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <p className="dialog-title">{dialog.title}</p>
        {dialog.kind === "prompt" && (
          <input
            ref={inputRef}
            className="dialog-input"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                handleConfirm();
              } else if (e.key === "Escape") {
                e.preventDefault();
                handleCancel();
              }
            }}
          />
        )}
        <div className="dialog-actions">
          <button type="button" onClick={handleCancel}>{t("common.cancel")}</button>
          <button type="button" className="primary" onClick={handleConfirm}>{t("common.confirm")}</button>
        </div>
      </div>
    </div>
  );
}
