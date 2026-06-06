import { invoke } from "@tauri-apps/api/core";
import type {
  AIOptions,
  AIContextPreview,
  CompanionApplyOpsResult,
  Beat,
  CompanionMessage,
  ContextCounts,
  ContextPreviewResponse,
  DiagnosticsSnapshot,
  EngineStatus,
  Entity,
  ExportPayload,
  FactCard,
  GitSyncInitResult,
  GitSyncResult,
  ImportMarkdownResult,
  ImportPreviewResult,
  ListProjectsParams,
  NewBeatInput,
  NewEntityInput,
  NewFactFromUrlInput,
  NewFactInput,
  NewNoteInput,
  NewProjectInput,
  NewRelationshipInput,
  NewRelationshipPairInput,
  NewThreadInput,
  NodeRow,
  Note,
  OpsStatus,
  PlotSpine,
  Project,
  ProposalOp,
  Relationship,
  SceneMention,
  SearchResult,
  ProviderID,
  ProviderTestResult,
  Settings,
  SettingsPatch,
  Snapshot,
  SnapshotEntry,
  Thread,
  UpdateBeatInput,
  UpdateEntityInput,
  UpdateFactInput,
  UpdateNoteInput,
  UpdateProjectInput,
  UpdateRelationshipInput,
  UpdateThreadInput,
  WebSearchTestResult,
} from "./types";

// Tauri commands defined in src-tauri.

export async function enginePing(): Promise<string> {
  return invoke<string>("engine_ping");
}

export async function engineStatus(): Promise<EngineStatus> {
  return invoke<EngineStatus>("engine_status");
}

export async function openPath(path: string): Promise<void> {
  return invoke<void>("open_path", { path });
}

export async function rpcCall<T>(method: string, params?: unknown): Promise<T> {
  return invoke<T>("engine_call", { method, params: params ?? null });
}

export const projects = {
  create: (input: NewProjectInput) => rpcCall<Project>("projects.create", input),
  list: (params: ListProjectsParams = {}) => rpcCall<Project[]>("projects.list", params),
  get: (id: string) => rpcCall<Project>("projects.get", { id }),
  update: (input: UpdateProjectInput) => rpcCall<Project>("projects.update", input),
  rewriteSynopsis: (id: string) => rpcCall<Project>("projects.rewrite_synopsis", { id }),
  clearSynopsis: (id: string) => rpcCall<Project>("projects.clear_synopsis", { id }),
  archive: (id: string) => rpcCall<{ ok: true }>("projects.archive", { id }),
  restore: (id: string) => rpcCall<{ ok: true }>("projects.restore", { id }),
  delete: (id: string) => rpcCall<{ ok: true }>("projects.delete", { id }),
};

export const nodes = {
  get: (id: string) => rpcCall<NodeRow>("nodes.get", { id }),
  updateContent: (id: string, doc: string) =>
    rpcCall<NodeRow>("nodes.update_content", { id, doc }),
  setLastOpened: (projectId: string, nodeId: string) =>
    rpcCall<{ ok: true }>("nodes.set_last_opened", { project_id: projectId, node_id: nodeId }),
  listTree: (projectId: string) =>
    rpcCall<NodeRow[]>("nodes.list_tree", { project_id: projectId }),
  createSibling: (referenceId: string, kind: "leaf" | "container", label: string, title: string) =>
    rpcCall<NodeRow>("nodes.create_sibling", { reference_id: referenceId, kind, label, title }),
  createChild: (parentId: string, kind: "leaf" | "container", label: string, title: string) =>
    rpcCall<NodeRow>("nodes.create_child", { parent_id: parentId, kind, label, title }),
  rename: (id: string, label: string, title: string) =>
    rpcCall<{ ok: true }>("nodes.rename", { id, label, title }),
  delete: (id: string) => rpcCall<{ ok: true }>("nodes.delete", { id }),
  moveUp: (id: string) => rpcCall<{ ok: true }>("nodes.move_up", { id }),
  moveDown: (id: string) => rpcCall<{ ok: true }>("nodes.move_down", { id }),
};

export const snapshots = {
  createManual: (nodeId: string, doc: string) =>
    rpcCall<Snapshot>("snapshots.create_manual", { node_id: nodeId, doc }),
  listForNode: (nodeId: string) =>
    rpcCall<SnapshotEntry[]>("snapshots.list_for_node", { node_id: nodeId }),
  restore: (snapshotId: string) =>
    rpcCall<NodeRow>("snapshots.restore", { snapshot_id: snapshotId }),
};

