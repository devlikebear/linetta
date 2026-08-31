import { invoke } from "@tauri-apps/api/core";
import type {
  ApplyContextResult,
  ApplyContextSelection,
  Beat,
  ConsistencyInput,
  ConsistencyReport,
  ContextChangeInput,
  ContextChangePlan,
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
  McpActivityEntry,
  McpClientStatus,
  McpConnectOutcome,
  McpStatus,
  McpTokenResult,
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
  Relationship,
  ResolveTargetInput,
  ReplacePlan,
  ApplyReplaceResult,
  SceneMention,
  SearchResult,
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

/** Tray residence + login autostart (#81). Desktop shells only — the invoke
 *  rejects on mobile, which the settings pane uses to hide the section. */
export interface BackgroundPrefs {
  close_to_tray: boolean;
  autostart: boolean;
}

export async function backgroundPrefsGet(): Promise<BackgroundPrefs> {
  return invoke<BackgroundPrefs>("background_prefs_get");
}

export async function backgroundPrefsSet(patch: {
  closeToTray?: boolean;
  autostart?: boolean;
  language?: string;
}): Promise<BackgroundPrefs> {
  return invoke<BackgroundPrefs>("background_prefs_set", patch);
}

export async function restoreLatestBackup(): Promise<{
  backup_path: string;
  quarantined_path: string | null;
}> {
  return invoke("restore_latest_backup");
}

/** Absolute path to the bundled MCP bridge, or null when this build ships
 *  without it (Mac App Store) or the dev build has not run the build script.
 *  The settings pane prints it into the writer's client config, so a guess
 *  would produce a snippet that silently fails. */
export async function mcpBridgePath(): Promise<string | null> {
  return invoke<string | null>("mcp_bridge_path");
}

/** Which supported MCP clients are on this machine, and whether their config
 *  already carries a linetta entry. */
export async function mcpClientStatus(): Promise<McpClientStatus[]> {
  return invoke<McpClientStatus[]>("mcp_client_status");
}

/** One-click connect: registers the stdio bridge with the given client. The
 *  shell backs up any config file it touches. */
export async function mcpConnectClient(client: string): Promise<McpConnectOutcome> {
  return invoke<McpConnectOutcome>("mcp_connect_client", { client });
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


export const projects = {
  create: (input: NewProjectInput) => rpcCall<Project>("projects.create", input),
  list: (params: ListProjectsParams = {}) => rpcCall<Project[]>("projects.list", params),
  get: (id: string) => rpcCall<Project>("projects.get", { id }),
  update: (input: UpdateProjectInput) => rpcCall<Project>("projects.update", input),
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




export const exportApi = {
  project: (projectId: string) =>
    rpcCall<ExportPayload>("export.project", { project_id: projectId }),
  node: (nodeId: string) =>
    rpcCall<ExportPayload>("export.node", { node_id: nodeId }),
  nodeText: (nodeId: string) =>
    rpcCall<ExportTextPayload>("export.nodeText", { node_id: nodeId }),
  /** Every project's companion transcript and remembered facts, in one
   *  markdown archive. Exists so the companion's removal does not take the
   *  writer's conversations with it. */
  companionHistory: () => rpcCall<ExportPayload>("export.companion_history"),
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

export const mcp = {
  status: () => rpcCall<McpStatus>("mcp.status"),
  enable: () => rpcCall<McpTokenResult>("mcp.enable"),
  disable: () => rpcCall<McpStatus>("mcp.disable"),
  regenerateToken: () => rpcCall<McpTokenResult>("mcp.regenerate_token"),
  activity: (limit?: number) => rpcCall<McpActivityEntry[]>("mcp.activity", { limit }),
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
  list: (projectId: string) =>
    rpcCall<Relationship[]>("relationships.list", { project_id: projectId }),
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

