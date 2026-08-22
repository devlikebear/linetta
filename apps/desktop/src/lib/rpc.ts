import { invoke } from "@tauri-apps/api/core";
import type {
  AIOptions,
  AIContextPreview,
  CompanionImageAttachment,
  CompanionHistoryScope,
  CompanionIntent,
  CompanionReference,
  CompanionReferenceInput,
  CompanionReferencePatch,
  CompanionApplyOpsResult,
  ApplyContextResult,
  ApplyContextSelection,
  Beat,
  CompanionMessage,
  ConsistencyInput,
  ConsistencyReport,
  ContextChangeInput,
  ContextChangePlan,
  ContextCounts,
  ContextPreviewResponse,
  ContextTarget,
  DiagnosticsSnapshot,
  EngineStatus,
  Entity,
  ExportPayload,
  ExportTextPayload,
  FactCard,
  FolderSyncResult,
  GitSyncInitResult,
  GitSyncResult,
  ImportMarkdownResult,
  ImportPreviewResult,
  ListProjectsParams,
  ManuscriptSearchHit,
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
  NodeStatus,
  Note,
  OpsStatus,
  PlotSpine,
  Project,
  ProposalOp,
  Relationship,
  ResolveTargetInput,
  ReplacePlan,
  ApplyReplaceResult,
  SceneMention,
  SearchResult,
  ProviderID,
  ProviderTestResult,
  OpenRouterKeyInfo,
  OpenRouterOAuthFinish,
  OpenRouterOAuthStart,
  Settings,
  SettingsPatch,
  Snapshot,
  SnapshotCompareResult,
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
  WritingStatsDay,
  WritingStatsSummary,
  WritingStatsToday,
} from "./types";

// Tauri commands defined in src-tauri.

export async function enginePing(): Promise<string> {
  return invoke<string>("engine_ping");
}

export async function engineStatus(): Promise<EngineStatus> {
  return invoke<EngineStatus>("engine_status");
}

export async function openRecoveryFolder(): Promise<void> {
  return invoke<void>("open_recovery_folder");
}

export async function restoreLatestBackup(): Promise<{
  backup_path: string;
  quarantined_path: string | null;
}> {
  return invoke("restore_latest_backup");
}

export async function openPath(path: string): Promise<void> {
  return invoke<void>("open_path", { path });
}

/** Open an external URL in the OS browser. `window.open` cannot reach it. */
export async function openExternalUrl(url: string): Promise<void> {
  return invoke<void>("open_external_url", { url });
}

export async function setFolderSyncDir(path: string): Promise<void> {
  return invoke<void>("set_folder_sync_dir", { path });
}

export async function folderSyncNow(): Promise<FolderSyncResult> {
  return invoke<FolderSyncResult>("folder_sync_now");
}

export class RpcError extends Error {
  readonly code?: number;
  readonly data?: unknown;
  readonly method: string;
  readonly requestId?: number;

  constructor(method: string, message: string, code?: number, data?: unknown, requestId?: number) {
    super(message);
    this.name = "RpcError";
    this.method = method;
    this.code = code;
    this.data = data;
    this.requestId = requestId;
  }
}

function normalizeRpcError(method: string, error: unknown): RpcError {
  if (error && typeof error === "object") {
    const wire = error as { code?: unknown; message?: unknown; data?: unknown; request_id?: unknown };
    if (typeof wire.message === "string") {
      return new RpcError(
        method,
        wire.message,
        typeof wire.code === "number" ? wire.code : undefined,
        wire.data,
        typeof wire.request_id === "number" ? wire.request_id : undefined,
      );
    }
  }
  return new RpcError(method, error instanceof Error ? error.message : String(error));
}

export async function rpcCall<T>(method: string, params?: unknown): Promise<T> {
  try {
    return await invoke<T>("engine_call", { method, params: params ?? null });
  } catch (error) {
    throw normalizeRpcError(method, error);
  }
}

