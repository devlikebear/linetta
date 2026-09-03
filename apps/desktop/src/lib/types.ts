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
  language?: string;
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
  language?: string;
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
export type SnapshotReason = "manual" | "autosave" | "companion-before";

export interface Snapshot {
  id: string;
  node_id: string;
  content_doc: string;
  reason: SnapshotReason;
  created_at: number;
}

export interface SnapshotCompareSide {
  id: string;
  reason: SnapshotReason;
  created_at: number;
  plaintext: string;
}

export interface SnapshotCompareResult {
  left: SnapshotCompareSide;
  right: SnapshotCompareSide;
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

export type CompanionIntentKind =
  | "scene_write"
  | "scene_rewrite"
  | "scene_proofread"
  | "outline_mutation"
  | "generic_mutation"
  | "read_only"
  | "chat";

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

export interface EngineStatus {
  ok: boolean;
  error: string | null;
  version: string | null;
  home: string | null;
  db_path: string | null;
  migration_version: number | null;
  migration_count: number | null;
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
  mcp_available?: boolean;
  /** Whether this library ever held a companion message. Decides whether the
   *  backup section offers the transcript export — a library that never used
   *  the companion has nothing to export. */
  companion_history_exists?: boolean;
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

/** Live state of the local MCP server that external agents connect to. */
export interface McpStatus {
  running: boolean;
  mode: string;
  port?: number;
  project_id?: string;
  token_set: boolean;
}

/** Enabling and regenerating both hand back the bearer token once: it is
 *  minted engine-side and settings.get redacts it forever after, so this is
 *  the only moment the pane can build a copyable client command. */
export interface McpTokenResult {
  token: string;
  status: McpStatus;
}

/** Detection result for one supported MCP client (#69). */
export interface McpClientStatus {
  id: string;
  installed: boolean;
  connected: boolean;
  config_path?: string | null;
  /** The client app is running now. Meaningful for clients that rewrite
   *  their own config while open (Claude Desktop) — connecting then is
   *  clobbered by the app's next internal save. */
  running?: boolean;
}

/** What a one-click connect did. */
export interface McpConnectOutcome {
  ok: boolean;
  /** "connected" | "already" */
  outcome: string;
  config_path?: string | null;
  backup_path?: string | null;
  detail?: string | null;
}

/** One recorded external tool call, shown in the activity list. */
export interface McpActivityEntry {
  id: string;
  at: number;
  tool: string;
  project_id?: string;
  target_id?: string;
  ok: boolean;
  detail?: string;
}

export interface GitSyncResult {
  skipped: boolean;
  files_written: number;
  committed: boolean;
  pushed: boolean;
  message: string;
  error: string;
}

export interface FolderSyncResult {
  skipped: boolean;
  files_copied: number;
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

/** The four providers the built-in agent can use (#90). Codex logs in with
 *  OAuth inside the app; the other three take an API key. "openai" is the
 *  OpenAI-compatible family — base_url points it at OpenRouter, Ollama, LM
 *  Studio. */
export type ProviderID = "openai-codex" | "anthropic" | "gemini-native" | "openai";
export type WebSearchProvider = "brave" | "perplexity";

export type AppLanguage = "ko" | "en" | "ja";

/** One provider's stored entry as settings.get shows it. The key itself never
 *  arrives — api_key_set says whether one is stored. */
export interface ProviderConfig {
  model?: string;
  api_key_set?: boolean;
  base_url?: string;
  /** Per-provider data-sharing consent, epoch ms; absent or 0 means none. */
  consented_at?: number;
}

/** One provider's entry in a settings.set patch. An empty api_key deletes
 *  the stored key; consented_at 0 revokes. */
export interface ProviderPatch {
  model?: string;
  base_url?: string;
  api_key?: string;
  consented_at?: number;
}

/** providers.list — where each provider stands. Never carries a secret. */
export interface ProviderStatus {
  id: ProviderID;
  auth: "oauth" | "api_key";
  active: boolean;
  configured: boolean;
  consented: boolean;
  model?: string;
  base_url?: string;
}

/** codex.login_start — the address the shell opens in the OS browser. */
export interface CodexLoginStart {
  auth_url: string;
}

/** codex.login_status — never carries a token, only who is signed in. */
export interface CodexStatus {
  logged_in: boolean;
  email?: string;
  account_id?: string;
  /** id_token expiry, epoch seconds. */
  expires_at?: number;
  /** True when the most recent attempt ended in a failure — the issuer
   *  refused the exchange, or the credential could not be written — rather
   *  than the writer still being out in the browser. Never true alongside
   *  logged_in. The next login_start clears it. */
  login_failed?: boolean;
}

export type ThemePreference = "system" | "light" | "dark";
/** Which set of colours the UI uses. Orthogonal to ThemePreference,
 *  which picks the light or dark end of whichever set is active.
 *  "hanji" is the default; "paper" is the original burnt-sienna palette. */
export type PalettePreference = "hanji" | "paper" | "bone" | "press";
export type PlatformProfileId = "plain" | "munpia" | "series" | "joara";

export interface Settings {
  language: AppLanguage;
  /** Widened past ProviderID on purpose: settings.get returns whatever the
   *  file on disk says, and a library written by 1.0 still carries a retired
   *  id such as "claude-code-cli" or "openrouter". The engine deliberately
   *  leaves those untouched rather than rewriting someone's settings.json, so
   *  the type must not promise more than the engine guarantees. Use
   *  `providers.list` for the ids this build can actually drive. */
  provider: ProviderID | (string & {});
  providers?: Record<string, ProviderConfig>;
  typewriter_default: boolean;
  focus_default: boolean;
  theme: ThemePreference;
  palette: PalettePreference;
  editor_font_size: number;
  editor_line_height: number;
  copy_profile: PlatformProfileId;
  git_sync_dir: string;
  git_sync_commit_template: string;
  folder_sync_dir: string;
  folder_sync_enabled: boolean;
  backup_dir: string;
  safety_checklist_dismissed: boolean;
  onboarding_tour_enabled: boolean;
  onboarding_tour_seen_version: string;
  ai_data_sharing_consent_version?: number;
  ai_data_sharing_consented_at?: number;
  web_search_provider: WebSearchProvider;
  web_search_api_key: string;
  web_search_api_key_set?: boolean;
  mcp_mode?: string;
  mcp_port?: number;
  mcp_project_id?: string;
  mcp_consent_version?: number;
  mcp_consented_at?: number;
  /** Presence flag only — the token itself never reaches the renderer here. */
  mcp_token_set?: boolean;
}

export interface SettingsPatch {
  language?: AppLanguage;
  provider?: ProviderID;
  providers?: Record<string, ProviderPatch>;
  typewriter_default?: boolean;
  focus_default?: boolean;
  theme?: ThemePreference;
  palette?: PalettePreference;
  editor_font_size?: number;
  editor_line_height?: number;
  copy_profile?: PlatformProfileId;
  git_sync_dir?: string;
  git_sync_commit_template?: string;
  folder_sync_dir?: string;
  folder_sync_enabled?: boolean;
  safety_checklist_dismissed?: boolean;
  onboarding_tour_enabled?: boolean;
  onboarding_tour_seen_version?: string;
  ai_data_sharing_consent_version?: number;
  ai_data_sharing_consented_at?: number;
  web_search_provider?: WebSearchProvider;
  web_search_api_key?: string;
  mcp_mode?: string;
  mcp_port?: number;
  mcp_project_id?: string;
  mcp_consent_version?: number;
  mcp_consented_at?: number;
}

export interface SnapshotEntry {
  id: string;
  reason: SnapshotReason;
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

export type CompanionReferenceSource = "text" | "clipboard" | "markdown" | "file";
export type CompanionReferencePurpose = "style" | "content" | "canon" | "constraint";
export type CompanionReferenceStatus = "active" | "summarized" | "disabled";

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

/** What a pending outline change would do to the tree. */
export interface OutlineChangeCounts {
  created: number;
  renamed: number;
  deleted: number;
  moved: number;
  /** Ops in the same batch that do not touch the tree (beats, world-building). */
  other: number;
}

export interface OutlinePreviewNode {
  ref?: string;
  node_id?: string;
  label?: string;
  title?: string;
  kind?: string;
  depth: number;
  action: "create" | "rename" | "delete" | "move";
}

export interface OutlineChangePreview {
  summary?: string;
  counts: OutlineChangeCounts;
  tree?: OutlinePreviewNode[];
  truncated?: number;
  ops: ProposalOp[];
}

/** Steps a long companion run walks through, shown as progress in the panel. */
export type CompanionPhase =
  | "requesting"
  | "generating"
  | "querying"
  | "searching"
  | "fetching"
  | "verifying"
  | "applying"
  | "applied";

