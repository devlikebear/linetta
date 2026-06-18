// Mirrors engine/internal/project Project struct (JSON tag names).
export type LengthTarget = "flash" | "short" | "novella" | "novel" | "series";
export type DefaultPOV = "first" | "third_limited" | "omniscient";
export type OutlinePreset = "novel" | "webnovel";

export interface Project {
  id: string;
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
  style_notes: string;
  outline: string;
  outline_preset?: OutlinePreset;
  episode_char_target: number;
  synopsis: string;
  word_count: number;
  last_opened_node_id?: string;
  created_at: number;
  updated_at: number;
  archived_at?: number;
}

export interface UpdateProjectInput {
  id: string;
  title?: string;
  outline?: string;
  outline_preset?: OutlinePreset;
  episode_char_target?: number;
  synopsis?: string;
}

export interface NewProjectInput {
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
  outline_preset?: OutlinePreset;
}

export interface WritingStatsToday {
  chars_added: number;
}

export interface WritingStatsDay {
  day: string;
  chars_added: number;
}

export interface WritingStatsSummary {
  today: number;
  week_avg: number;
  total_days: number;
}

export interface ListProjectsParams {
  include_archived?: boolean;
  limit?: number;
}

// Mirrors engine/internal/node Node struct.
export type NodeKind = "container" | "leaf";
export type NodeStatus = "draft" | "revision" | "final" | "published";

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

export interface ManuscriptSearchHit {
  node_id: string;
  breadcrumb: string;
  snippet: string;
  updated_at?: number;
}

export interface ReplaceCandidate {
  id: string;
  node_id: string;
  breadcrumb: string;
  before: string;
  after: string;
  occurrences: number;
  selected: boolean;
  preview_version: number;
}

export interface ReplacePlan {
  id?: string;
  project_id: string;
  query: string;
  replacement: string;
  candidates: ReplaceCandidate[];
  generated_at?: number;
}

export interface ApplyFailure {
  candidate_id: string;
  node_id?: string;
  breadcrumb?: string;
  reason: string;
  message: string;
}

export interface ApplyReplaceResult {
  applied: number;
  skipped: number;
  failures: ApplyFailure[];
  changed_node_ids: string[];
}

export type ContextChangeType = "rename" | "setting";

export interface ContextTarget {
  canonical_name: string;
  aliases?: string[];
  kind: string;
  entity_ids?: string[];
  fact_ids?: string[];
  relationship_ids?: string[];
}

export interface ResolveTargetInput {
  project_id: string;
  entity_id?: string;
  fact_id?: string;
  selected_text?: string;
  query?: string;
}

export interface ContextChangeInput extends ResolveTargetInput {
  type: ContextChangeType;
  old_terms?: string[];
  new_terms?: string[];
  review_only?: boolean;
}

export interface MetadataCandidate {
  id: string;
  kind: string;
  target_id: string;
  label: string;
  before: string;
  after: string;
  selected: boolean;
}

export interface ReviewCandidate {
  id: string;
  kind: string;
  target_id: string;
  label: string;
  snippet: string;
  selected: boolean;
}

export interface ContextChangePlan {
  id: string;
  project_id: string;
  target: ContextTarget;
  type: ContextChangeType;
  old_terms: string[];
  new_terms: string[];
  metadata_candidates: MetadataCandidate[];
  manuscript_plans: ReplacePlan[];
  review_candidates: ReviewCandidate[];
  warnings?: string[];
}

export interface ApplyContextSelection {
  metadata_candidate_ids?: string[];
  manuscript_candidate_ids?: Record<string, string[]>;
}

export interface ApplyContextResult {
  metadata_applied: number;
  manuscript: ApplyReplaceResult;
  failures?: ApplyFailure[];
}

export interface ConsistencyInput {
  project_id: string;
  old_terms: string[];
  new_terms?: string[];
  changed_entity_ids?: string[];
}

export interface ConsistencyIssue {
  severity: string;
  kind: string;
  node_id?: string;
  breadcrumb?: string;
  snippet?: string;
  message: string;
}

export interface ConsistencyReport {
  ok: boolean;
  issues: ConsistencyIssue[];
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
  content_version?: number;
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
  context?: AIContextSelection;
  outline_structure?: string;
}

