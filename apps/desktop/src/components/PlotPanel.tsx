import { useCallback, useEffect, useRef, useState } from "react";
import type { Project, PlotSpine, PlotScene, Thread } from "../lib/types";
import { plot as plotApi, beats as beatsApi, threads as threadsApi, projects as projectsApi } from "../lib/rpc";
import { Plus, X, Pencil } from "../lib/icons";
import "./PlotPanel.css";

interface Props {
  project: Project;
  nodeId: string;
  onOpenThread: (threadId: string) => void;
  onProjectChanged?: (project: Project) => void;
}

function IntensityBars({ level }: { level: number }) {
  return (
    <span className="intensity">
      {[1, 2, 3].map((l) => (
        <i key={l} className={l <= level ? "on" : ""} />
      ))}
    </span>
  );
}

export function PlotPanel({ project, nodeId, onOpenThread, onProjectChanged }: Props) {
  const [spine, setSpine] = useState<PlotSpine | null>(null);
  const [openThreads, setOpenThreads] = useState<Thread[]>([]);
  const [outlineOpen, setOutlineOpen] = useState(true);
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
      <div className={"spine-scene" + (mode === "current" ? " current" : "")}>
        <div className="spine-label">
          {mode === "current"
            ? <span className="tag">현재 · {scene.label}</span>
            : <>{mode === "prev" ? "이전" : "다음"} · {scene.label}</>}
        </div>
        {scene.beats.length === 0 && !editable && <p className="sec-empty" style={{ margin: "0 0 4px" }}>비트 없음</p>}
        {scene.beats.map((bt) => (
          <div className={"beat" + (editable ? "" : " dim")} key={bt.id}>
            <button type="button" className="beat-head" onClick={() => onOpenThread(bt.thread_id)}>
              <span className="beat-dot" style={{ "--bc": bt.thread_color } as React.CSSProperties} aria-hidden />
              <span className="beat-thread">{bt.thread_name}</span>
            </button>
            {editingBeat === bt.id && editable ? (
              <div className="plot-beat-edit">
                <input className="attr-value" defaultValue={bt.label} placeholder="제목"
                  onBlur={(e) => { if (e.target.value !== bt.label) patchBeat(bt.id, { label: e.target.value }); }} />
                <textarea defaultValue={bt.description} placeholder="무슨 일이 일어나는지" rows={3}
                  onBlur={(e) => { if (e.target.value !== bt.description) patchBeat(bt.id, { description: e.target.value }); }} />
                <div className="beat-foot">
                  <span className="intensity-pick">
                    {[1, 2, 3].map((lvl) => (
                      <button key={lvl} type="button" className={bt.intensity === lvl ? "sel" : ""} onClick={() => patchBeat(bt.id, { intensity: lvl })}>{lvl}</button>
                    ))}
                  </span>
                  <button type="button" className="attr-del" aria-label="삭제" onClick={() => deleteBeat(bt.id)}><X size={13} /></button>
                </div>
              </div>
            ) : (
              <>
                <div className="beat-label" style={{ marginTop: 6 }}>{bt.label || "(제목 없음)"}</div>
                {bt.description && <p className="beat-desc">{bt.description}</p>}
                <div className="beat-foot">
                  <IntensityBars level={bt.intensity} />
                  {editable && (
                    <button type="button" className="attr-del" title="편집" aria-label="비트 편집" onClick={() => setEditingBeat(bt.id)}>
                      <Pencil size={13} />
                    </button>
                  )}
                </div>
              </>
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
            <button type="button" className="add-beat" disabled={openThreads.length === 0}
              onClick={() => { setAdding(mode); setDraftThread(openThreads[0]?.id ?? ""); setDraftLabel(""); }}>
              <Plus size={13} /> {mode === "current" ? "비트 추가" : "다음 씬에 비트"}
            </button>
          )
        )}
        {mode !== "next" && <div className="spine-connector" />}
      </div>
    );
  };

  return (
    <section className="sec">
      <h4>플롯</h4>
      <button type="button" className="plot-toggle" onClick={() => setOutlineOpen((v) => !v)}>
        {outlineOpen ? "▾" : "▸"} 개요
      </button>
      {outlineOpen && (
        <textarea className="plot-outline-edit" value={outline} rows={5} placeholder="작품 전체 개요 (로그라인 + 줄거리)"
          onChange={(e) => saveOutline(e.target.value)} />
      )}
      {openThreads.length === 0 && (
        <p className="sec-empty" style={{ marginTop: 12 }}>스토리라인이 없어요. 명령 팔레트에서 "이 씬을 새 Thread로 표시"로 시작하세요.</p>
      )}
      <div className="spine" style={{ marginTop: 12 }}>
        {renderScene(spine?.prev, "prev")}
        {renderScene(spine?.current, "current")}
        {renderScene(spine?.next, "next")}
      </div>
    </section>
  );
}
