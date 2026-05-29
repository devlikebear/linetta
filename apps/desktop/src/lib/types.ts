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
  word_count: number;
  last_opened_node_id?: string;
  created_at: number;
  updated_at: number;
  archived_at?: number;
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

// Wire shape from ai.preview_context RPC. Mirrors engine PreviewCounts JSON.
// Mapped to ContextCounts (camelCase) inside the rpc client.
export interface ContextPreviewResponse {
  nearby_scenes: number;
  same_chapter: number;
  other_chapter: number;
  other_part: number;
  has_synopsis: boolean;
  related_scenes: number;
  entities: number;
  active_threads: number;
  notes: number;
  project_meta_fields: number;
  has_style_notes: boolean;
}

/** Camel-case context-payload counts surfaced by the FE. Wire shape is
 *  ContextPreviewResponse (snake_case); rpc.ts maps wire → camel. */
export interface ContextCounts {
  nearbyScenes: number;
  sameChapter: number;
  otherChapter: number;
  otherPart: number;
  hasSynopsis: boolean;
  relatedScenes: number;
  entities: number;
  activeThreads: number;
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

export type ProviderID = "claude-code-cli" | "openai-codex";

export interface Settings {
  provider: ProviderID;
  typewriter_default: boolean;
  focus_default: boolean;
  git_sync_dir: string;
  git_sync_commit_template: string;
  backup_dir: string;
}

export interface SettingsPatch {
  provider?: ProviderID;
  typewriter_default?: boolean;
  focus_default?: boolean;
  git_sync_dir?: string;
  git_sync_commit_template?: string;
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
  intensity: number;
}

export interface NewBeatInput {
  thread_id: string;
  node_id?: string;
  label?: string;
  intensity?: number;
}

export interface UpdateBeatInput {
  id: string;
  label?: string;
  intensity?: number;
}

// Mirrors engine/internal/ai ActiveThread / BeatBrief (used in ai_runs.context_json).
export interface BeatBrief {
  label: string;
  ordinal: number;
}

export interface ActiveThread {
  name: string;
  color: string;
  summary: string;
  recent_beats: BeatBrief[];
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
