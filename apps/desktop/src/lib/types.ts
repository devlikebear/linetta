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

export interface AIOptions {
  tone_preset: boolean;
  short_form: boolean;
}

export interface AIDelta {
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

export type ProviderID = "claude-code-cli" | "openai-codex";

export interface Settings {
  provider: ProviderID;
  typewriter_default: boolean;
  backup_dir: string;
}

export interface SettingsPatch {
  provider?: ProviderID;
  typewriter_default?: boolean;
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