function mapContextPreviewResponse(r: ContextPreviewResponse): AIContextPreview {
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
      charCount: s.char_count ?? 0,
      tokenEstimate: s.token_estimate ?? 0,
    })),
    selectedItemCount: r.selected_item_count ?? 0,
    selectedCharCount: r.selected_char_count ?? 0,
    selectedTokenEstimate: r.selected_token_estimate ?? 0,
    budgetTokenEstimate: r.budget_token_estimate ?? 0,
  };
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
  updateContent: (id: string, doc: string, expectedContentVersion?: number) =>
    rpcCall<NodeRow>("nodes.update_content", {
      id,
      doc,
      ...(expectedContentVersion === undefined
        ? {}
        : { expected_content_version: expectedContentVersion }),
    }),
  setLastOpened: (projectId: string, nodeId: string) =>
    rpcCall<{ ok: true }>("nodes.set_last_opened", { project_id: projectId, node_id: nodeId }),
  listTree: (projectId: string) =>
    rpcCall<NodeRow[]>("nodes.list_tree", { project_id: projectId }),
  createSibling: (referenceId: string, kind: "leaf" | "container", label: string, title: string) =>
    rpcCall<NodeRow>("nodes.create_sibling", { reference_id: referenceId, kind, label, title }),
  createChild: (parentId: string, kind: "leaf" | "container", label: string, title: string) =>
    rpcCall<NodeRow>("nodes.create_child", { parent_id: parentId, kind, label, title }),
  moveToParent: (id: string, parentId: string) =>
    rpcCall<{ ok: true }>("nodes.move_to_parent", { id, parent_id: parentId }),
  moveTo: (id: string, parentId: string | null, ordinal: number) =>
    rpcCall<{ ok: true }>("nodes.move_to", { id, parent_id: parentId, ordinal }),
  moveToRoot: (id: string) =>
    rpcCall<{ ok: true }>("nodes.move_to_root", { id }),
  convertToContainer: (id: string) =>
    rpcCall<{ ok: true }>("nodes.convert_to_container", { id }),
  restoreOutline: (projectId: string, snapshot: NodeRow[]) =>
    rpcCall<{ ok: true }>("nodes.restore_outline", { project_id: projectId, nodes: snapshot }),
  rename: (id: string, label: string, title: string) =>
    rpcCall<{ ok: true }>("nodes.rename", { id, label, title }),
  delete: (id: string) => rpcCall<{ ok: true }>("nodes.delete", { id }),
  moveUp: (id: string) => rpcCall<{ ok: true }>("nodes.move_up", { id }),
  moveDown: (id: string) => rpcCall<{ ok: true }>("nodes.move_down", { id }),
  setStatus: (id: string, status: NodeStatus) =>
    rpcCall<{ ok: true }>("nodes.set_status", { id, status }),
};

export const stats = {
  today: (projectId: string) => rpcCall<WritingStatsToday>("stats.today", { project_id: projectId }),
  range: (projectId: string, fromDay: string, toDay: string) =>
    rpcCall<WritingStatsDay[]>("stats.range", { project_id: projectId, from_day: fromDay, to_day: toDay }),
  summary: (projectId: string) =>
    rpcCall<WritingStatsSummary>("stats.summary", { project_id: projectId }),
};

export const snapshots = {
  createManual: (nodeId: string, doc: string) =>
    rpcCall<Snapshot>("snapshots.create_manual", { node_id: nodeId, doc }),
  createAuto: (nodeId: string, doc: string) =>
    rpcCall<Snapshot | { skipped: true }>("snapshots.create_auto", { node_id: nodeId, doc }),
  listForNode: (nodeId: string) =>
    rpcCall<SnapshotEntry[]>("snapshots.list_for_node", { node_id: nodeId }),
  compare: (leftId: string, rightId: string) =>
    rpcCall<SnapshotCompareResult>("snapshots.compare", { left_id: leftId, right_id: rightId }),
  restore: (snapshotId: string) =>
    rpcCall<NodeRow>("snapshots.restore", { snapshot_id: snapshotId }),
};

export const settings = {
  get: () => rpcCall<Settings>("settings.get"),
  set: (patch: SettingsPatch) => rpcCall<Settings>("settings.set", patch),
};

export const backupApi = {
  createRecovery: () => rpcCall<{ path: string; format_version: number }>("backup.create_recovery"),
};

export const providers = {
  listModels: (provider: ProviderID) =>
    rpcCall<{ models: string[] }>("providers.list_models", { provider }),
  detectCli: () => rpcCall<{ path: string }>("providers.detect_cli"),
  test: (provider: ProviderID) =>
    rpcCall<ProviderTestResult>("providers.test", { provider }),
};

export const openRouter = {
  keyInfo: () => rpcCall<OpenRouterKeyInfo>("openrouter.key_info"),
  oauthStart: () => rpcCall<OpenRouterOAuthStart>("openrouter.oauth_start"),
  oauthFinish: (requestId: string) =>
    rpcCall<OpenRouterOAuthFinish>("openrouter.oauth_finish", { request_id: requestId }),
};

