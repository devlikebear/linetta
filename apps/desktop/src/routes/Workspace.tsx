import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { Search, Command as CommandIcon, Sparkles, MessageCircle, Maximize2, ArrowLeft } from "lucide-react";
import { nodes, projects, snapshots, entities as entitiesApi, mentions as mentionsApi, threads as threadsApi, beats as beatsApi, settings as settingsApi, exportApi, notes as notesApi, gitSync, ai as aiApi } from "../lib/rpc";
import { NoteMarkerExtension } from "../components/editor/NoteMarkerExtension";
import { AITargetExtension } from "../components/editor/AITargetExtension";
import { NotePopover } from "../components/NotePopover";
import type { NodeRow, Project, Entity, AIContextPreview, AIOptions, ContextCounts, SearchResult } from "../lib/types";
import { buildMentionExtension, type MentionPickerState } from "../components/editor/MentionExtension";
import { MentionPicker } from "../components/editor/MentionPicker";
import { EntitySheet } from "../components/EntitySheet";
import { ThreadSheet } from "../components/ThreadSheet";
import { VersionSheet } from "../components/VersionSheet";
import { saveExportedMarkdown } from "../lib/exportSave";
import { TiptapEditor, type TiptapHandle } from "../components/editor/Tiptap";
import { useAIGeneration } from "../lib/editor/useAIGeneration";
import { AIPanel } from "../components/ai/AIPanel";
import { commitGenerated, type CommitMode } from "../lib/editor/commitGenerated";
import { autoMentionDoc } from "../lib/editor/autoMention";
import { DEFAULT_AI_CONTEXT_SELECTION, totalContextItems } from "../components/ai/AIContextChecklist";
import { ZenMode } from "../components/ZenMode";
import { ContextPanel, type SaveStatus } from "../components/ContextPanel";
import { CompanionPanel } from "../components/companion/CompanionPanel";
import { OutlinePanel } from "../components/OutlinePanel";
import { InlineEditableText } from "../components/InlineEditableText";
import { CommandPalette, type Command } from "../components/CommandPalette";
import { ShortcutsModal } from "../components/ShortcutsModal";
import { SearchModal } from "../components/SearchModal";
import { useDebouncedCallback } from "../hooks/useDebouncedCallback";
import { useThrottledCallback } from "../hooks/useThrottledCallback";
import { useToast } from "../components/ToastProvider";
import { displayNodeLabel, localeForLanguage, useI18n } from "../lib/i18n";
import {
  buildTree,
  findFirstLeaf,
  flatten,
  leafNeighbors,
  type TreeNode,
} from "../hooks/useFirstLeaf";

const SAVE_DEBOUNCE_MS = 800;
const LAST_OPENED_THROTTLE_MS = 5000;

const FALLBACK_COUNTS: ContextCounts = {
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
};

const FALLBACK_CONTEXT_PREVIEW: AIContextPreview = {
  counts: FALLBACK_COUNTS,
  sections: [],
  selectedItemCount: 0,
};

interface LoadState {
  project: Project;
  node: NodeRow;
  initialDoc: object;
  tree: TreeNode[];
}

type DialogState =
  | { kind: "prompt"; title: string; initial: string; resolve: (v: string | null) => void }
  | { kind: "confirm"; title: string; resolve: (v: boolean) => void };