export const settings = {
  get: () => rpcCall<Settings>("settings.get"),
  set: (patch: SettingsPatch) => rpcCall<Settings>("settings.set", patch),
};

export const providers = {
  listModels: (provider: ProviderID) =>
    rpcCall<{ models: string[] }>("providers.list_models", { provider }),
  detectCli: () => rpcCall<{ path: string }>("providers.detect_cli"),
  test: (provider: ProviderID) =>
    rpcCall<ProviderTestResult>("providers.test", { provider }),
};

export const webSearch = {
  test: () => rpcCall<WebSearchTestResult>("web_search.test"),
};

export const exportApi = {
  project: (projectId: string) =>
    rpcCall<ExportPayload>("export.project", { project_id: projectId }),
  node: (nodeId: string) =>
    rpcCall<ExportPayload>("export.node", { node_id: nodeId }),
};

export const imports = {
  markdown: (fileName: string, content: string) =>
    rpcCall<ImportMarkdownResult>("imports.markdown", {
      file_name: fileName,
      content,
    }),
  preview: (fileName: string, content: string) =>
    rpcCall<ImportPreviewResult>("imports.preview", {
      file_name: fileName,
      content,
    }),
};

export const gitSync = {
  run: () => rpcCall<GitSyncResult>("git_sync.run"),
  init: () => rpcCall<GitSyncInitResult>("git_sync.init"),
};

export const opsStatus = {
  get: () => rpcCall<OpsStatus[]>("ops_status.get"),
  clearError: (jobName: string) =>
    rpcCall<{ ok: true }>("ops_status.clear_error", { job_name: jobName }),
};

export const diagnostics = {
  get: () => rpcCall<DiagnosticsSnapshot>("diagnostics.get"),
};

export const search = {
  query: (query: string, limit = 20) =>
    rpcCall<SearchResult[]>("search.query", { query, limit }),
};

export const entities = {
  list: (projectId: string) => rpcCall<Entity[]>("entities.list", { project_id: projectId }),
  search: (projectId: string, query: string, limit = 20) =>
    rpcCall<Entity[]>("entities.search", { project_id: projectId, query, limit }),
  get: (id: string) => rpcCall<Entity>("entities.get", { id }),
  create: (input: NewEntityInput) => rpcCall<Entity>("entities.create", input),
  update: (input: UpdateEntityInput) => rpcCall<Entity>("entities.update", input),
  scenes: (entityId: string) => rpcCall<SceneMention[]>("entities.scenes", { entity_id: entityId }),
};

export const mentions = {
  listForNode: (nodeId: string) =>
    rpcCall<Entity[]>("mentions.list_for_node", { node_id: nodeId }),
};

export const ai = {
  run: (nodeId: string, prompt: string, options: AIOptions, selectionText: string = "") =>
    rpcCall<{ run_id: string }>("ai.run", { node_id: nodeId, prompt, selection_text: selectionText, options }),
  cancel: (runId: string) => rpcCall<{ ok: true }>("ai.cancel", { run_id: runId }),
  previewContext: (nodeId: string, options?: AIOptions): Promise<AIContextPreview> =>
    rpcCall<ContextPreviewResponse>(
      "ai.preview_context",
      options ? { node_id: nodeId, options } : { node_id: nodeId },
    )
      .then((r) => {
        const counts: ContextCounts = {
          nearbyScenes: r.nearby_scenes,
          hasOutline: r.has_outline,
          hasSynopsis: r.has_synopsis,
          relatedScenes: r.related_scenes,
          entities: r.entities,
          relationships: r.relationships,
          plotBeats: r.plot_beats,
          notes: r.notes,
          projectMetaFields: r.project_meta_fields,
          hasStyleNotes: r.has_style_notes,
        };
        return {
          counts,
          sections: (r.sections ?? []).map((s) => ({
            id: s.id,
            label: s.label,
            present: s.present,
            selected: s.selected,
            count: s.count,
            preview: s.preview,
          })),
          selectedItemCount: r.selected_item_count ?? 0,
        };
      }),
};