export const webSearch = {
  test: () => rpcCall<WebSearchTestResult>("web_search.test"),
};

export const exportApi = {
  project: (projectId: string) =>
    rpcCall<ExportPayload>("export.project", { project_id: projectId }),
  node: (nodeId: string) =>
    rpcCall<ExportPayload>("export.node", { node_id: nodeId }),
  nodeText: (nodeId: string) =>
    rpcCall<ExportTextPayload>("export.nodeText", { node_id: nodeId }),
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

export const manuscript = {
  search: (projectId: string, query: string, limit = 20) =>
    rpcCall<ManuscriptSearchHit[]>("manuscript.search", { project_id: projectId, query, limit }),
  replacePreview: (projectId: string, query: string, replacement: string, nodeIds: string[] = []) =>
    rpcCall<ReplacePlan>("manuscript.replace_preview", {
      project_id: projectId,
      query,
      replacement,
      node_ids: nodeIds,
    }),
  replaceApply: (plan: ReplacePlan, candidateIds: string[]) =>
    rpcCall<ApplyReplaceResult>("manuscript.replace_apply", { plan, candidate_ids: candidateIds }),
};

export const contextual = {
  resolveTarget: (input: ResolveTargetInput) =>
    rpcCall<ContextTarget>("contextual.resolve_target", input),
  planChange: (input: ContextChangeInput) =>
    rpcCall<ContextChangePlan>("contextual.plan_change", input),
  applyChange: (plan: ContextChangePlan, selection: ApplyContextSelection) =>
    rpcCall<ApplyContextResult>("contextual.apply_change", { plan, selection }),
  checkConsistency: (input: ConsistencyInput) =>
    rpcCall<ConsistencyReport>("contextual.check_consistency", input),
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
      .then(mapContextPreviewResponse),
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
  send: (projectId: string, nodeId: string, text: string, options?: Pick<AIOptions, "context" | "outline_structure"> & { images?: CompanionImageAttachment[]; intent?: CompanionIntent; scope?: CompanionHistoryScope; language?: string }) =>
    rpcCall<{ run_id: string }>("companion.send", {
      project_id: projectId,
      node_id: nodeId,
      text,
      options: options ? { context: options.context, outline_structure: options.outline_structure, intent: options.intent, scope: options.scope, language: options.language } : {},
      images: options?.images ?? [],
    }),
  previewContext: (projectId: string, nodeId: string, options?: Pick<AIOptions, "context">): Promise<AIContextPreview> =>
    rpcCall<ContextPreviewResponse>(
      "companion.preview_context",
      { project_id: projectId, node_id: nodeId, options: options ?? {} },
    ).then(mapContextPreviewResponse),
  history: (projectId: string, nodeId?: string | null, scope?: CompanionHistoryScope, limit?: number) =>
    rpcCall<{ messages: CompanionMessage[] }>("companion.history", {
      project_id: projectId,
      ...(nodeId ? { node_id: nodeId } : {}),
      ...(scope ? { scope } : {}),
      ...(limit ? { limit } : {}),
    })
      .then((r) => r.messages ?? []),
  compact: (projectId: string, nodeId?: string | null, scope?: CompanionHistoryScope, language?: string) =>
    rpcCall<{ messages: CompanionMessage[] }>("companion.compact", {
      project_id: projectId,
      ...(nodeId ? { node_id: nodeId } : {}),
      ...(scope ? { scope } : {}),
      ...(language ? { language } : {}),
    })
      .then((r) => r.messages ?? []),
  clear: (projectId: string, nodeId?: string | null, scope?: CompanionHistoryScope) =>
    rpcCall<{ ok: true }>("companion.clear", {
      project_id: projectId,
      ...(nodeId ? { node_id: nodeId } : {}),
      ...(scope ? { scope } : {}),
    }),
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
  references: {
    list: (projectId: string, nodeId?: string | null, includeDisabled = true) =>
      rpcCall<{ references: CompanionReference[] }>("companion.references.list", {
        project_id: projectId,
        ...(nodeId ? { node_id: nodeId } : {}),
        include_disabled: includeDisabled,
      }).then((r) => r.references ?? []),
    create: (input: CompanionReferenceInput) =>
      rpcCall<CompanionReference>("companion.references.create", input),
    update: (input: CompanionReferencePatch) =>
      rpcCall<CompanionReference>("companion.references.update", input),
    delete: (projectId: string, id: string) =>
      rpcCall<{ ok: true }>("companion.references.delete", { project_id: projectId, id }),
  },
};
