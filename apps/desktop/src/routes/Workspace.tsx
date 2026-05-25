import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { nodes, projects, snapshots } from "../lib/rpc";
import type { NodeRow, Project } from "../lib/types";
import { TiptapEditor } from "../components/editor/Tiptap";
import { ContextPanel } from "../components/ContextPanel";
import { useDebouncedCallback } from "../hooks/useDebouncedCallback";
import { useThrottledCallback } from "../hooks/useThrottledCallback";

const SAVE_DEBOUNCE_MS = 800;
const LAST_OPENED_THROTTLE_MS = 5000;

interface LoadState {
  project: Project;
  node: NodeRow;
  initialDoc: object;
}

export function Workspace() {
  const { projectId } = useParams();
  const [load, setLoad] = useState<LoadState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [charCount, setCharCount] = useState(0);
  const [typewriter, setTypewriter] = useState(false);

  // Initial load: project → first leaf node → parse content_doc.
  useEffect(() => {
    if (!projectId) return;
    let cancelled = false;
    (async () => {
      try {
        const p = await projects.get(projectId);
        if (!p.last_opened_node_id) {
          throw new Error("project has no opened node");
        }
        const n = await nodes.get(p.last_opened_node_id);
        const docStr = n.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`;
        const initialDoc = JSON.parse(docStr);
        if (!cancelled) {
          setLoad({ project: p, node: n, initialDoc });
          setCharCount(n.word_count);
        }
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => { cancelled = true; };
  }, [projectId]);

  const saveNow = useCallback(
    async (doc: object) => {
      if (!load) return;
      try {
        await nodes.updateContent(load.node.id, JSON.stringify(doc));
      } catch (e) {
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

  // Touch last_opened periodically while the editor is active.
  useEffect(() => {
    if (!load) return;
    throttledLastOpened();
  }, [load, throttledLastOpened]);

  const handleManualSave = useCallback(
    async (doc: object) => {
      if (!load) return;
      try {
        // Flush latest content first so the snapshot is in sync.
        await nodes.updateContent(load.node.id, JSON.stringify(doc));
        await snapshots.createManual(load.node.id, JSON.stringify(doc));
        showToast("스냅샷 저장됨");
      } catch (e) {
        setError(String(e));
      }
    },
    [load],
  );

  const showToast = (msg: string) => {
    setToast(msg);
    window.setTimeout(() => setToast(null), 1800);
  };

  const breadcrumb = useMemo(() => {
    if (!load) return "";
    return `← 작품 · ${load.node.label}${load.node.title ? ` — ${load.node.title}` : ""}`;
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
        />
      </div>

      {toast && <div className="ws-toast">{toast}</div>}
    </main>
  );
}