export const threads = {
  create: (input: NewThreadInput) => rpcCall<Thread>("threads.create", input),
  list: (projectId: string, includeClosed = false) =>
    rpcCall<Thread[]>("threads.list", { project_id: projectId, include_closed: includeClosed }),
  get: (id: string) => rpcCall<Thread>("threads.get", { id }),
  update: (input: UpdateThreadInput) => rpcCall<Thread>("threads.update", input),
  close: (id: string) => rpcCall<Thread>("threads.close", { id }),
  reopen: (id: string) => rpcCall<Thread>("threads.reopen", { id }),
};

export const beats = {
  create: (input: NewBeatInput) => rpcCall<Beat>("beats.create", input),
  listByThread: (threadId: string) =>
    rpcCall<Beat[]>("beats.list_by_thread", { thread_id: threadId }),
  listByNode: (nodeId: string) =>
    rpcCall<Beat[]>("beats.list_by_node", { node_id: nodeId }),
  update: (input: UpdateBeatInput) => rpcCall<Beat>("beats.update", input),
  reorder: (threadId: string, ids: string[]) =>
    rpcCall<{ ok: true }>("beats.reorder", { thread_id: threadId, ids }),
  delete: (id: string) => rpcCall<{ ok: true }>("beats.delete", { id }),
};

export const notes = {
  create: (input: NewNoteInput) => rpcCall<Note>("notes.create", input),
  listForNode: (nodeId: string) =>
    rpcCall<Note[]>("notes.list_for_node", { node_id: nodeId }),
  get: (id: string) => rpcCall<Note>("notes.get", { id }),
  update: (input: UpdateNoteInput) => rpcCall<Note>("notes.update", input),
  delete: (id: string) => rpcCall<{ ok: true }>("notes.delete", { id }),
};

export const facts = {
  list: (projectId: string, nodeId?: string) =>
    rpcCall<FactCard[]>("facts.list", nodeId ? { project_id: projectId, node_id: nodeId } : { project_id: projectId }),
  create: (input: NewFactInput) => rpcCall<FactCard>("facts.create", input),
  createFromUrl: (input: NewFactFromUrlInput) => rpcCall<FactCard>("facts.create_from_url", input),
  update: (input: UpdateFactInput) => rpcCall<FactCard>("facts.update", input),
  delete: (id: string) => rpcCall<{ ok: true }>("facts.delete", { id }),
};

export const relationships = {
  createOne: (input: NewRelationshipInput) =>
    rpcCall<Relationship>("relationships.create_one", input),
  createPair: (input: NewRelationshipPairInput) =>
    rpcCall<Relationship[]>("relationships.create_pair", input),
  listByEntity: (entityId: string) =>
    rpcCall<Relationship[]>("relationships.list_by_entity", { entity_id: entityId }),
  update: (input: UpdateRelationshipInput) =>
    rpcCall<Relationship>("relationships.update", input),
  delete: (id: string) => rpcCall<{ ok: true }>("relationships.delete", { id }),
};

export const plot = {
  spinePanel: (nodeId: string) =>
    rpcCall<PlotSpine>("plot.spine_panel", { node_id: nodeId }),
};

export const companion = {
  send: (projectId: string, nodeId: string, text: string) =>
    rpcCall<{ run_id: string }>("companion.send", { project_id: projectId, node_id: nodeId, text }),
  history: (projectId: string) =>
    rpcCall<{ messages: CompanionMessage[] }>("companion.history", { project_id: projectId })
      .then((r) => r.messages ?? []),
  compact: (projectId: string) =>
    rpcCall<{ messages: CompanionMessage[] }>("companion.compact", { project_id: projectId })
      .then((r) => r.messages ?? []),
  clear: (projectId: string) =>
    rpcCall<{ ok: true }>("companion.clear", { project_id: projectId }),
  cancel: (runId: string) =>
    rpcCall<{ ok: true }>("companion.cancel", { run_id: runId }),
  remember: (projectId: string, text: string, category?: string) =>
    rpcCall<{ ok: true }>("companion.remember", { project_id: projectId, text, category }),
  applyOps: (projectId: string, nodeId: string | null, summary: string, ops: ProposalOp[]) =>
    rpcCall<CompanionApplyOpsResult>("companion.apply_ops", {
      project_id: projectId,
      node_id: nodeId ?? "",
      summary,
      ops,
    }),
};
