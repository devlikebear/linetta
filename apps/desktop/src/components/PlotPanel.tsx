import { useCallback, useEffect, useRef, useState } from "react";
import type { Project, PlotSpine, PlotScene, Thread } from "../lib/types";
import { plot as plotApi, beats as beatsApi, threads as threadsApi, projects as projectsApi } from "../lib/rpc";
import { Plus, X } from "../lib/icons";
import "./PlotPanel.css";

interface Props {
  project: Project;
  nodeId: string;
  onOpenThread: (threadId: string) => void;
  onProjectChanged?: (project: Project) => void;
}

export function PlotPanel({ project, nodeId, onOpenThread, onProjectChanged }: Props) {
  const [spine, setSpine] = useState<PlotSpine | null>(null);
  const [openThreads, setOpenThreads] = useState<Thread[]>([]);
  const [outlineOpen, setOutlineOpen] = useState(false);
  const [outline, setOutline] = useState(project.outline ?? "");
  const [editingBeat, setEditingBeat] = useState<string | null>(null);
  const [adding, setAdding] = useState<"current" | "next" | null>(null);
  const [draftThread, setDraftThread] = useState("");
  const [draftLabel, setDraftLabel] = useState("");
  const saveTimer = useRef<number | null>(null);

  const reload = useCallback(async () => {
    try {
      const [sp, ths] = await Promise.all([
        plotApi.spinePanel(nodeId),
        threadsApi.list(project.id, false),
      ]);
      setSpine(sp);
      setOpenThreads(ths);
    } catch {
      setSpine(null);
    }
  }, [nodeId, project.id]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [sp, ths] = await Promise.all([
          plotApi.spinePanel(nodeId),
          threadsApi.list(project.id, false),
        ]);
        if (!cancelled) { setSpine(sp); setOpenThreads(ths); }
      } catch {
        if (!cancelled) setSpine(null);
      }
    })();
    return () => { cancelled = true; };
  }, [nodeId, project.id]);

  useEffect(() => () => {
    if (saveTimer.current) window.clearTimeout(saveTimer.current);
  }, []);

  useEffect(() => { setOutline(project.outline ?? ""); }, [project.id, project.outline]);

  const saveOutline = (next: string) => {
    setOutline(next);
    if (saveTimer.current) window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(async () => {
      try {
        const updated = await projectsApi.update({ id: project.id, outline: next });
        onProjectChanged?.(updated);
      } catch { /* benign; keep local draft */ }
    }, 600);
  };

  const addBeat = async (target: "current" | "next") => {
    const sceneId = target === "current" ? spine?.current.node_id : spine?.next?.node_id;
    const threadId = draftThread || openThreads[0]?.id;
    if (!sceneId || !threadId || !draftLabel.trim()) { setAdding(null); return; }
    try {
      await beatsApi.create({ thread_id: threadId, node_id: sceneId, label: draftLabel.trim() });
      setAdding(null); setDraftLabel(""); setDraftThread("");
      reload();
    } catch { /* benign */ }
  };

  const patchBeat = async (id: string, patch: { label?: string; description?: string; intensity?: number }) => {
    try {
      await beatsApi.update({ id, ...patch });
      reload();
    } catch { /* benign */ }
  };

  const deleteBeat = async (id: string) => {
    try { await beatsApi.delete(id); reload(); } catch { /* benign */ }
  };

  const renderScene = (scene: PlotScene | null | undefined, mode: "prev" | "current" | "next") => {
    if (!scene) return null;
    const editable = mode === "current";
    return (
      <div className={`plot-scene plot-scene-${mode}`}>
        <div className="plot-scene-label">{mode === "current" ? "현재 씬" : `${mode === "prev" ? "이전" : "다음"} 씬 · ${scene.label}`}</div>
        {scene.beats.length === 0 && !editable && <p className="plot-empty">비트 없음</p>}
        {scene.beats.map((bt) => (
          <div className="plot-beat" key={bt.id}>
            <button type="button" className="plot-beat-head" onClick={() => onOpenThread(bt.thread_id)}>
              <span className="plot-dot" style={{ backgroundColor: bt.thread_color }} aria-hidden />
              <span className="plot-thread">{bt.thread_name}</span>
              <span className="plot-label">{bt.label || "(제목 없음)"}</span>
            </button>
            {editable && (
              <button type="button" className="plot-edit" aria-label="비트 편집" onClick={() => setEditingBeat(editingBeat === bt.id ? null : bt.id)}>✎</button>
            )}
            {bt.description && editingBeat !== bt.id && <p className="plot-desc">{bt.description}</p>}
            {editable && editingBeat === bt.id && (
              <div className="plot-beat-edit">
                <input className="attr-value" defaultValue={bt.label} placeholder="제목"
                  onBlur={(e) => { if (e.target.value !== bt.label) patchBeat(bt.id, { label: e.target.value }); }} />
                <textarea defaultValue={bt.description} placeholder="무슨 일이 일어나는지" rows={3}
                  onBlur={(e) => { if (e.target.value !== bt.description) patchBeat(bt.id, { description: e.target.value }); }} />
                <div className="plot-beat-edit-actions">
                  <div className="plot-intensity">
                    {[1, 2, 3].map((lvl) => (
                      <button key={lvl} type="button" className={bt.intensity === lvl ? "sel" : ""} onClick={() => patchBeat(bt.id, { intensity: lvl })}>{lvl}</button>
                    ))}
                  </div>
                  <button type="button" className="attr-del" aria-label="삭제" onClick={() => deleteBeat(bt.id)}><X size={14} /></button>
                </div>
              </div>
            )}
          </div>
        ))}
        {(mode === "current" || mode === "next") && (
          adding === mode ? (
            <div className="plot-add">
              <select className="attr-value" value={draftThread} onChange={(e) => setDraftThread(e.target.value)}>
                {openThreads.length === 0 && <option value="">스토리라인 없음</option>}
                {openThreads.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
              </select>
              <input autoFocus className="attr-value" value={draftLabel} placeholder="비트 제목 (Enter)"
                onChange={(e) => setDraftLabel(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") { e.preventDefault(); addBeat(mode); }
                  else if (e.key === "Escape") { e.preventDefault(); setAdding(null); }
                }} />
            </div>
          ) : (
            <button type="button" className="plot-add-btn" disabled={openThreads.length === 0}
              onClick={() => { setAdding(mode); setDraftThread(openThreads[0]?.id ?? ""); setDraftLabel(""); }}>
              <Plus size={12} /> {mode === "current" ? "비트 추가" : "다음 씬에 비트 추가"}
            </button>
          )
        )}
      </div>
    );
  };

  return (
    <section className="ctx-section plot-panel">
      <h4>플롯</h4>
      <div className="plot-outline">
        <button type="button" className="plot-outline-toggle" onClick={() => setOutlineOpen((v) => !v)}>
          {outlineOpen ? "▾" : "▸"} 개요
        </button>
        {outlineOpen && (
          <textarea className="plot-outline-text" value={outline} rows={5} placeholder="작품 전체 개요 (로그라인 + 줄거리)"
            onChange={(e) => saveOutline(e.target.value)} />
        )}
      </div>
      {openThreads.length === 0 && (
        <p className="ctx-empty">스토리라인이 없어요. 명령 팔레트에서 "이 씬을 새 Thread로 표시"로 시작하세요.</p>
      )}
      {renderScene(spine?.prev, "prev")}
      {renderScene(spine?.current, "current")}
      {renderScene(spine?.next, "next")}
    </section>
  );
}
