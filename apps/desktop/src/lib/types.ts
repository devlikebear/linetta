// Mirrors engine/internal/project Project struct (JSON tag names).
export type LengthTarget = "flash" | "short" | "novella" | "novel" | "series";
export type DefaultPOV = "first" | "third_limited" | "omniscient";

export interface Project {
  id: string;
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
  style_notes: string;
  outline: string;
  word_count: number;
  last_opened_node_id?: string;
  created_at: number;
  updated_at: number;
  archived_at?: number;
}

export interface UpdateProjectInput {
  id: string;
  outline?: string;
}

export interface NewProjectInput {
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
}

export interface ListProjectsParams {
  include_archived?: boolean;
  limit?: number;
}

// Mirrors engine/internal/node Node struct.
export type NodeKind = "container" | "leaf";
export type NodeStatus = "draft" | "revision" | "final";

export interface SearchResult {
  project_id: string;
  project_title: string;
  node_id: string;
  node_label: string;
  node_title: string;
  node_kind: NodeKind;
  preview: string;
  updated_at: number;
}

export interface NodeRow {
  id: string;
  project_id: string;
  parent_id?: string;
  ordinal: number;
  kind: NodeKind;
  label: string;
  title: string;
  content_doc?: string; // raw JSON string for leaves
  status: NodeStatus;
  word_count: number;
  created_at: number;
  updated_at: number;
}

// Mirrors engine/internal/snapshot Snapshot struct.
export type SnapshotReason = "manual" | "autosave" | "ai-replace";

export interface Snapshot {
  id: string;
  node_id: string;
  content_doc: string;
  reason: SnapshotReason;
  created_at: number;
}

// Mirrors engine/internal/entity Entity struct.
export type EntityKind = "character" | "place" | "item" | "concept";

export interface Entity {
  id: string;
  project_id: string;
  kind: EntityKind;
  name: string;
  aliases: string[];
  role: string;
  summary: string;
  attributes: Record<string, string>;
  created_at: number;
  updated_at: number;
}

export interface NewEntityInput {
  project_id: string;
  kind: EntityKind;
  name: string;
  role?: string;
}

export interface UpdateEntityInput {
  id: string;
  kind?: EntityKind;
  name?: string;
  role?: string;
  summary?: string;
  attributes?: Record<string, string>;
}

export interface SceneMention {
  node_id: string;
  label: string;
}

export type ToneID =
  | "my"
  | "cool"
  | "sensory"
  | "dry"
  | "tense"
  | "lyrical"
  | "humor";

export interface AIOptions {
  tone: ToneID;
  short_form: boolean;
}

export interface AIDelta {
  run_id: string;
  text: string;
}

// Sent when the running text must be REPLACED (not appended) — emitted by the
// engine's dedup when an upstream retry diverges from the first attempt.
export interface AIReset {
  run_id: string;
  text: string;
}

export interface AIDone {
  run_id: string;
  full_text: string;
}

export interface AIError {
  run_id: string;
  message: string;
}

export interface AICancelled {
  run_id: string;
}

export interface EngineStatus {
  ok: boolean;
  error?: string;
  version?: string;
  home?: string;
  db_path?: string;
  migration_version?: number;
  migration_count?: number;
}

export interface OpsStatus {
  job_name: string;
  last_started_at?: number;
  last_finished_at?: number;
  last_ok: boolean;
  last_error: string;
  metadata_json: string;
}

export interface DiagnosticsSnapshot {
  version: string;
  home: string;
  db_path: string;
  migration_version: number;
  migration_count: number;
  ops_status: OpsStatus[];
}

// Wire shape from ai.preview_context RPC. Mirrors engine PreviewCounts JSON.
// Mapped to ContextCounts (camelCase) inside the rpc client.
export interface ContextPreviewResponse {
  nearby_scenes: number;
  has_outline: boolean;
  has_synopsis: boolean;
  related_scenes: number;
  entities: number;
  relationships: number;
  plot_beats: number;
  notes: number;
  project_meta_fields: number;
  has_style_notes: boolean;
}

/** Camel-case context-payload counts surfaced by the FE. Wire shape is
 *  ContextPreviewResponse (snake_case); rpc.ts maps wire → camel. */
export interface ContextCounts {
  nearbyScenes: number;
  hasOutline: boolean;
  hasSynopsis: boolean;
  relatedScenes: number;
  entities: number;
  relationships: number;
  plotBeats: number;
  notes: number;
  projectMetaFields: number;
  hasStyleNotes: boolean;
}

export interface GitSyncResult {
  skipped: boolean;
  files_written: number;
  committed: boolean;
  pushed: boolean;
  message: string;
  error: string;
}

export interface GitSyncInitResult {
  skipped: boolean;
  already_repo: boolean;
  created: boolean;
  dir: string;
  error: string;
}

export type ProviderID =
  | "claude-code-cli"
  | "openai-codex"
  | "anthropic"
  | "openai"
  | "gemini-native";
export type WebSearchProvider = "brave" | "perplexity";

export interface ProviderConfig {
  model?: string;
  api_key?: string;
  base_url?: string;
  cli_path?: string;
}

export interface Settings {
  provider: ProviderID;
  providers?: Record<string, ProviderConfig>;
  typewriter_default: boolean;
  focus_default: boolean;
  git_sync_dir: string;
  git_sync_commit_template: string;
  backup_dir: string;
  safety_checklist_dismissed: boolean;
  web_search_provider: WebSearchProvider;
  web_search_api_key: string;
}

