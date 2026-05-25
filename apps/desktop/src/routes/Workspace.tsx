import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { nodes, projects, snapshots } from "../lib/rpc";
import type { NodeRow, Project } from "../lib/types";
import { TiptapEditor } from "../components/editor/Tiptap";
import { ContextPanel, type SaveStatus } from "../components/ContextPanel";
import { OutlinePanel } from "../components/OutlinePanel";
import { CommandPalette, type Command } from "../components/CommandPalette";
import { useDebouncedCallback } from "../hooks/useDebouncedCallback";
import { useThrottledCallback } from "../hooks/useThrottledCallback";
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
  const [load, setLoad] = useState<LoadState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [charCount, setCharCount] = useState(0);
  const [typewriter, setTypewriter] = useState(false);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>({ kind: "idle" });
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [dialog, setDialog] = useState<DialogState | null>(null);

  const showToast = (msg: string) => {
    setToast(msg);
    window.setTimeout(() => setToast(null), 1800);
  };

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
        if (!p.last_opened_node_id) throw new Error("project has no opened node");
        const next = await fetchTree(projectId, p.last_opened_node_id);
        if (!cancelled) {
          setLoad(next);
          setCharCount(next.node.word_count);
        }
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => { cancelled = true; };
  }, [projectId, fetchTree]);

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
      } catch (e) {
        setSaveStatus({ kind: "error", message: String(e) });
        setError(String(e));
      }
    },
    [load],
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
      id: "view-character",
      section: "보기",
      label: "캐릭터 시트",
      hint: "(곧 추가됨 — Plan 4)",
      disabled: true,
      run: () => {},
    });
    cmds.push({
      id: "view-threads",
      section: "보기",
      label: "흐름(Thread)",
      hint: "(곧 추가됨 — post-MVP)",
      disabled: true,
      run: () => {},
    });
    return cmds;
  }, [load, navigateToNode, refreshTreeAndNavigateTo, refreshTreeKeepNode, navigate, promptDialog, confirmDialog]);

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
          <span className="mode-toggle on">편집</span>
          <span className="mode-toggle">AI</span>
        </span>
        <span className="ws-zen">ZEN</span>
      </header>

      <div className="ws-body">
        <div className="ws-editor">
          <TiptapEditor
            key={load.node.id}
            initialDoc={load.initialDoc}
            onChange={(doc) => {
              debouncedSave(doc);
              throttledLastOpened();
            }}
            onCharCount={setCharCount}
            typewriter={typewriter}
            onManualSave={handleManualSave}
          />
        </div>
        <ContextPanel
          project={load.project}
          node={load.node}
          charCount={charCount}
          typewriter={typewriter}
          onToggleTypewriter={() => setTypewriter((v) => !v)}
          saveStatus={saveStatus}
        />
      </div>

      <OutlinePanel
        tree={load.tree}
        currentId={load.node.id}
        onSelect={(n) => navigateToNode(n)}
      />

      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        commands={commands}
      />

      {dialog && <DialogModal dialog={dialog} onClose={() => setDialog(null)} />}

      {toast && <div className="ws-toast">{toast}</div>}
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