export type CompanionIntentKind =
  | "scene_write"
  | "scene_rewrite"
  | "scene_proofread"
  | "outline_mutation"
  | "generic_mutation"
  | "chat";

export interface CompanionIntent {
  kind: CompanionIntentKind;
  target_node_id?: string;
  apply_policy?: "direct" | "proposal";
}

export type CompanionHistoryScope = "scene" | "project";

export type AIContextKey =
  | "current_scene"
  | "overview"
  | "synopsis"
  | "nearby_scenes"
  | "related_scenes"
  | "plot"
  | "entities"
  | "relationships"
  | "notes"
  | "project_meta"
  | "style_notes"
  | "facts"
  | "memories"
  | "references";

export type AIContextSelection = Record<AIContextKey, boolean>;

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
  unavailable_providers?: string[];
  git_sync_available?: boolean;
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
  sections?: ContextPreviewSectionResponse[];
  selected_item_count?: number;
  selected_char_count?: number;
  selected_token_estimate?: number;
  budget_token_estimate?: number;
}

export interface ContextPreviewSectionResponse {
  id: AIContextKey;
  label: string;
  present: boolean;
  selected: boolean;
  count: number;
  preview: string;
  char_count?: number;
  token_estimate?: number;
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

export interface AIContextSection {
  id: AIContextKey;
  label: string;
  present: boolean;
  selected: boolean;
  count: number;
  preview: string;
  charCount: number;
  tokenEstimate: number;
}

export interface AIContextPreview {
  counts: ContextCounts;
  sections: AIContextSection[];
  selectedItemCount: number;
  selectedCharCount: number;
  selectedTokenEstimate: number;
  budgetTokenEstimate: number;
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

export type AppLanguage = "ko" | "en" | "ja";

export interface ProviderConfig {
  model?: string;
  api_key?: string;
  api_key_set?: boolean;
  clear_api_key?: boolean;
  base_url?: string;
  cli_path?: string;
}

export interface ProviderTestResult {
  ok: boolean;
  provider: ProviderID;
  model?: string;
  message: string;
}

export interface WebSearchTestResult {
  ok: boolean;
  provider: WebSearchProvider;
  message: string;
}

export type ThemePreference = "system" | "light" | "dark";
export type PlatformProfileId = "plain" | "munpia" | "series" | "joara";

export interface Settings {
  language: AppLanguage;
  provider: ProviderID;
  providers?: Record<string, ProviderConfig>;
  typewriter_default: boolean;
  focus_default: boolean;
  theme: ThemePreference;
  editor_font_size: number;
  editor_line_height: number;
  copy_profile: PlatformProfileId;
  git_sync_dir: string;
  git_sync_commit_template: string;
  backup_dir: string;
  safety_checklist_dismissed: boolean;
  onboarding_tour_enabled: boolean;
  onboarding_tour_seen_version: string;
  web_search_provider: WebSearchProvider;
  web_search_api_key: string;
  web_search_api_key_set?: boolean;
}

export interface SettingsPatch {
  language?: AppLanguage;
  provider?: ProviderID;
  providers?: Record<string, ProviderConfig>;
  typewriter_default?: boolean;
  focus_default?: boolean;
  theme?: ThemePreference;
  editor_font_size?: number;
  editor_line_height?: number;
  copy_profile?: PlatformProfileId;
  git_sync_dir?: string;
  git_sync_commit_template?: string;
  safety_checklist_dismissed?: boolean;
  onboarding_tour_enabled?: boolean;
  onboarding_tour_seen_version?: string;
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

export interface ExportTextPayload {
  text: string;
  char_count: number;
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

export type FactStatus = "verified" | "uncertain" | "intentional_fiction" | "stale";

export interface FactSource {
  id: string;
  card_id: string;
  url: string;
  title: string;
  snippet: string;
  accessed_at: number;
}

export interface FactSourceInput {
  url: string;
  title?: string;
  snippet?: string;
  accessed_at?: number;
}

export interface FactCard {
  id: string;
  project_id: string;
  node_id?: string;
  claim: string;
  result: string;
  status: FactStatus;
  category: string;
  sources: FactSource[];
  created_at: number;
  updated_at: number;
}

export interface NewFactInput {
  project_id: string;
  node_id?: string;
  claim: string;
  result: string;
  status: FactStatus;
  category?: string;
  sources: FactSourceInput[];
}

export interface NewFactFromUrlInput {
  project_id: string;
  node_id?: string;
  claim: string;
  result?: string;
  status?: FactStatus;
  category?: string;
  url: string;
}

export interface UpdateFactInput {
  id: string;
  claim?: string;
  result?: string;
  status?: FactStatus;
  category?: string;
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
  id?: string;
  project_id?: string;
  node_id?: string | null;
  node_label?: string;
  run_id?: string;
  role: string;
  content: string;
  timestamp: number;
  scope?: "scene" | "project" | "global";
  intent?: string;
  status?: string;
}

export interface CompanionImageAttachment {
  name: string;
  media_type: string;
  data: string;
  size?: number;
}

export type CompanionReferenceSource = "text" | "clipboard" | "markdown" | "file";
export type CompanionReferencePurpose = "style" | "content" | "canon" | "constraint";
export type CompanionReferenceStatus = "active" | "summarized" | "disabled";

export interface CompanionReference {
  id: string;
  project_id: string;
  node_id?: string;
  source_type: CompanionReferenceSource;
  purpose: CompanionReferencePurpose;
  title: string;
  content: string;
  summary: string;
  char_count: number;
  token_estimate: number;
  status: CompanionReferenceStatus;
  created_at: number;
  updated_at: number;
}

export interface CompanionReferenceInput {
  project_id: string;
  node_id?: string;
  source_type: CompanionReferenceSource;
  purpose: CompanionReferencePurpose;
  title?: string;
  content: string;
  summary?: string;
  status?: CompanionReferenceStatus;
}

export interface CompanionReferencePatch {
  project_id: string;
  id: string;
  node_id?: string;
  source_type?: CompanionReferenceSource;
  purpose?: CompanionReferencePurpose;
  title?: string;
  content?: string;
  summary?: string;
  status?: CompanionReferenceStatus;
}

export type ProposalOpType =
  | "create_thread" | "update_thread"
  | "add_beat" | "update_beat" | "delete_beat"
  | "set_outline"
  | "set_scene_text"
  | "remember"
  | "create_entity" | "update_entity" | "create_relationship"
  | "create_scene" | "create_outline_node"
  | "create_fact_card";

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
  attributes?: Record<string, string>;
  entity_id?: string;
  from?: string;
  from_ref?: string;
  to?: string;
  to_ref?: string;
  notes?: string;
  after_node_id?: string;
  after_node_ref?: string;
  title?: string;
  node_ref?: string;
  parent_node_id?: string;
  parent_node_ref?: string;
  inverse_label?: string;
  claim?: string;
  result?: string;
  status?: FactStatus;
  sources?: FactSourceInput[];
}

export interface CompanionProposal {
  run_id: string;
  project_id?: string;
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

export interface AppliedNodeChange {
  node_id: string;
  op: string;
  content_version: number;
  char_count: number;
  text_preview?: string;
}

export interface CompanionApplyOpsResult {
  summary?: string;
  applied: number;
  created?: Record<string, string>;
  changed_nodes?: AppliedNodeChange[];
  failures?: CompanionApplyOpsFailure[];
}

interface CompanionRunMeta {
  run_id: string;
  project_id?: string;
  node_id?: string;
  scope?: "scene" | "project" | "global";
  intent?: string;
}

export interface CompanionDelta extends CompanionRunMeta { text: string; }
export interface CompanionReset extends CompanionRunMeta { text: string; }
export interface CompanionDone extends CompanionRunMeta { full_text: string; }
export interface CompanionError extends CompanionRunMeta { message: string; }
export interface CompanionCancelled extends CompanionRunMeta {}
export interface CompanionApplied extends CompanionRunMeta { summary?: string; applied: number; changed_nodes?: AppliedNodeChange[]; }
export interface CompanionThinking extends CompanionRunMeta { text: string; }
export interface CompanionReasoning extends CompanionRunMeta { text: string; }
export interface CompanionChoices {
  run_id: string;
  project_id?: string;
  node_id?: string;
  scope?: "scene" | "project" | "global";
  intent?: string;
  prompt?: string;
  options: string[];
  allow_custom: boolean;
}