export interface SettingsPatch {
  provider?: ProviderID;
  providers?: Record<string, ProviderConfig>;
  typewriter_default?: boolean;
  focus_default?: boolean;
  git_sync_dir?: string;
  git_sync_commit_template?: string;
  safety_checklist_dismissed?: boolean;
  web_search_provider?: WebSearchProvider;
  web_search_api_key?: string;
}

export interface SnapshotEntry {
  id: string;
  reason: "manual" | "autosave" | "ai-replace";
  created_at: number;
  doc_preview: string;
}

export interface ExportPayload {
  markdown: string;
  suggested_filename: string;
}

export interface ImportMarkdownResult {
  project_id: string;
  container_count: number;
  leaf_count: number;
  warnings: string[];
}

export interface ImportPreviewNode {
  label: string;
  kind: "container" | "leaf";
  children?: ImportPreviewNode[];
}

export interface ImportPreviewResult {
  title: string;
  container_count: number;
  leaf_count: number;
  warnings: string[];
  roots: ImportPreviewNode[];
}

// Mirrors engine/internal/thread Thread struct.
export interface Thread {
  id: string;
  project_id: string;
  name: string;
  color: string;
  summary: string;
  closed_at?: number;
}

export interface NewThreadInput {
  project_id: string;
  name: string;
  color?: string;
}

export interface UpdateThreadInput {
  id: string;
  name?: string;
  color?: string;
  summary?: string;
}

// Mirrors engine/internal/beat Beat struct.
export interface Beat {
  id: string;
  thread_id: string;
  node_id?: string;
  ordinal: number;
  label: string;
  description: string;
  intensity: number;
}

export interface NewBeatInput {
  thread_id: string;
  node_id?: string;
  label?: string;
  description?: string;
  intensity?: number;
}

export interface UpdateBeatInput {
  id: string;
  label?: string;
  description?: string;
  intensity?: number;
}

// Mirrors engine/internal/plot Spine / SceneBeats / Beat (plot.spine_panel RPC).
export interface PlotBeat {
  id: string;
  thread_id: string;
  thread_name: string;
  thread_color: string;
  label: string;
  description: string;
  intensity: number;
  ordinal: number;
}

export interface PlotScene {
  node_id: string;
  label: string;
  beats: PlotBeat[];
}

export interface PlotSpine {
  prev: PlotScene | null;
  current: PlotScene;
  next: PlotScene | null;
}

// Mirrors engine/internal/relationship Relationship struct.
export interface Relationship {
  id: string;
  project_id: string;
  from_id: string;
  to_id: string;
  label: string;
  notes: string;
  pair_id?: string;
}

export interface NewRelationshipInput {
  project_id: string;
  from_id: string;
  to_id: string;
  label: string;
  notes?: string;
}

export interface NewRelationshipPairInput {
  project_id: string;
  from_id: string;
  to_id: string;
  label: string;
  inverse_label: string;
  notes?: string;
}

export interface UpdateRelationshipInput {
  id: string;
  label: string;
  notes: string;
}

// Mirrors engine/internal/note Note struct.
export interface Note {
  id: string;
  node_id: string;
  anchor: number;
  body: string;
  created_at: number;
}

export interface NewNoteInput {
  node_id: string;
  anchor: number;
  body: string;
}

export interface UpdateNoteInput {
  id: string;
  body: string;
}

// Mirrors engine/internal/ai NoteBrief (used in ai_runs.context_json).
export interface NoteBrief {
  anchor: number;
  body: string;
}

// Companion (Phase 2) — mirrors engine companion.* payloads.
export interface CompanionMessage {
  role: string;
  content: string;
  timestamp: number;
}

export type ProposalOpType =
  | "create_thread" | "update_thread"
  | "add_beat" | "update_beat" | "delete_beat"
  | "set_outline"
  | "remember"
  | "create_entity" | "update_entity" | "create_relationship"
  | "create_scene";

export interface ProposalOp {
  op: ProposalOpType;
  ref?: string;
  name?: string;
  color?: string;
  summary?: string;
  thread_id?: string;
  thread_ref?: string;
  node_id?: string;
  beat_id?: string;
  label?: string;
  description?: string;
  intensity?: number;
  outline?: string;
  text?: string;
  category?: string;
  kind?: string;
  role?: string;
  entity_id?: string;
  from?: string;
  from_ref?: string;
  to?: string;
  to_ref?: string;
  notes?: string;
  after_node_id?: string;
  title?: string;
  node_ref?: string;
  inverse_label?: string;
}

export interface CompanionProposal {
  run_id: string;
  valid: boolean;
  summary?: string;
  ops?: ProposalOp[];
  error?: string;
}

export interface CompanionApplyOpsFailure {
  index: number;
  op?: string;
  error: string;
}

export interface CompanionApplyOpsResult {
  summary?: string;
  applied: number;
  created?: Record<string, string>;
  failures?: CompanionApplyOpsFailure[];
}

export interface CompanionDelta { run_id: string; text: string; }
export interface CompanionReset { run_id: string; text: string; }
export interface CompanionDone { run_id: string; full_text: string; }
export interface CompanionError { run_id: string; message: string; }
export interface CompanionCancelled { run_id: string; }
export interface CompanionApplied { run_id: string; summary?: string; applied: number; }
export interface CompanionThinking { run_id: string; text: string; }
export interface CompanionReasoning { run_id: string; text: string; }
