import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { nodes, projects, snapshots, entities as entitiesApi, mentions as mentionsApi, threads as threadsApi, beats as beatsApi, ai as aiApi, settings as settingsApi, exportApi, notes as notesApi, gitSync } from "../lib/rpc";
import { NoteMarkerExtension } from "../components/editor/NoteMarkerExtension";
import { NotePopover } from "../components/NotePopover";
import type { NodeRow, Project, Entity, AIOptions, AIDelta, AIReset, AIDone, AIError, AICancelled } from "../lib/types";
import { buildMentionExtension, type MentionPickerState } from "../components/editor/MentionExtension";
import { MentionPicker } from "../components/editor/MentionPicker";
import { EntitySheet } from "../components/EntitySheet";
import { ThreadSheet } from "../components/ThreadSheet";
import { VersionSheet } from "../components/VersionSheet";
import { saveExportedMarkdown } from "../lib/exportSave";
import { TiptapEditor, type TiptapHandle } from "../components/editor/Tiptap";
import { ZenMode } from "../components/ZenMode";
import { ContextPanel, type SaveStatus } from "../components/ContextPanel";
import { OutlinePanel } from "../components/OutlinePanel";
import { CommandPalette, type Command } from "../components/CommandPalette";
import { useDebouncedCallback } from "../hooks/useDebouncedCallback";
import { useThrottledCallback } from "../hooks/useThrottledCallback";
import { useEngineEvent } from "../hooks/useEngineEvent";
import { AIMode, type AIRunStatus } from "../components/ai/AIMode";
import { AIContextPanel } from "../components/ai/AIContextPanel";
import { useToast } from "../components/ToastProvider";
import {
  buildTree,
  findFirstLeaf,
  flatten,
  leafNeighbors,
  type TreeNode,
} from "../hooks/useFirstLeaf";