export function Workspace() {
  const { projectId } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const [load, setLoad] = useState<LoadState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { showToast } = useToast();
  const { language, t } = useI18n();
  const locale = localeForLanguage(language);
  const [charCount, setCharCount] = useState(0);
  const [typewriter, setTypewriter] = useState(false);
  const [focus, setFocus] = useState(false);
  const [railCollapsed, setRailCollapsed] = useState(false);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>({ kind: "idle" });
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [versionSheetNodeId, setVersionSheetNodeId] = useState<string | null>(null);
  const [zenOpen, setZenOpen] = useState(false);
  const [dialog, setDialog] = useState<DialogState | null>(null);
  const [mentionState, setMentionState] = useState<MentionPickerState | null>(null);
  const [entitySheetId, setEntitySheetId] = useState<string | null>(null);
  const [threadSheetId, setThreadSheetId] = useState<string | null>(null);
  const [companionOpen, setCompanionOpen] = useState(false);
  const companionNodeRef = useRef<string | null>(null);
  const [mentioned, setMentioned] = useState<Entity[]>([]);
  const [autoMentionBusy, setAutoMentionBusy] = useState(false);
  const [aiOptions, setAiOptions] = useState<AIOptions>({
    tone: "my",
    short_form: true,
    context: DEFAULT_AI_CONTEXT_SELECTION,
  });
  const [aiModal, setAiModal] = useState<{
    mode: CommitMode;
    canChooseMode: boolean;
    sel: { from: number; to: number };
  } | null>(null);
  const aiModalOpenRef = useRef(false);
  useEffect(() => { aiModalOpenRef.current = aiModal !== null; }, [aiModal]);
  const closeAIModalRef = useRef<(() => void) | null>(null);
  const openAIModalRef = useRef<(() => void) | null>(null);
  const [aiCtxChecklistOpen, setAiCtxChecklistOpen] = useState(false);
  const [contextPreview, setContextPreview] = useState<AIContextPreview | null>(null);
  const previewReqIdRef = useRef(0);
  const loadRef = useRef<LoadState | null>(null);
  useEffect(() => {
    loadRef.current = load;
  }, [load]);
  useEffect(() => { companionNodeRef.current = load?.node.id ?? null; }, [load]);
  const editorRef = useRef<TiptapHandle>(null);
  const savedSelectionRef = useRef<{ from: number; to: number } | null>(null);
  const zenEditorRef = useRef<TiptapHandle | null>(null);
  const [notePopover, setNotePopover] = useState<{
    noteId: string;
    targetEl: HTMLElement;
    mode: "read" | "edit";
  } | null>(null);

  useEffect(() => {
    const onHover = (e: Event) => {
      const ce = e as CustomEvent<{ noteId: string; target: HTMLElement }>;
      setNotePopover((cur) => {
        if (cur && cur.mode === "edit" && cur.noteId === ce.detail.noteId) return cur;
        return { noteId: ce.detail.noteId, targetEl: ce.detail.target, mode: "read" };
      });
    };
    const onHoverEnd = (e: Event) => {
      const ce = e as CustomEvent<{ noteId: string }>;
      setNotePopover((cur) => {
        if (!cur) return null;
        if (cur.mode === "edit") return cur;
        if (cur.noteId !== ce.detail.noteId) return cur;
        return null;
      });
    };
    const onClick = (e: Event) => {
      const ce = e as CustomEvent<{ noteId: string; target: HTMLElement }>;
      setNotePopover({ noteId: ce.detail.noteId, targetEl: ce.detail.target, mode: "edit" });
    };
    window.addEventListener("linetta:note-hover", onHover);
    window.addEventListener("linetta:note-hover-end", onHoverEnd);
    window.addEventListener("linetta:note-click", onClick);
    return () => {
      window.removeEventListener("linetta:note-hover", onHover);
      window.removeEventListener("linetta:note-hover-end", onHoverEnd);
      window.removeEventListener("linetta:note-click", onClick);
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

  // Apply typewriter + focus defaults from settings exactly once on mount.
  useEffect(() => {
    let cancelled = false;
    settingsApi.get()
      .then((s) => {
        if (cancelled) return;
        setTypewriter(s.typewriter_default);
        setFocus(s.focus_default);
      })
      .catch(() => { /* benign */ });
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
  const refreshTreeKeepNode = useCallback(
    async (currentNodeId: string) => {
      if (!projectId) return;
      const fresh = await fetchTree(projectId, currentNodeId);
      setLoad(fresh);
    },
    [projectId, fetchTree],
  );

  const handleRenameNode = useCallback(
    async (target: TreeNode) => {
      const nextLabel = await promptDialog(t("workspace.prompt.renameLabel"), target.label);
      if (nextLabel === null) return;
      const label = nextLabel.trim();
      if (!label) return;
      const nextTitle = await promptDialog(t("workspace.prompt.displayTitle"), target.title);
      if (nextTitle === null) return;
      try {
        await nodes.rename(target.id, label, nextTitle.trim());
        await refreshTreeKeepNode(loadRef.current?.node.id ?? target.id);
        showToast(t("workspace.toast.renameSuccess"));
      } catch (e) {
        showToast(t("workspace.toast.renameFailed", { error: String(e) }));
      }
    },
    [promptDialog, refreshTreeKeepNode, showToast, t],
  );

  const handleCreateSceneFromOutline = useCallback(
    async (anchor: TreeNode) => {
      const current = loadRef.current;
      if (!current) return;
      try {
        const allNodes = flatten(current.tree);
        let created: NodeRow;
        if (anchor.kind === "container") {
          const childLeaves = anchor.children.filter((n) => n.kind === "leaf");
          created = await nodes.createChild(anchor.id, "leaf", t("workspace.sceneNumber", { number: childLeaves.length + 1 }), "");
        } else {
          const siblings = allNodes.filter((n) => (n.parent_id ?? null) === (anchor.parent_id ?? null));
          const leafCount = siblings.filter((n) => n.kind === "leaf").length;
          created = await nodes.createSibling(anchor.id, "leaf", t("workspace.sceneNumber", { number: leafCount + 1 }), "");
        }
        await refreshTreeAndNavigateTo(created.id);
      } catch (e) {
        showToast(t("workspace.toast.createSceneFailed", { error: String(e) }));
      }
    },
    [refreshTreeAndNavigateTo, showToast, t],
  );

  const handleCreateChapterFromOutline = useCallback(
    async (anchor: TreeNode) => {
      const current = loadRef.current;
      if (!current) return;
      try {
        const allNodes = flatten(current.tree);
        const parentContainer = anchor.parent_id
          ? allNodes.find((n) => n.id === anchor.parent_id && n.kind === "container")
          : undefined;
        const reference = anchor.kind === "leaf" && parentContainer ? parentContainer : anchor;
        const siblings = allNodes.filter((n) => (n.parent_id ?? null) === (reference.parent_id ?? null));
        const chapterCount = siblings.filter((n) => n.kind === "container").length;
        const chapter = await nodes.createSibling(reference.id, "container", t("workspace.chapterNumber", { number: chapterCount + 1 }), "");
        const seeded = await nodes.createChild(chapter.id, "leaf", t("workspace.sceneNumber", { number: 1 }), "");
        await refreshTreeAndNavigateTo(seeded.id);
      } catch (e) {
        showToast(t("workspace.toast.createChapterFailed", { error: String(e) }));
      }
    },
    [refreshTreeAndNavigateTo, showToast, t],
  );

  const handleMoveSceneFromOutline = useCallback(
    async (target: TreeNode, direction: "up" | "down") => {
      if (target.kind !== "leaf") return;
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

  const handleDeleteSceneFromOutline = useCallback(
    async (target: TreeNode) => {
      if (target.kind !== "leaf") return;
      const current = loadRef.current;
      if (!current) return;
      const ok = await confirmDialog(t("workspace.confirm.deleteScene", { label: displayNodeLabel(language, target.label) }));
      if (!ok) return;
      try {
        const { prev, next } = leafNeighbors(current.tree, target.id);
        const fallback = prev ?? next ?? null;
        await nodes.delete(target.id);
        if (target.id === current.node.id) {
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
  }, [projectId, fetchTree]);

  // Refresh mentioned entities when the active node changes.
  useEffect(() => {
    if (load) refreshMentioned(load.node.id);
  }, [load?.node.id, refreshMentioned]);

  // Listen for "new entity" event dispatched by MentionExtension.
  useEffect(() => {
    if (!projectId) return;
    const handler = async (e: Event) => {
      const detail = (e as CustomEvent).detail as { query: string; range: { from: number; to: number }; editor: any };
      try {
        const created = await entitiesApi.create({ project_id: projectId, kind: "character", name: detail.query });
        detail.editor.chain().focus().deleteRange(detail.range).insertContent([
          { type: "mention", attrs: { id: created.id, label: created.name } },
          { type: "text", text: " " },
        ]).run();
        setEntitySheetId(created.id);
      } catch (err) {
        showToast(t("workspace.toast.entityCreateFailed", { error: String(err) }));
      }
    };
    window.addEventListener("linetta:mention-pick-new", handler);
    return () => window.removeEventListener("linetta:mention-pick-new", handler);
  }, [projectId, showToast, t]);

  // Global Cmd+R reload + Cmd+P palette toggle + Cmd+F search + Cmd+I AI modal.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const isMac = navigator.platform.toLowerCase().includes("mac");
      const mod = isMac ? e.metaKey : e.ctrlKey;
      if (!mod) return;
      if (e.key.toLowerCase() === "r") {
        if (aiModalOpenRef.current) { e.preventDefault(); return; }
        e.preventDefault();
        window.location.reload();
      } else if (e.key.toLowerCase() === "p") {
        if (aiModalOpenRef.current) { e.preventDefault(); return; }
        e.preventDefault();
        setPaletteOpen((v) => !v);
      } else if (e.key.toLowerCase() === "f") {
        if (aiModalOpenRef.current) { e.preventDefault(); return; }
        e.preventDefault();
        setSearchOpen(true);
      } else if (e.key.toLowerCase() === "i") {
        e.preventDefault();
        if (aiModalOpenRef.current) {
          closeAIModalRef.current?.();
          return;
        }
        openAIModalRef.current?.();
      } else if (e.key.toLowerCase() === "j") {
        if (aiModalOpenRef.current) { e.preventDefault(); return; }
        e.preventDefault();
        setCompanionOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const saveNow = useCallback(
    async (doc: object) => {
      if (!load) return;
      setSaveStatus({ kind: "saving" });
      try {
        await nodes.updateContent(load.node.id, JSON.stringify(doc));
        setSaveStatus({ kind: "saved", at: Date.now() });
        refreshMentioned(load.node.id);
      } catch (e) {
        setSaveStatus({ kind: "error", message: String(e) });
        setError(String(e));
      }
    },
    [load, refreshMentioned],
  );
  const debouncedSave = useDebouncedCallback(saveNow, SAVE_DEBOUNCE_MS);
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
      setSaveStatus({ kind: "saving" });
      try {
        await nodes.updateContent(load.node.id, JSON.stringify(doc));
        await snapshots.createManual(load.node.id, JSON.stringify(doc));
        setSaveStatus({ kind: "saved", at: Date.now() });
        showToast(t("workspace.toast.snapshotSaved"));
      } catch (e) {
        setSaveStatus({ kind: "error", message: String(e) });
        setError(String(e));
      }
    },
    [load, showToast, t],
  );

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
        showToast(t("workspace.toast.autoMentionNone"));
        return;
      }
      setSaveStatus({ kind: "saving" });
      editor.commands.setContent(result.doc);
      const updated = await nodes.updateContent(load.node.id, JSON.stringify(result.doc));
      setLoad((prev) => (prev ? { ...prev, node: updated, initialDoc: result.doc } : prev));
      setCharCount(updated.word_count);
      await refreshMentioned(load.node.id);
      setSaveStatus({ kind: "saved", at: Date.now() });
      showToast(t("workspace.toast.autoMentionApplied", { count: result.applied }));
    } catch (e) {
      setSaveStatus({ kind: "error", message: String(e) });
      showToast(t("workspace.toast.scanSceneFailed", { error: String(e) }));
    } finally {
      setAutoMentionBusy(false);
    }
  }, [load, refreshMentioned, showToast, t]);

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

  // --- AI generation (Cmd+I modal) ---

  const tiptapEditor = editorRef.current?.editor ?? null;
  const gen = useAIGeneration();

  const closeAIModal = useCallback(() => {
    gen.cancel();
    if (tiptapEditor) {
      tiptapEditor.commands.clearAITarget();
      tiptapEditor.setEditable(true);
    }
    setAiModal(null);
    setContextPreview(null);
    setAiCtxChecklistOpen(false);
    previewReqIdRef.current++;
  }, [gen, tiptapEditor]);
  useEffect(() => { closeAIModalRef.current = closeAIModal; }, [closeAIModal]);

  // Open the Cmd+I AI generation modal targeting the current selection. Shared
  // by the keyboard shortcut and the top-bar AI button.
  const openAIModal = useCallback(() => {
    const ed = editorRef.current?.editor;
    const currentLoad = loadRef.current;
    if (!ed || !currentLoad) return;
    const { from, to, empty } = ed.state.selection;
    ed.setEditable(false);
    const mode = empty ? "insert" : "replace";
    ed.commands.setAITarget(mode, from, to);
    setAiModal({
      mode,
      canChooseMode: empty,
      sel: { from, to },
    });
    const reqId = ++previewReqIdRef.current;
    aiApi.previewContext(currentLoad.node.id, aiOptions)
      .then((preview) => {
        if (reqId !== previewReqIdRef.current) return;
        setContextPreview(preview);
      })
      .catch((err) => {
        if (reqId !== previewReqIdRef.current) return;
        showToast(t("workspace.toast.contextLoadFailed", { error: String(err) }));
      });
  }, [aiOptions, showToast, t]);
  useEffect(() => { openAIModalRef.current = openAIModal; }, [openAIModal]);

  // Toggle the AI modal — mirrors the Cmd+I keyboard behaviour for the top-bar button.
  const toggleAIModal = useCallback(() => {
    if (aiModalOpenRef.current) closeAIModal();
    else openAIModal();
  }, [closeAIModal, openAIModal]);

  // Defensive: if the active node changes while the modal is open, close it so
  // stale selection offsets can't be committed into a freshly-mounted editor.
  useEffect(() => {
    if (aiModalOpenRef.current) {
      closeAIModalRef.current?.();
    }
  }, [load?.node.id]);

  const acceptAIModal = useCallback(() => {
    if (!aiModal || !tiptapEditor) return;
    const v = gen.variations[gen.currentIdx];
    if (!v || v.error) return;
    tiptapEditor.commands.clearAITarget();
    commitGenerated(tiptapEditor, aiModal.mode, aiModal.sel, v.text);
    gen.cancel();
    tiptapEditor.setEditable(true);
    setAiModal(null);
    setContextPreview(null);
    setAiCtxChecklistOpen(false);
    previewReqIdRef.current++;
  }, [aiModal, gen, tiptapEditor]);

  // Safety: if the modal closes for any reason, re-enable editing.
  useEffect(() => {
    if (aiModal === null && tiptapEditor && !tiptapEditor.isEditable) {
      tiptapEditor.commands.clearAITarget();
      tiptapEditor.setEditable(true);
    }
  }, [aiModal, tiptapEditor]);

  // --- Commands ---

  // Cmd+P palette uses a fixed 8-section vocabulary. Every `cmds.push({ section })`
  // call below MUST use one of these exact strings — adding a 9th label fractures
  // the palette's mental model and reverts the Phase-15 cleanup. The labels are:
  //   이동 · 보기 · 노드 · 엔티티 · 프로젝트 · AI · 내보내기 · 도움말
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
    const nextSceneLabel = t("workspace.sceneNumber", { number: leafSiblings.length + 1 });
    const currentTreeNode = allNodes.find((n) => n.id === load.node.id) ?? ({ ...load.node, children: [] } as TreeNode);
    const parentContainer = currentTreeNode.parent_id
      ? allNodes.find((n) => n.id === currentTreeNode.parent_id && n.kind === "container")
      : undefined;
    const chapterReference = currentTreeNode.kind === "leaf" && parentContainer ? parentContainer : currentTreeNode;
    const chapterSiblings = allNodes.filter(
      (n) => (n.parent_id ?? null) === (chapterReference.parent_id ?? null) && n.kind === "container",
    );
    const nextChapterLabel = t("workspace.chapterNumber", { number: chapterSiblings.length + 1 });
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
      run: async () => { await handleRenameNode(currentTreeNode); },
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
      hint: t("workspace.command.restore"),
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
          if (path) showToast(t("workspace.toast.exportComplete"));
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
          if (path) showToast(t("workspace.toast.exportComplete"));
        } catch (e) {
          showToast(t("workspace.toast.exportFailed", { error: String(e) }));
        }
      },
    });
    cmds.push({
      id: "go-settings",
      section: sectionProject,
      label: t("workspace.command.openSettings"),
      run: () => navigate("/settings"),
    });
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
      id: "toggle-companion",
      section: "AI",
      label: companionOpen ? t("workspace.command.closeCompanion") : t("workspace.command.openWritingCompanion"),
      hint: "Cmd+J",
      run: () => setCompanionOpen((v) => !v),
    });
    cmds.push({
      id: "show-shortcuts",
      section: sectionHelp,
      label: t("workspace.command.shortcutHelp"),
      run: () => setShortcutsOpen(true),
    });
    return cmds;
  }, [load, navigateToNode, refreshTreeAndNavigateTo, refreshTreeKeepNode, navigate, promptDialog, enterZen, focus, companionOpen, railCollapsed, handleCreateSceneFromOutline, handleCreateChapterFromOutline, handleRenameNode, handleMoveSceneFromOutline, handleDeleteSceneFromOutline, showToast, language, t]);

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
  const aiContextSelection = aiOptions.context ?? DEFAULT_AI_CONTEXT_SELECTION;
  const aiContextPreview = contextPreview ?? FALLBACK_CONTEXT_PREVIEW;
  const aiContextItemCount = totalContextItems(aiContextPreview, aiContextSelection);

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
        <div className="ws-top-actions">
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
          <div className="ws-sep" />
          <button
            type="button"
            className={`ws-tool${aiModal ? " is-active" : ""}`}
            onClick={toggleAIModal}
          >
            <Sparkles size={15} /> AI <span className="kbd">⌘I</span>
          </button>
          <button
            type="button"
            className={`ws-tool${companionOpen ? " is-active" : ""}`}
            onClick={() => setCompanionOpen((v) => !v)}
          >
            <MessageCircle size={15} /> {t("workspace.companion")} <span className="kbd">⌘J</span>
          </button>
          <div className="ws-sep" />
          <button type="button" className="ws-tool" onClick={enterZen}>
            <Maximize2 size={15} /> ZEN
          </button>
        </div>
      </header>

      <div className={`ws-body${railCollapsed ? " rail-collapsed" : ""}${
        (aiModal || companionOpen) ? " right-wide" : ""
      }${companionOpen ? " right-xwide" : ""}`}>
        <OutlinePanel
          tree={load.tree}
          currentId={load.node.id}
          collapsed={railCollapsed}
          onToggleCollapse={() => setRailCollapsed((v) => !v)}
          onSelect={(n) => navigateToNode(n)}
          onRename={handleRenameNode}
          onCreateScene={handleCreateSceneFromOutline}
          onCreateChapter={handleCreateChapterFromOutline}
          onMoveSceneUp={(node) => handleMoveSceneFromOutline(node, "up")}
          onMoveSceneDown={(node) => handleMoveSceneFromOutline(node, "down")}
          onDeleteScene={handleDeleteSceneFromOutline}
        />
        <section className={`ws-editor${focus ? " focus-mode" : ""}`}>
          <div className="editor-col">
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
              key={load.node.id}
              ref={editorRef}
              initialDoc={load.initialDoc}
              onChange={(doc) => {
                debouncedSave(doc);
                throttledLastOpened();
              }}
              onCharCount={setCharCount}
              typewriter={typewriter}
              focus={focus}
              onManualSave={handleManualSave}
              extensions={[
                ...(mentionExtension ? [mentionExtension] : []),
                NoteMarkerExtension,
                AITargetExtension,
              ]}
              onMentionDoubleClick={(id) => setEntitySheetId(id)}
            />
          </div>
          <div className="editor-foot">
            <span>{currentSceneTitle}</span>
            <span>·</span>
            <span>{t("workspace.charCount", { count: charCount.toLocaleString(locale) })}</span>
          </div>
        </section>
        {aiModal && load ? (
          <AIPanel
            mode={aiModal.mode}
            canChooseMode={aiModal.canChooseMode}
            options={aiOptions}
            contextItemCount={aiContextItemCount}
            contextPreview={aiContextPreview}
            contextSelection={aiContextSelection}
            variations={gen.variations}
            currentIdx={gen.currentIdx}
            status={gen.status}
            onModeChange={(m) => {
              setAiModal((s) => (s ? { ...s, mode: m } : s));
              if (!tiptapEditor || !aiModal) return;
              if (m === "replaceAll") {
                tiptapEditor.commands.setAITarget("replaceAll", 1, tiptapEditor.state.doc.content.size);
              } else if (m === "insert") {
                tiptapEditor.commands.setAITarget("insert", aiModal.sel.from, aiModal.sel.from);
              } else {
                tiptapEditor.commands.setAITarget("replace", aiModal.sel.from, aiModal.sel.to);
              }
            }}
            onOptionsChange={setAiOptions}
            onContextSelectionChange={(context) => setAiOptions((opts) => ({ ...opts, context }))}
            onRun={(promptText, variationsOn) => {
              const selectionText =
                aiModal.mode === "replace"
                  ? tiptapEditor!.state.doc.textBetween(aiModal.sel.from, aiModal.sel.to, "\n")
                  : "";
              const args = {
                nodeId: load.node.id,
                prompt: promptText,
                options: aiOptions,
                selectionText,
              };
              if (variationsOn) gen.startVariations(args, 3);
              else gen.start(args);
            }}
            onSwitch={gen.switchVariation}
            onAccept={acceptAIModal}
            onCancel={closeAIModal}
            onContextClick={() => setAiCtxChecklistOpen((v) => !v)}
            showChecklist={aiCtxChecklistOpen}
          />
        ) : companionOpen && load ? (
          <CompanionPanel
            projectId={load.project.id}
            nodeIdRef={companionNodeRef}
            onClose={() => { setCompanionOpen(false); focusEditor(); }}
            onApplied={() => {
              if (!load) return;
              refreshTreeKeepNode(load.node.id);
              refreshMentioned(load.node.id);
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
        ) : (
          <ContextPanel
            project={load.project}
            node={load.node}
            charCount={charCount}
            typewriter={typewriter}
            onToggleTypewriter={() => setTypewriter((v) => !v)}
            saveStatus={saveStatus}
            mentionedEntities={mentioned}
            onMentionClick={(id) => setEntitySheetId(id)}
            onAutoMention={handleAutoMentionScene}
            autoMentionBusy={autoMentionBusy}
            onOpenThread={setThreadSheetId}
            onProjectTitleChange={handleProjectTitleCommit}
            onProjectChanged={(p) =>
              setLoad((prev) => (prev ? { ...prev, project: p } : prev))
            }
          />
        )}
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

      {versionSheetNodeId && (
        <VersionSheet
          nodeId={versionSheetNodeId}
          onClose={() => {
            setVersionSheetNodeId(null);
            focusEditor();
          }}
          onRestored={(updatedNode) => {
            const docStr = updatedNode.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`;
            setLoad((prev) => prev ? { ...prev, node: updatedNode, initialDoc: JSON.parse(docStr) } : prev);
            setCharCount(updatedNode.word_count);
            showToast(t("workspace.toast.versionRestored"));
          }}
        />
      )}

      {zenOpen && (
        <ZenMode
          initialDoc={load.initialDoc}
          initialSelection={savedSelectionRef.current}
          charCount={charCount}
          sceneLabel={currentNodeLabel}
          onChange={(doc) => { debouncedSave(doc); }}
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
