import type { CharacterVisualSheet, ReferenceImageMeta, ArtStyle, StyleApproveMode, StyleCheckResult, StyleDraftState, StyleRules } from "./types";
import type { FocusState, SceneIntent, SceneIntentState } from "./types";
import type { ContinuityCheck, ContinuityObservation, ContinuityOpenCounts, ContinuityStatus } from "./types";
import type {
  AcceptSuggestionsResult,
  ExtractionEstimate,
  ExtractionProgress,
  Forecast,
  ProposalOp,
  SelectForecastResult,
  SuggestionBatch,
  SuggestionItemStatus,
} from "./types";
import type { IllustrationComposition, IllustrationFormat, IllustrationGenerateInput, IllustrationMeta, IllustrationRepresentatives, IllustrationRequest, IllustrationRequestEdits, IllustrationRequestStatus, ImageProviderStatus } from "./types";
import { invoke } from "@tauri-apps/api/core";
import type {
  AgentHistoryRow,
  AgentImage,
  AgentRunResult,
  AgentRunStatus,
  ApplyContextResult,
  ApplyContextSelection,
  Beat,
  CodexLoginStart,
  CodexStatus,
  PublishConnectStatus,
  PublishCoverView,
  PublishEpisodeView,
  PublishResult,
  PublishSerialStatus,
  PublishSheetsStatus,
  PublishWorkStatus,
  PublishWorkView,
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
  MemoryDocument,
  MemoryState,
  Skill,
  OrphanedSkillsResult,
  SkillDeleteResult,
  SkillScope,
  SkillVersion,
  SkillWriteResult,
  SkillsListResult,
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
  ProviderStatus,
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

/** The engine's rpc.CodeContentConflict: a save was rejected because the
 *  node's content changed since the caller last read it (optimistic-lock
 *  mismatch on nodes.update_content). Kept in sync with
 *  engine/internal/rpc/server.go. */
export const RPC_CODE_CONTENT_CONFLICT = -32009;

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

export const memory = {
  get: (projectId: string) => rpcCall<MemoryState>("memory.get", { project_id: projectId }),
  set: (scope: MemoryDocument["scope"], projectId: string, body: string) =>
    rpcCall<MemoryDocument>("memory.set", { scope, project_id: projectId, body }),
};

/** The skills surface (#98). `list` returns BOTH scopes plus the diagnostics:
 *  a SKILL.md the writer broke by hand is never listed as a skill, because it
 *  must not reach a prompt, and must still be visible so it can be fixed.
 *
 *  `read` deliberately does not screen what it opens — that is the repair
 *  path, and a body over the cap cannot be trimmed by someone who cannot read
 *  it — while `write` does. `enabled` is optional on the way in: omitting it
 *  leaves a stored skill's flag alone and turns a new one on.
 *
 *  There is no rename. A skill's identity is (scope, work, name), which is
 *  what its version history is keyed on, so renaming through a write would
 *  strand every version under a name nothing on disk answers to. A rename is
 *  a delete plus a create. */
/** The work id a scoped call may carry. A writer-scope skill is global and
 *  the engine hard-refuses a work id on it (`agentskills.projectArg`), which
 *  means every caller would otherwise have to remember to blank the id it is
 *  already holding — the selected work does not stop being selected because
 *  the writer opened a writer-scope skill. #97 shipped that same
 *  `scope === … ? id : ""` by hand at every call site in MemorySection; here
 *  it lives once, in the wrapper, so no pane can forget it. */
const workIdFor = (scope: SkillScope, projectId: string) => (scope === "work" ? projectId : "");

export const skills = {
  list: (projectId: string) => rpcCall<SkillsListResult>("skills.list", { project_id: projectId }),
  read: (scope: SkillScope, projectId: string, name: string) =>
    rpcCall<Skill>("skills.read", { scope, project_id: workIdFor(scope, projectId), name }),
  write: (input: {
    scope: SkillScope;
    projectId: string;
    name: string;
    description: string;
    body: string;
    enabled?: boolean;
  }) =>
    rpcCall<SkillWriteResult>("skills.write", {
      scope: input.scope,
      project_id: workIdFor(input.scope, input.projectId),
      name: input.name,
      description: input.description,
      body: input.body,
      enabled: input.enabled,
    }),
  delete: (scope: SkillScope, projectId: string, name: string) =>
    rpcCall<SkillDeleteResult>("skills.delete", { scope, project_id: workIdFor(scope, projectId), name }),
  history: (scope: SkillScope, projectId: string, name: string, limit?: number) =>
    rpcCall<{ versions: SkillVersion[] }>("skills.history", {
      scope,
      project_id: workIdFor(scope, projectId),
      name,
      limit,
    }),
  restore: (id: string) => rpcCall<SkillWriteResult>("skills.restore", { id }),
  /** #119: names skill_snapshots remembers for this scope/work that have no
   *  file on disk — see OrphanedSkillsResult. Same "writer scope always,
   *  work scope only with a project id" rule as `list`. */
  orphaned: (projectId: string) => rpcCall<OrphanedSkillsResult>("skills.orphaned", { project_id: projectId }),
};

export interface BackupEntry {
  path: string;
  kind: "daily" | "pre_migration" | "recovery";
  size_bytes: number;
  created_at: number;
}

export interface BackupProject {
  id: string;
  title: string;
  word_count: number;
  updated_at: number;
  archived_at?: number;
}

export const backupApi = {
  createRecovery: () => rpcCall<{ path: string; format_version: number }>("backup.create_recovery"),
  list: () => rpcCall<{ backups: BackupEntry[] }>("backup.list"),
  peek: (path: string) => rpcCall<{ projects: BackupProject[] }>("backup.peek", { path }),
  /** Additive merge: the picked work comes back as a NEW project; the live
   *  library is never overwritten. The engine snapshots the library first. */
  restoreProject: (path: string, projectId: string, titleSuffix: string) =>
    rpcCall<{ project_id: string; title: string }>("backup.restore_project", {
      path,
      project_id: projectId,
      title_suffix: titleSuffix,
    }),
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

export const providers = {
  list: () => rpcCall<ProviderStatus[]>("providers.list"),
  listModels: (provider: string) =>
    rpcCall<{ models: string[] }>("providers.list_models", { provider }),
  test: (provider: string) => rpcCall<{ ok: true }>("providers.test", { provider }),
};

export const imageProviders = {
  list: () => rpcCall<ImageProviderStatus[]>("image_providers.list"),
  listModels: (provider: string) =>
    rpcCall<{ models: string[] }>("image_providers.list_models", { provider }),
};

export const illustrations = {
  compose: (input: { project_id: string; entity_ids?: string[]; names?: string[]; scene: string; format?: IllustrationFormat }) =>
    rpcCall<IllustrationComposition>("illustrations.compose", input),
  /** Runs as long as the provider draws — about 30 s for Codex. */
  generate: (input: IllustrationGenerateInput) => rpcCall<IllustrationMeta>("illustrations.generate", input),
  list: (projectId: string) =>
    rpcCall<{ items: IllustrationMeta[]; count: number; bytes: number }>("illustrations.list", { project_id: projectId }),
  get: (id: string) => rpcCall<IllustrationMeta & { data_base64: string }>("illustrations.get", { id }),
  update: (id: string, caption: string) => rpcCall<IllustrationMeta>("illustrations.update", { id, caption }),
  delete: (id: string) => rpcCall<{ ok: boolean }>("illustrations.delete", { id }),
  /** node id → the picture that stands for that scene, episode or part. */
  representatives: (projectId: string) =>
    rpcCall<IllustrationRepresentatives>("illustrations.representatives", { project_id: projectId }),
  /** An empty illustrationId clears the node's picture. */
  setRepresentative: (nodeId: string, illustrationId: string) =>
    rpcCall<{ previous: string }>("illustrations.set_representative", { node_id: nodeId, illustration_id: illustrationId }),
  /** Earlier pictures of the work a new one of these characters is drawn
   *  from, best first (at most two). */
  suggestPrior: (projectId: string, characterIds: string[]) =>
    rpcCall<IllustrationMeta[]>("illustrations.suggest_prior", { project_id: projectId, character_ids: characterIds }),
};

/** Pictures agents asked for. Only approve draws, and it runs as long as
 *  the provider draws (like illustrations.generate). */
export const illustrationRequests = {
  list: (projectId: string, statuses?: IllustrationRequestStatus[]) =>
    rpcCall<IllustrationRequest[]>("illustration_requests.list", { project_id: projectId, statuses }),
  get: (id: string) => rpcCall<IllustrationRequest>("illustration_requests.get", { id }),
  approve: (id: string, edits: IllustrationRequestEdits) =>
    rpcCall<IllustrationRequest>("illustration_requests.approve", { id, ...edits }),
  reject: (id: string) => rpcCall<IllustrationRequest>("illustration_requests.reject", { id }),
};

export const codex = {
  loginStart: () => rpcCall<CodexLoginStart>("codex.login_start"),
  loginStatus: () => rpcCall<CodexStatus>("codex.login_status"),
  logout: () => rpcCall<{ ok: true }>("codex.logout"),
};

/** Linetta Works publishing (Phase 3). Nothing here reaches the site until
 *  the writer connects: every call before that answers from this device. */
export const publishApi = {
  connectStart: (consented: boolean) =>
    rpcCall<PublishConnectStatus>("publish.connect_start", { consented }),
  connectStatus: () => rpcCall<PublishConnectStatus>("publish.connect_status"),
  /** Also abandons a connection attempt still waiting for the browser. */
  disconnect: () => rpcCall<{ ok: true }>("publish.disconnect"),
  workStatus: (projectId: string) =>
    rpcCall<PublishWorkStatus>("publish.work_status", { project_id: projectId }),
  upsertWork: (input: { project_id: string; blurb: string; serial_status: PublishSerialStatus; slug?: string }) =>
    rpcCall<PublishWorkView>("publish.work_upsert", input),
  /** nodeId may be the episode or any scene inside it. */
  publishEpisode: (nodeId: string) =>
    rpcCall<PublishResult>("publish.episode_publish", { node_id: nodeId }),
  setEpisodeVisibility: (projectId: string, nodeId: string, visibility: "public" | "private") =>
    rpcCall<PublishEpisodeView>("publish.episode_set_visibility", {
      project_id: projectId,
      node_id: nodeId,
      visibility,
    }),
  /** Whether the writer turned on publishing for a sheet (Works Phase 8). */
  sheetGet: (entityId: string) => rpcCall<{ published: boolean }>("publish.sheet_get", { entity_id: entityId }),
  /** Changes the choice only; sheetsSync sends it. */
  sheetSet: (entityId: string, published: boolean) =>
    rpcCall<{ published: boolean }>("publish.sheet_set", { entity_id: entityId, published }),
  /** An empty id clears the cover. */
  setCover: (projectId: string, illustrationId: string) =>
    rpcCall<{ cover_id: string }>("publish.cover_set", { project_id: projectId, illustration_id: illustrationId }),
  /** Chooses the picture as the cover and uploads it now, without the sheets. */
  publishCover: (projectId: string, illustrationId: string) =>
    rpcCall<PublishCoverView>("publish.cover_publish", { project_id: projectId, illustration_id: illustrationId }),
  /** Sends the chosen sheets and cover to the site. */
  sheetsSync: (projectId: string) => rpcCall<PublishSheetsStatus>("publish.sheets_sync", { project_id: projectId }),
};

/** The built-in agent's loop (#93). Its panel is #95; these wrappers are the
 *  only way the renderer can reach agent.* today — a manual JSON-RPC call is
 *  the alternative until the panel calls them. */
export const agent = {
  /** selection: a passage the writer highlighted in the open scene;
   *  images: pictures attached to the message. */
  run: (projectId: string, prompt: string, nodeId?: string, selection?: string, images?: AgentImage[]) =>
    rpcCall<AgentRunResult>("agent.run", {
      project_id: projectId,
      node_id: nodeId,
      prompt,
      selection,
      images: images && images.length > 0 ? images : undefined,
    }),
  cancel: (runId: string) => rpcCall<{ ok: true }>("agent.cancel", { run_id: runId }),
  history: (projectId: string, limit?: number) =>
    rpcCall<AgentHistoryRow[]>("agent.history", { project_id: projectId, limit }),
  status: (projectId: string) => rpcCall<AgentRunStatus>("agent.status", { project_id: projectId }),
  clear: (projectId: string) => rpcCall<{ ok: true }>("agent.clear", { project_id: projectId }),
  // Exactly one of batchId/snapshotId is expected — an outline batch from
  // linetta_apply_story_ops, or a scene write/revise's pre-write snapshot
  // (#112). rpcCall drops the other since it stringifies as undefined.
  undo: (batchId?: string, snapshotId?: string) =>
    rpcCall<{ ok: true }>("agent.undo", { batch_id: batchId, snapshot_id: snapshotId }),
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


export const visuals = {
 getSheet: (entityId: string) => rpcCall<CharacterVisualSheet>("visuals.get_sheet", { entity_id: entityId }),
 setSheet: (sheet: Omit<CharacterVisualSheet, "updated_at">) => rpcCall<CharacterVisualSheet>("visuals.set_sheet", sheet),
 listImages: (entityId: string) => rpcCall<ReferenceImageMeta[]>("visuals.list_images", { entity_id: entityId }),
 getImage: (id: string) => rpcCall<ReferenceImageMeta & { data_base64: string }>("visuals.get_image", { id }),
 addImage: (input: { entity_id: string; mime: string; caption: string; data_base64: string }) => rpcCall<ReferenceImageMeta>("visuals.add_image", input),
 updateImage: (id: string, caption: string) => rpcCall<ReferenceImageMeta>("visuals.update_image", { id, caption }),
 deleteImage: (id: string) => rpcCall<{ ok: boolean }>("visuals.delete_image", { id }),
 getArtStyle: (projectId: string) => rpcCall<ArtStyle>("visuals.get_art_style", { project_id: projectId }),
 setArtStyle: (style: Omit<ArtStyle, "updated_at">) => rpcCall<ArtStyle>("visuals.set_art_style", style),
};

export const style = {
  getRules: (projectId: string) => rpcCall<StyleRules>("style.get_rules", { project_id: projectId }),
  setRules: (rules: Omit<StyleRules, "updated_at">) => rpcCall<StyleRules>("style.set_rules", rules),
  /** Checks one scene (or the scenes under a chapter) when nodeId is given, the whole work otherwise. */
  check: (projectId: string, nodeId?: string) =>
    rpcCall<StyleCheckResult>("style.check", { project_id: projectId, node_id: nodeId ?? "" }),
  getDraft: (projectId: string) => rpcCall<StyleDraftState>("style.get_draft", { project_id: projectId }),
  /** The writer's approval — the only way a draft becomes style notes. Returns the new notes. */
  approveDraft: (projectId: string, body: string, mode: StyleApproveMode) =>
    rpcCall<{ style_notes: string }>("style.approve_draft", { project_id: projectId, body, mode }),
  discardDraft: (projectId: string) => rpcCall<{ ok: boolean }>("style.discard_draft", { project_id: projectId }),
};

/** The writer's own direction (story-assist Phase 1): no MCP tool can write either. */
export const direction = {
  getFocus: (projectId: string) => rpcCall<FocusState>("direction.get_focus", { project_id: projectId }),
  /** Empty body clears the focus. The engine, not the caller, picks the anchor episode. */
  setFocus: (projectId: string, body: string) =>
    rpcCall<FocusState>("direction.set_focus", { project_id: projectId, body }),
  getSceneIntent: (nodeId: string) => rpcCall<SceneIntentState>("direction.get_scene_intent", { node_id: nodeId }),
  /** goal, keep and avoid all empty clears the scene's intent. */
  setSceneIntent: (nodeId: string, goal: string, keep: string[], avoid: string[]) =>
    rpcCall<SceneIntent>("direction.set_scene_intent", { node_id: nodeId, goal, keep, avoid }),
};

/** Continuity checks (story-assist Phase 2): engine-verified observations an agent recorded. */
export const continuity = {
  list: (projectId: string, nodeId?: string, statuses?: ContinuityStatus[]) =>
    rpcCall<{ checks: ContinuityCheck[] }>("continuity.list", {
      project_id: projectId,
      node_id: nodeId,
      statuses,
    }),
  setStatus: (observationId: string, status: ContinuityStatus) =>
    rpcCall<ContinuityObservation>("continuity.set_status", { observation_id: observationId, status }),
  openCounts: (projectId: string) =>
    rpcCall<ContinuityOpenCounts>("continuity.open_counts", { project_id: projectId }),
};

/** The suggestions inbox (story-assist Phase 3): an agent's
 *  linetta_propose_story_ops candidates, and the writer's item-by-item
 *  accept/reject of them. Nothing here applies anything on its own except
 *  accept, which runs the chosen items through storyops. */
export const proposals = {
  list: (projectId: string, statuses?: SuggestionItemStatus[]) =>
    rpcCall<{ proposals: SuggestionBatch[] }>("proposals.list", { project_id: projectId, statuses }),
  accept: (projectId: string, itemIds: string[], edits?: Record<string, ProposalOp>) =>
    rpcCall<AcceptSuggestionsResult>("proposals.accept", { project_id: projectId, item_ids: itemIds, edits }),
  reject: (projectId: string, itemIds: string[]) =>
    rpcCall<{ ok: boolean }>("proposals.reject", { project_id: projectId, item_ids: itemIds }),
  pendingCount: (projectId: string) =>
    rpcCall<{ count: number }>("proposals.pending_count", { project_id: projectId }),
  extractionEstimate: (projectId: string) =>
    rpcCall<ExtractionEstimate>("proposals.extraction_estimate", { project_id: projectId }),
  extractionProgress: (projectId: string) =>
    rpcCall<ExtractionProgress>("proposals.extraction_progress", { project_id: projectId }),
  /** Reverts one purpose=forecast structural batch (AcceptSuggestionsResult's
   *  undo_batch_ids / SuggestionItem.undo_batch_id) — the outline, and any
   *  beat an add_beat item added onto a scene the batch created. */
  undo: (projectId: string, batchId: string) =>
    rpcCall<{ ok: boolean }>("proposals.undo", { project_id: projectId, batch_id: batchId }),
};

/** "Where the story could go" (story-assist Phase 4): possible
 *  continuations an agent drafted with linetta_save_forecast, for the
 *  writer to compare. Select/clearSelection/delete are direct RPCs — no
 *  LLM turn. Never canon: see the Forecast type's own doc comment. */
export const forecast = {
  list: (projectId: string) =>
    rpcCall<{ forecasts: Forecast[] }>("forecast.list", { project_id: projectId }),
  // project_id scopes select/delete to the caller's own project (review
  // finding f2) — without it, a stale forecast_id left over from a project
  // the writer has since navigated away from could change or delete a
  // DIFFERENT project's forecast with no error at all.
  select: (projectId: string, forecastId: string, branchId: string) =>
    rpcCall<SelectForecastResult>("forecast.select", {
      project_id: projectId, forecast_id: forecastId, branch_id: branchId,
    }),
  clearSelection: (projectId: string) =>
    rpcCall<{ ok: boolean }>("forecast.clear_selection", { project_id: projectId }),
  delete: (projectId: string, forecastId: string) =>
    rpcCall<{ ok: boolean }>("forecast.delete", { project_id: projectId, forecast_id: forecastId }),
};