const SAVE_DEBOUNCE_MS = 800;
const LAST_OPENED_THROTTLE_MS = 5000;

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
  const [charCount, setCharCount] = useState(0);
  const [typewriter, setTypewriter] = useState(false);
  const [focus, setFocus] = useState(false);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>({ kind: "idle" });
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [versionSheetNodeId, setVersionSheetNodeId] = useState<string | null>(null);
  const [zenOpen, setZenOpen] = useState(false);
  const [dialog, setDialog] = useState<DialogState | null>(null);
  const [mentionState, setMentionState] = useState<MentionPickerState | null>(null);
  const [entitySheetId, setEntitySheetId] = useState<string | null>(null);
  const [threadSheetId, setThreadSheetId] = useState<string | null>(null);
  const [mentioned, setMentioned] = useState<Entity[]>([]);
  const [mode, setMode] = useState<"edit" | "ai">("edit");
  const [aiPrompt, setAiPrompt] = useState("");
  const [aiOptions, setAiOptions] = useState<AIOptions>({ tone: "my", short_form: false });
  const [aiStatus, setAiStatus] = useState<AIRunStatus>({ kind: "idle" });
  const aiRunIdRef = useRef<string | null>(null);
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

  useEngineEvent<AIDelta>("ai-delta", (p) => {
    if (p.run_id !== aiRunIdRef.current) return;
    setAiStatus((s) => {
      const prev = s.kind === "running" ? s.text : "";
      return { kind: "running", runId: p.run_id, text: prev + p.text };
    });
  });
  useEngineEvent<AIReset>("ai-reset", (p) => {
    // Engine's dedup observed a divergent retry — REPLACE the running text
    // with the authoritative reconciled buffer.
    if (p.run_id !== aiRunIdRef.current) return;
    setAiStatus({ kind: "running", runId: p.run_id, text: p.text });
  });
  useEngineEvent<AIDone>("ai-done", (p) => {
    if (p.run_id !== aiRunIdRef.current) return;
    setAiStatus({ kind: "done", text: p.full_text });
    aiRunIdRef.current = null;
  });
  useEngineEvent<AIError>("ai-error", (p) => {
    if (p.run_id !== aiRunIdRef.current) return;
    setAiStatus((s) => ({ kind: "error", message: p.message, text: s.kind === "running" ? s.text : "" }));
    aiRunIdRef.current = null;
  });
  useEngineEvent<AICancelled>("ai-cancelled", (p) => {
    if (p.run_id !== aiRunIdRef.current) return;
    setAiStatus({ kind: "idle" });
    aiRunIdRef.current = null;
  });

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

  // Navigate without re-fetching the tree (used by outline click + leaf neighbor cmds).
  const navigateToNode = useCallback(
    async (target: TreeNode | NodeRow) => {
      if (!projectId) return;
      const leaf = "children" in target ? findFirstLeaf(target as TreeNode) : (target as NodeRow);
      if (!leaf) {
        showToast("이동할 씬이 없습니다");
        return;
      }
      const n = await nodes.get(leaf.id);
      const initialDoc = JSON.parse(n.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`);
      // Functional setLoad so we don't clobber the latest tree.
      setLoad((prev) => (prev ? { ...prev, node: n, initialDoc } : prev));
      setCharCount(n.word_count);
      nodes.setLastOpened(projectId, n.id).catch(() => { /* benign */ });
    },
    [projectId],
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
        showToast("엔티티 생성 실패: " + String(err));
      }
    };
    window.addEventListener("linetta:mention-pick-new", handler);
    return () => window.removeEventListener("linetta:mention-pick-new", handler);
  }, [projectId]);

  // Global Cmd+R reload + Cmd+K palette toggle.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const isMac = navigator.platform.toLowerCase().includes("mac");
      const mod = isMac ? e.metaKey : e.ctrlKey;
      if (!mod) return;
      if (e.key.toLowerCase() === "r") {
        e.preventDefault();
        window.location.reload();
      } else if (e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
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

  const handleManualSave = useCallback(
    async (doc: object) => {
      if (!load) return;
      setSaveStatus({ kind: "saving" });
      try {
        await nodes.updateContent(load.node.id, JSON.stringify(doc));
        await snapshots.createManual(load.node.id, JSON.stringify(doc));
        setSaveStatus({ kind: "saved", at: Date.now() });
        showToast("스냅샷 저장됨");
      } catch (e) {
        setSaveStatus({ kind: "error", message: String(e) });
        setError(String(e));
      }
    },
    [load],
  );

  // --- AI handlers ---

  const handlePreset = (preset: "rewrite" | "expand" | "compact") => {
    const seeds: Record<typeof preset, string> = {
      rewrite: "이 문단을 다른 톤으로 다시 써줘.",
      expand: "이 장면을 더 감각적으로 확장해줘.",
      compact: "이 씬을 한 문단으로 요약해줘.",
    };
    setAiPrompt(seeds[preset]);
  };

  const startAIRun = async () => {
    if (!load || !aiPrompt.trim()) return;
    setAiStatus({ kind: "running", runId: "pending", text: "" });
    try {
      const { run_id } = await aiApi.run(load.node.id, aiPrompt, aiOptions);
      aiRunIdRef.current = run_id;
      setAiStatus({ kind: "running", runId: run_id, text: "" });
    } catch (e) {
      setAiStatus({ kind: "error", message: String(e), text: "" });
    }
  };

  const cancelAIRun = async () => {
    if (!aiRunIdRef.current) return;
    try { await aiApi.cancel(aiRunIdRef.current); } catch { /* benign */ }
  };

  const insertResult = (text: string) => {
    if (!load || !text) return;
    const docStr = JSON.stringify({
      type: "doc",
      content: [
        ...((load.initialDoc as any).content ?? []),
        { type: "paragraph", content: [{ type: "text", text }] },
      ],
    });
    nodes.updateContent(load.node.id, docStr).then(() => {
      setMode("edit");
      setAiStatus({ kind: "idle" });
      refreshTreeKeepNode(load.node.id);
      setTimeout(() => focusEditor(), 0);
    });
  };

  const replaceWithResult = (text: string) => {
    if (!load || !text) return;
    const docStr = JSON.stringify({
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text }] }],
    });
    // Snapshot the old body first.
    snapshots.createManual(load.node.id, JSON.stringify(load.initialDoc)).catch(() => {});
    nodes.updateContent(load.node.id, docStr).then(() => {
      setMode("edit");
      setAiStatus({ kind: "idle" });
      refreshTreeKeepNode(load.node.id);
      setTimeout(() => focusEditor(), 0);
    });
  };

  // --- Commands ---

  const commands: Command[] = useMemo(() => {
    if (!load) return [];
    const { prev, next } = leafNeighbors(load.tree, load.node.id);
    const allNodes = flatten(load.tree);
    const siblingsOfCurrent = allNodes.filter(
      (n) => (n.parent_id ?? null) === (load.node.parent_id ?? null),
    );
    const leafSiblings = siblingsOfCurrent.filter((n) => n.kind === "leaf");
    const containerSiblings = siblingsOfCurrent.filter((n) => n.kind === "container");
    const nextSceneLabel = `씬 ${leafSiblings.length + 1}`;
    const nextChapterLabel = `${containerSiblings.length + 1}장`;

    const cmds: Command[] = [];
    cmds.push({
      id: "go-prev",
      section: "이동",
      label: "이전 씬",
      hint: prev ? prev.label : "(없음)",
      disabled: !prev,
      run: async () => { if (prev) await navigateToNode(prev); },
    });
    cmds.push({
      id: "go-next",
      section: "이동",
      label: "다음 씬",
      hint: next ? next.label : "(없음)",
      disabled: !next,
      run: async () => { if (next) await navigateToNode(next); },
    });
    for (const leaf of allNodes.filter((n) => n.kind === "leaf").slice(0, 20)) {
      cmds.push({
        id: `go-${leaf.id}`,
        section: "이동",
        label: `씬으로 이동: ${leaf.label}`,
        hint: leaf.title || undefined,
        disabled: leaf.id === load.node.id,
        run: async () => navigateToNode(leaf),
      });
    }

    cmds.push({
      id: "new-scene",
      section: "노드",
      label: `여기 옆에 새 씬 (${nextSceneLabel})`,
      run: async () => {
        const created = await nodes.createSibling(load.node.id, "leaf", nextSceneLabel, "");
        await refreshTreeAndNavigateTo(created.id);
      },
    });
    cmds.push({
      id: "new-chapter",
      section: "노드",
      label: `여기 옆에 새 장 (${nextChapterLabel})`,
      run: async () => {
        const chapter = await nodes.createSibling(load.node.id, "container", nextChapterLabel, "");
        const seeded = await nodes.createChild(chapter.id, "leaf", "씬 1", "");
        await refreshTreeAndNavigateTo(seeded.id);
      },
    });
    cmds.push({
      id: "mark-thread",
      section: "노드",
      label: "이 씬을 새 Thread로 표시",
      run: async () => {
        const name = await promptDialog("새 스토리라인 이름", load.node.title || load.node.label);
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
          showToast("스토리라인 생성 실패: " + String(e));
        }
      },
    });
    cmds.push({
      id: "add-note",
      section: "노드",
      label: "여백 주석 추가",
      run: async () => {
        const body = await promptDialog("여백 주석 본문", "");
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
          showToast("주석 추가 실패: " + String(e));
        }
      },
    });
    cmds.push({
      id: "rename",
      section: "노드",
      label: "이름 바꾸기",
      run: async () => {
        const nextLabel = await promptDialog("새 이름 (label)", load.node.label);
        if (nextLabel === null) return;
        const trimmed = nextLabel.trim();
        if (!trimmed) return;
        const nextTitle = await promptDialog("부제 (title, 비울 수 있음)", load.node.title);
        await nodes.rename(load.node.id, trimmed, nextTitle ?? "");
        await refreshTreeKeepNode(load.node.id);
        showToast("이름이 변경되었습니다");
      },
    });
    cmds.push({
      id: "delete",
      section: "노드",
      label: "삭제",
      hint: load.node.label,
      run: async () => {
        const ok = await confirmDialog(`"${load.node.label}"을(를) 삭제하시겠습니까?`);
        if (!ok) return;
        const fallback = prev ?? next ?? null;
        await nodes.delete(load.node.id);
        if (fallback) {
          await refreshTreeAndNavigateTo(fallback.id);
        } else {
          navigate("/");
        }
      },
    });
    cmds.push({
      id: "move-up",
      section: "노드",
      label: "이 씬 위로",
      run: async () => {
        await nodes.moveUp(load.node.id);
        await refreshTreeKeepNode(load.node.id);
      },
    });
    cmds.push({
      id: "move-down",
      section: "노드",
      label: "이 씬 아래로",
      run: async () => {
        await nodes.moveDown(load.node.id);
        await refreshTreeKeepNode(load.node.id);
      },
    });
    cmds.push({
      id: "view-outline",
      section: "보기",
      label: "아웃라인 (왼쪽 가장자리 호버)",
      disabled: true,
      hint: "↤",
      run: () => {},
    });
    cmds.push({
      id: "view-threads",
      section: "보기",
      label: "흐름 (Thread View)",
      run: () => navigate(`/workspace/${load.project.id}/threads`),
    });
    cmds.push({
      id: "version-restore",
      section: "프로젝트",
      label: "이 씬의 이전 버전",
      hint: "복원",
      run: () => setVersionSheetNodeId(load.node.id),
    });
    cmds.push({
      id: "export-project",
      section: "내보내기",
      label: "프로젝트 (.md)",
      run: async () => {
        try {
          const payload = await exportApi.project(load.project.id);
          const path = await saveExportedMarkdown(payload);
          if (path) showToast("내보내기 완료");
        } catch (e) {
          showToast("내보내기 실패: " + String(e));
        }
      },
    });
    cmds.push({
      id: "export-node",
      section: "내보내기",
      label: "이 씬 (.md)",
      run: async () => {
        try {
          const payload = await exportApi.node(load.node.id);
          const path = await saveExportedMarkdown(payload);
          if (path) showToast("내보내기 완료");
        } catch (e) {
          showToast("내보내기 실패: " + String(e));
        }
      },
    });
    cmds.push({
      id: "go-settings",
      section: "프로젝트",
      label: "설정 열기",
      run: () => navigate("/settings"),
    });
    cmds.push({
      id: "git-sync-now",
      section: "프로젝트",
      label: "지금 GitHub로 동기화",
      run: async () => {
        try {
          const res = await gitSync.run();
          if (res.skipped) {
            showToast("동기화 비활성화 — 설정에서 git 폴더를 지정하세요");
            return;
          }
          if (res.error) {
            showToast(`동기화 오류: ${res.error}`);
            return;
          }
          if (!res.committed) {
            showToast(`변경 없음 (${res.files_written}개 파일 점검 완료)`);
            return;
          }
          if (res.pushed) {
            showToast(`${res.files_written}개 파일 동기화 완료 (push OK)`);
          } else {
            showToast(`${res.files_written}개 파일 커밋됨 — push 실패`);
          }
        } catch (e) {
          showToast("동기화 실패: " + String(e));
        }
      },
    });
    cmds.push({
      id: "enter-zen",
      section: "보기",
      label: "ZEN 모드 열기",
      hint: "ESC로 종료",
      run: enterZen,
    });
    cmds.push({
      id: "toggle-focus",
      section: "보기",
      label: focus ? "Focus 모드 끄기" : "Focus 모드 켜기",
      run: () => setFocus((v) => !v),
    });
    return cmds;
  }, [load, navigateToNode, refreshTreeAndNavigateTo, refreshTreeKeepNode, navigate, promptDialog, confirmDialog, enterZen, focus]);

  const breadcrumb = useMemo(() => {
    if (!load) return "";
    const byId = new Map(flatten(load.tree).map((n) => [n.id, n] as const));
    const chain: string[] = [];
    let cur: TreeNode | undefined = byId.get(load.node.id);
    while (cur) {
      chain.unshift(cur.label);
      cur = cur.parent_id ? byId.get(cur.parent_id) : undefined;
    }
    return `← 작품 · ${chain.join(" › ")}${load.node.title ? ` — ${load.node.title}` : ""}`;
  }, [load]);

  const aiContextSummary = useMemo(() => {
    const parts: string[] = ["현재 씬 + 직전 씬"];
    if (mentioned.length > 0) parts.push("@" + mentioned.map((e) => e.name).join(", @"));
    if (load?.project.style_notes) parts.push("style notes");
    return parts.join(" · ");
  }, [mentioned, load]);

  if (error) {
    return (
      <main className="shell">
        <p><Link to="/">← Library</Link></p>
        <p className="error">{error}</p>
      </main>
    );
  }
  if (!load) {
    return (
      <main className="shell">
        <p className="hint">불러오는 중…</p>
      </main>
    );
  }

  return (
    <main className="workspace">
      <header className="ws-top">
        <Link to="/" className="ws-breadcrumb">{breadcrumb}</Link>
        <span className="ws-modes">
          <button
            type="button"
            className={`mode-toggle${mode === "edit" ? " on" : ""}`}
            onClick={() => setMode("edit")}
          >
            편집
          </button>
          <button
            type="button"
            className={`mode-toggle${mode === "ai" ? " on" : ""}`}
            onClick={() => setMode("ai")}
          >
            AI
          </button>
        </span>
        <button type="button" className="mode-toggle ws-zen-btn" onClick={enterZen}>
          ZEN
        </button>
      </header>

      <div className={`ws-body${(entitySheetId || threadSheetId) ? " with-sheet" : ""}`}>
        {mode === "edit" ? (
          <div className="ws-editor">
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
              ]}
              onMentionDoubleClick={(id) => setEntitySheetId(id)}
            />
          </div>
        ) : (
          <AIMode
            status={aiStatus}
            prompt={aiPrompt}
            onPromptChange={setAiPrompt}
            options={aiOptions}
            onOptionsChange={setAiOptions}
            onPresetClick={handlePreset}
            onRun={startAIRun}
            onCancel={cancelAIRun}
            onInsert={insertResult}
            onReplace={replaceWithResult}
            onRegenerate={startAIRun}
            onDiscard={() => setAiStatus({ kind: "idle" })}
            contextSummary={aiContextSummary}
          />
        )}
        {entitySheetId ? (
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
          />
        ) : threadSheetId ? (
          <ThreadSheet
            threadId={threadSheetId}
            onClose={() => {
              setThreadSheetId(null);
              focusEditor();
            }}
            onSaved={() => {
              /* ActiveThreadsPanel self-reloads */
            }}
          />
        ) : mode === "ai" ? (
          <AIContextPanel
            project={load.project}
            mentioned={mentioned}
            options={aiOptions}
            onOptionsChange={setAiOptions}
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
            onOpenThread={setThreadSheetId}
            onThreadDataChanged={() => {
              /* no-op; panel auto-reloads */
            }}
          />
        )}
      </div>

      <OutlinePanel
        tree={load.tree}
        currentId={load.node.id}
        onSelect={(n) => navigateToNode(n)}
        onClose={focusEditor}
      />

      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        commands={commands}
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
            showToast("이전 버전으로 복원되었습니다");
          }}
        />
      )}

      {zenOpen && (
        <ZenMode
          initialDoc={load.initialDoc}
          initialSelection={savedSelectionRef.current}
          charCount={charCount}
          sceneLabel={load.node.label}
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
    </main>
  );
}

function DialogModal({ dialog, onClose }: { dialog: DialogState; onClose: () => void }) {
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
          <button type="button" onClick={handleCancel}>취소</button>
          <button type="button" className="primary" onClick={handleConfirm}>확인</button>
        </div>
      </div>
    </div>
  );
}
