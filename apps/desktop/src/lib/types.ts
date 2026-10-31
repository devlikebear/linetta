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
  style_notes?: string;
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
  /** What the scene is meant to do; writing it does not overwrite this. */
  plan?: string;
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
  | "references"
  | "direction";

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
  /** Whether this build has the built-in writing agent. False on mobile
   *  builds, where internal/agent is compiled out entirely — the panel must
   *  not offer a feature the engine cannot run. */
  agent_available?: boolean;
  /** False on builds that cannot publish to Linetta Works (iOS). */
  publish_available?: boolean;
  /** False on builds without the private vault (iOS). */
  vault_available?: boolean;
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
  /** Who called the tool: "agent" for Linetta's built-in writing agent,
   *  "external" for a client connected over the MCP port. The engine stamps
   *  this from a static field on the tool deps, never from the wire, so an
   *  external client cannot claim to be the agent. */
  source?: string;
  /** Which agent turn the call belonged to. Only the built-in agent sets it. */
  run_id?: string;
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
export type ProviderID = "openai-codex" | "anthropic" | "gemini-native" | "openai" | "claude-code";
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
  /** "cli" is the Claude Code CLI: no key, the writer's own Claude login. */
  auth: "oauth" | "api_key" | "cli";
  active: boolean;
  /** For "cli", whether the binary was found — not whether it is signed in. */
  configured: boolean;
  consented: boolean;
  model?: string;
  base_url?: string;
  /** Where the Claude Code CLI was found; absent when it was not. */
  cli_path?: string;
}

/** The providers that can draw illustrations. Their key or login is the
 *  agent provider's; their consent and model are their own. */
export type ImageProviderID = "openai-codex" | "gemini-native";

/** One image provider's stored entry. Never holds a credential. */
export interface ImageProviderConfig {
  model?: string;
  /** Image consent, epoch ms; absent or 0 means none. Not the agent's. */
  consented_at?: number;
}

export interface ImageProviderPatch {
  model?: string;
  consented_at?: number;
}

/** image_providers.list — where each image provider stands. */
export interface ImageProviderStatus {
  id: ImageProviderID;
  active: boolean;
  /** The agent provider's key or login exists. */
  configured: boolean;
  consented: boolean;
  model?: string;
}

export type IllustrationAspect = "square" | "portrait" | "landscape";
export type IllustrationFormat = "single" | "four_panel";

/** One stored illustration, without its bytes. */
export interface IllustrationMeta {
  id: string;
  project_id: string;
  source_node_id?: string;
  caption: string;
  /** Exactly what was sent to the provider. */
  prompt: string;
  provider: string;
  model: string;
  mime: string;
  width: number;
  height: number;
  byte_size: number;
  created_at: number;
  /** The scenes that show the picture, from illustrations.list. */
  used_in?: Array<{ node_id: string; label: string; title?: string }>;
}

/** What an agent asked to have drawn (illustration Phase 4). Nothing is
 *  drawn until the writer approves it on a card. */
export type IllustrationRequestPurpose = "insert" | "representative" | "gallery";
export type IllustrationRequestStatus = "pending" | "generating" | "done" | "rejected" | "failed" | "cancelled";

export interface IllustrationRequest {
  id: string;
  project_id: string;
  /** The scene (insert) or the scene/episode (representative). */
  target_node_id?: string;
  purpose: IllustrationRequestPurpose;
  prompt: string;
  character_ids: string[];
  include_references: boolean;
  /** Earlier gallery pictures sent along, as the agent chose them or as the
   *  engine suggested when the request was left. Absent on older rows. */
  prior_illustration_ids?: string[];
  aspect: IllustrationAspect;
  caption: string;
  /** The agent's one line to the writer. */
  note: string;
  source: "agent" | "external";
  status: IllustrationRequestStatus;
  illustration_id?: string;
  /** Set once the drawing went where the purpose said. */
  placed_at?: number;
  error?: string;
  created_at: number;
  decided_at?: number;
}

/** illustrations.representatives: node id → the picture that stands for it. */
export type IllustrationRepresentatives = Record<string, string>;

/** What the writer changed on the card before drawing. */
export interface IllustrationRequestEdits {
  prompt?: string;
  aspect?: IllustrationAspect;
  caption?: string;
  include_references?: boolean;
  /** Earlier gallery pictures sent along (illustrations.suggest_prior). */
  prior_illustration_ids?: string[];
}

/** illustrations-changed's payload: a request was left, decided, drawn or
 *  failed. node_ids are scenes a drawing was put into. */
export interface IllustrationsChangedPayload {
  project_id: string;
  request_id: string;
  status: IllustrationRequestStatus;
  node_ids?: string[];
}

/** illustrations.compose — the prompt for the writer to read before drawing. */
export interface IllustrationComposition {
  prompt: string;
  negative_prompt: string;
  format: IllustrationFormat;
  characters: Array<{ entity_id: string; name: string }>;
  reference_image_ids: string[];
  missing_sheets: string[];
}

export interface IllustrationGenerateInput {
  project_id: string;
  source_node_id?: string;
  prompt: string;
  character_ids?: string[];
  include_references?: boolean;
  /** Earlier gallery pictures sent after the characters' references, so a
   *  face drawn once is drawn the same way again. At most two. */
  prior_illustration_ids?: string[];
  aspect?: IllustrationAspect;
  caption?: string;
}

/** publish.connect_status — Linetta Works connection. Never carries the
 *  token or the pairing secret. */
export type PublishConnectState = "disconnected" | "waiting" | "connected" | "failed";

/** Why the last connection attempt did not end connected. "revoked" is the
 *  site refusing a stored token: the writer removed this device on the web. */
export type PublishFailureReason =
  | "denied"
  | "expired"
  | "network"
  | "claimed"
  | "store"
  | "failed"
  | "revoked";

export interface PublishConnectStatus {
  state: PublishConnectState;
  /** Shown while waiting; the browser shows the same code. */
  confirm_code?: string;
  verify_url?: string;
  /** Epoch ms the pairing expires at. */
  expires_at?: number;
  pen_name?: string;
  handle?: string;
  site_url: string;
  failure_reason?: PublishFailureReason;
}

/** An episode's publishing state, the site's status combined with whether the
 *  local text still matches what was sent. */
export type PublishEpisodeState =
  | "unpublished"
  | "public"
  | "modified"
  | "pending"
  | "rejected"
  | "needs_review"
  | "private"
  | "site_only";

export type PublishSerialStatus = "ongoing" | "completed";

export interface PublishEpisodeView {
  node_id: string;
  label: string;
  title: string;
  ordinal?: number;
  /** The scenes the episode is made of. */
  leaf_ids?: string[];
  char_count: number;
  empty?: boolean;
  /** Pictures that go up with the episode. */
  illustrations?: number;
  state: PublishEpisodeState;
  /** The local text differs from the site's, whatever the state. */
  modified?: boolean;
  number?: number;
  url?: string;
  remote_status?: string;
  moderation_reason?: string;
  published_at?: string;
}

export interface PublishWorkView {
  url: string;
  slug: string;
  blurb: string;
  serial_status: PublishSerialStatus;
  visibility?: string;
}

/** publish.work_status */
export interface PublishWorkStatus {
  project_id: string;
  connected: boolean;
  registered: boolean;
  /** The site could not be reached; the remote half is the saved copy. */
  offline?: boolean;
  last_synced_at?: number;
  work?: PublishWorkView;
  episodes: PublishEpisodeView[];
  site_only: PublishEpisodeView[];
  sheets: PublishSheetsStatus;
}

/** A sheet or the cover as the site has it. The state uses the episode
 *  states that apply: public, pending, rejected, needs_review. */
export interface PublishSheetView {
  entity_id: string;
  kind: EntityKind;
  name: string;
  state: PublishEpisodeState;
  moderation_reason?: string;
}

export interface PublishCoverView {
  illustration_id: string;
  state: PublishEpisodeState;
  moderation_reason?: string;
}

/** Sheets and the cover (Works Phase 8): the writer's choices always, the
 *  site's half only when remote is true. */
export interface PublishSheetsStatus {
  chosen: number;
  cover_id?: string;
  remote: boolean;
  sheets: PublishSheetView[];
  cover?: PublishCoverView;
}

/** publish.episode_publish */
export interface PublishResult {
  created: boolean;
  episode: PublishEpisodeView;
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

/** agent.run — the turn id; progress arrives as agent.* notifications (#93). */
export interface AgentRunResult {
  run_id: string;
}

/** A picture attached to an agent message. The engine checks the bytes and
 *  takes at most four, 5 MB each. */
export interface AgentImage {
  mime: string;
  data_base64: string;
}

/** agent.status — the turn running for a work, if any. What a panel that
 *  mounts mid-turn rejoins it from: `seq` is the last agent.* event the turn
 *  had emitted, `text` the reply streamed so far that is not yet a transcript
 *  row, `tool` the tool it is inside. Mirrors agent.RunStatus (runs.go). */
export interface AgentRunStatus {
  running: boolean;
  run_id?: string;
  started_at?: number;
  seq: number;
  text?: string;
  tool?: string;
}

/** agent.history — one row of the built-in agent's transcript (#93). Shape
 *  is the panel's projection of companion.HistoryMessage, not the engine's
 *  internal row (see agentHistoryRow in engine/internal/engineapp). */
export interface AgentHistoryRow {
  id: string;
  project_id: string;
  node_id?: string;
  node_label?: string;
  run_id?: string;
  role: string;
  status: string;
  content: string;
  created_at: number;
  /** The first message the model still gets. Set only when earlier messages
   *  no longer go to it. */
  replay_start?: boolean;
}

export type ThemePreference = "system" | "light" | "dark";
/** Which set of colours the UI uses. Orthogonal to ThemePreference,
 *  which picks the light or dark end of whichever set is active.
 *  "hanji" is the default; "paper" is the original burnt-sienna palette. */
export type PalettePreference = "hanji" | "paper" | "bone" | "press";
export type PlatformProfileId = "plain" | "munpia" | "series" | "joara";

/** One switchable tool group's share of the budget: how many tools it adds
 *  and how many bytes of tool schema they cost in every request. Reported
 *  whether or not the group is currently on, because the writer deciding
 *  about a switch needs the price of the thing they would be giving up. */
export interface ToolBudgetGroup {
  tools: number;
  bytes: number;
}

/** What the built-in agent's tool set costs right now (#99). `tools` and
 *  `bytes` are the totals with the writer's switches as they stand; `memory`
 *  and `skills` are what each group costs when it is on. Every number is
 *  measured engine-side from the tools it would actually register. */
export interface ToolBudget {
  visual_writes?: ToolBudgetGroup;
  tools: number;
  bytes: number;
  memory: ToolBudgetGroup;
  skills: ToolBudgetGroup;
  /** story-assist Phase 2: linetta_report_continuity's group. */
  review: ToolBudgetGroup;
}

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
  /** Absent or "" while illustration drawing is off. */
  image_provider?: ImageProviderID | "";
  image_providers?: Record<string, ImageProviderConfig>;
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
  /** The agent's self-improvement pass after a working turn (#98). Optional
   *  because a settings payload from an older engine simply omits it; every
   *  reader must treat "missing" as the default, which is ON. */
  agent_self_review_enabled?: boolean;
  /** Off unless the writer turns it on: it sends finished scenes to the model again. */
  agent_episode_review_enabled?: boolean;
  /** Phrases to keep out of every work. */
  style_avoid_phrases?: string[];
  /** How many tool calls one turn of the built-in agent may execute before
   *  it is stopped and asked to say how far it got. Missing (an older
   *  engine) means the default, 48. */
  agent_max_tool_calls?: number;
  /** The writer's tool budget (#99): whether the memory tool and the two
   *  skills tools are offered to any agent at all. Both optional and both
   *  default ON, so — like agent_self_review_enabled above — every reader
   *  must treat "missing" as on and test `!== false`, never truthiness. */
  visual_write_tools_disabled?: boolean;
  memory_tools_enabled?: boolean;
  skill_tools_enabled?: boolean;
  /** story-assist Phase 2: linetta_report_continuity and later phases'
   *  additions to it. Optional, default ON, same read rule as the two above. */
  review_tools_enabled?: boolean;
  /** What that tool set costs, measured by the engine from the tools it would
   *  actually register — never a table maintained here, which is how the site's
   *  tool counts drifted. Absent on a build that cannot measure it (mobile),
   *  so the pane draws the switches without the numbers rather than inventing
   *  any. */
  tool_budget?: ToolBudget;
  /** Provider ids whose pre-1.0 plaintext `api_key` is still sitting in
   *  settings.json because the engine had nowhere to move it (#113). Derived
   *  at load time and never persisted, so it is absent on every healthy
   *  install and on any engine older than the fix. */
  legacy_plaintext_providers?: string[];
  /** The same, for the web-search key. */
  legacy_plaintext_web_search?: boolean;
  /** Why the move failed: "unsupported" (this platform has no secret store at
   *  all — Linux today) or "error" (it has one and it refused, e.g. a locked
   *  Keychain). The two need different sentences. */
  legacy_plaintext_reason?: "unsupported" | "error" | (string & {});
  /** The settings.json holding them, so the notice can name the file. */
  legacy_plaintext_path?: string;
}

export interface SettingsPatch {
  language?: AppLanguage;
  provider?: ProviderID;
  providers?: Record<string, ProviderPatch>;
  /** "" turns illustration drawing off. */
  image_provider?: ImageProviderID | "";
  image_providers?: Record<string, ImageProviderPatch>;
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
  agent_self_review_enabled?: boolean;
  /** Off unless the writer turns it on: it sends finished scenes to the model again. */
  agent_episode_review_enabled?: boolean;
  /** Phrases to keep out of every work. */
  style_avoid_phrases?: string[];
  agent_max_tool_calls?: number;
  visual_write_tools_disabled?: boolean;
  memory_tools_enabled?: boolean;
  skill_tools_enabled?: boolean;
  review_tools_enabled?: boolean;
  /** Deletes the plaintext keys the engine could not migrate out of
   *  settings.json (#113). Destructive and one-way — where there is no secret
   *  store the file is the only place those keys exist — so it is only ever
   *  sent from a button the writer pressed. */
  clear_legacy_plaintext_keys?: boolean;
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
  /** create_entity sets aliases; update_entity only adds to the existing
   *  list and never removes one (story-assist Phase 3, 3.A.2). */
  aliases?: string[];
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

/** One curated-memory document: the global writer profile, or one work's notes. */
export interface MemoryDocument {
  scope: "writer_profile" | "work_notes";
  project_id?: string;
  body: string;
  chars_used: number;
  chars_budget: number;
  updated_at: number;
}

/** memory.get's shape: both documents, since Settings needs both at once. */
export interface MemoryState {
  writer_profile: MemoryDocument;
  work_notes: MemoryDocument;
}

/** memory-changed's payload. "agent" and "external" come from mcphost's
 *  linetta_edit_memory tool (mcphost/tools_write.go); "writer" is a save made
 *  by the person at the keyboard, from memory.set. */
export interface MemoryChangedPayload {
  scope: MemoryDocument["scope"];
  project_id?: string;
  source: "agent" | "external" | "writer";
}

/** Where a skill lives: global to the writer, or scoped to one work. */
export type SkillScope = "writer" | "work";

/** Who wrote a skill last — the writer editing by hand, or the agent that
 *  authored it during a session. */
export type SkillAuthor = "writer" | "agent";

/** One row of the Settings skill list: the whole SKILL.md except its body.
 *  skills.read fetches the body of the one the writer opens. */
export interface SkillSummary {
  name: string;
  scope: SkillScope;
  project_id?: string;
  description: string;
  author: SkillAuthor;
  enabled: boolean;
  updated_at: number;
  body_runes: number;
  body_budget: number;
}

/** One full SKILL.md, as skills.read returns it.
 *
 *  parse_error is set only when the file on disk is not a readable skill —
 *  frontmatter the writer broke by hand, which is the same file skills.list
 *  reports as a diagnostic. Then `body` is the file VERBATIM (every byte,
 *  broken frontmatter included), `description` is empty and `name` is the
 *  folder's, because the file's own could not be read. The pane must show
 *  the message: a writer handed that body with no explanation would save it
 *  straight back and wonder why the skill still does not appear. */
export interface Skill extends SkillSummary {
  body: string;
  parse_error?: string;
}

/** skills.write and skills.restore return the stored skill plus one field the
 *  document itself does not carry. versioned=false means the file changed but
 *  the version row did not land: that one edit cannot be reverted, and the
 *  writer has to be told rather than finding out when they try. */
export interface SkillWriteResult extends Skill {
  versioned: boolean;
}

/** skills.delete's shape: the same flag, and for a sharper reason — a
 *  deletion whose version row did not land took the body with it. */
export interface SkillDeleteResult {
  versioned: boolean;
}

/** Something on disk that should have been a skill and is not — a SKILL.md
 *  the writer broke by hand. It is never listed as a skill (it must not reach
 *  a prompt) and always reported here, because a broken skill has to be
 *  visible before it can be fixed. */
export interface SkillDiagnostic {
  path: string;
  message: string;
}

/** skills.list's shape: both scopes at once, plus what could not be read. */
export interface SkillsListResult {
  skills: SkillSummary[];
  diagnostics: SkillDiagnostic[];
}

/** One version of a skill, newest first out of skills.history. The body
 *  travels with each row because skills.restore takes only an id — this is
 *  the only place the writer can see what they are about to revert to. */
export interface SkillVersion {
  id: string;
  name: string;
  scope: SkillScope;
  project_id?: string;
  description: string;
  author: SkillAuthor;
  body: string;
  body_runes: number;
  reason: "created" | "edited" | "deleted";
  created_at: number;
}

/** One name skills.orphaned found in the version log with no file behind it
 *  (#119): the daily backup carries skill_snapshots but never the
 *  `<LINETTA_HOME>/skills` directory, so a skill lost from disk — by hand, or
 *  by restoring that backup — drops out of skills.list even though its
 *  history still has it. No body: the writer opens the same history panel
 *  skills.history already feeds to see versions and restore one. */
export interface OrphanedSkill {
  name: string;
  scope: SkillScope;
  project_id?: string;
  description: string;
  latest_at: number;
}

/** skills.orphaned's shape: mirrors skills.list's "writer scope always, work
 *  scope only with a project id" rule, answered from skill_snapshots instead
 *  of the filesystem. */
export interface OrphanedSkillsResult {
  skills: OrphanedSkill[];
}

/** skills-changed's payload. "agent" and "external" come from mcphost's
 *  linetta_edit_skill tool (mcphost/tools_write.go); "writer" is a save made
 *  by the person at the keyboard, from skills.write, skills.delete or
 *  skills.restore. */
export interface SkillChangedPayload {
  scope: SkillScope;
  project_id?: string;
  name: string;
  source: "agent" | "external" | "writer";
}

export interface CharacterVisualSheet {
 entity_id: string;
 age_range: string;
 build: string;
 hair: string;
 outfit: string;
 signature: string;
 palette: string;
 notes: string;
 updated_at: number;
}
export interface ReferenceImageMeta {
 id: string;
 entity_id: string;
 mime: string;
 caption: string;
 byte_size: number;
 ordinal: number;
 created_at: number;
}
/** One work's checkable style rules (#162). */
export interface StyleRules {
  project_id: string;
  avoid_phrases: string[];
  /** In characters; 0 means no limit. */
  max_sentence_chars: number;
  /** The scene the writer set as the prose to match, or "". */
  anchor_node_id: string;
  updated_at: number;
}
export type LedgerKind = "promise" | "gag" | "question" | "decision" | "number" | "timeline" | "knowledge" | "cut_plan";
export type LedgerStatus = "open" | "paid" | "dropped";
export interface LedgerEntry {
  id: string;
  project_id: string;
  kind: LedgerKind;
  subject: string;
  body: string;
  status: LedgerStatus;
  planted_node_id?: string;
  planted_quote?: string;
  due_node_id?: string;
  paid_node_id?: string;
  paid_quote?: string;
  source: "writer" | "agent" | "external";
  created_at: number;
  updated_at: number;
}
export interface PartArc {
  node_id: string;
  label: string;
  role: string;
  goal: string;
  /** 0 is unset; otherwise 1 to 10. */
  tension: number;
}
export interface LedgerState {
  entries: LedgerEntry[];
  parts: PartArc[];
  /** Scene and part labels, by node id. */
  labels: Record<string, string>;
}
export interface StyleViolation {
  node_id: string;
  label: string;
  rule: "avoid_phrase" | "sentence_too_long" | "parallel_contrast" | "narrator_explains" | "repeated_line" | "ending_formula";
  /** 1-based, counting the scene's non-empty paragraphs. */
  paragraph: number;
  phrase?: string;
  excerpt: string;
  length?: number;
  limit?: number;
}
/** style.check: the rules the check ran against, and what it found. */
export interface StyleCheckResult {
  rules: StyleRules;
  scenes_checked: number;
  /** Every violation found; `violations` may hold fewer when `truncated`. */
  total: number;
  violations: StyleViolation[];
  truncated: boolean;
}
/** style.get_draft: a style profile an agent proposed, waiting for the writer (#163). */
export interface StyleDraftState {
  pending: boolean;
  draft: { project_id: string; body: string; author: string; created_at: number };
  /** The size an approved profile may be, in characters. */
  limit: number;
}
/** Whether an approved draft replaces the style notes or is added after them. */
export type StyleApproveMode = "replace" | "append";

/**
 * The writer's current focus for a work (story-assist Phase 1), and what
 * direction.get_focus can tell about it right now. Present is false when no
 * focus is set — a valid state, not an error.
 */
export interface FocusState {
  project_id: string;
  body: string;
  anchor_node_id?: string;
  set_at: number;
  present: boolean;
  /** -1 when the anchor episode was recorded but no longer exists. */
  episodes_since: number;
  stale: boolean;
}

/** One scene's intent (story-assist Phase 1): its goal, and what to keep or avoid. */
export interface SceneIntent {
  node_id: string;
  project_id: string;
  goal: string;
  keep: string[];
  avoid: string[];
  updated_at: number;
}

/** direction.get_scene_intent's reply. */
export interface SceneIntentState {
  present: boolean;
  intent: SceneIntent;
}

/** story-assist Phase 2: categories, kinds, severities and statuses an observation carries. */
export type ContinuityCategory =
  | "character"
  | "place"
  | "item"
  | "relationship"
  | "timeline"
  | "fact"
  | "plot"
  | "direction"
  | "other";
export type ContinuityKind = "contradiction" | "gap" | "question";
export type ContinuitySeverity = "high" | "medium" | "low";
export type ContinuityStatus = "open" | "dismissed" | "resolved" | "replaced";
export type ContinuityRefType = "entity" | "fact" | "beat" | "scene";

/** One ref an observation cites, with the display name continuity.list resolved. */
export interface ContinuityRef {
  type: ContinuityRefType;
  id: string;
  paragraph?: number;
  quote?: string;
  /** Empty when the ref no longer resolves (its target was deleted since). */
  label?: string;
}

/** One stored continuity observation. */
export interface ContinuityObservation {
  id: string;
  check_id: string;
  project_id: string;
  node_id: string;
  category: ContinuityCategory;
  kind: ContinuityKind;
  severity: ContinuitySeverity;
  summary: string;
  suggestion: string;
  paragraph_from: number;
  paragraph_to: number;
  quote: string;
  refs: ContinuityRef[];
  status: ContinuityStatus;
  status_at: number;
  created_at: number;
}

/** One continuity check (a scene's reading) plus its observations, from continuity.list. */
export interface ContinuityCheck {
  id: string;
  project_id: string;
  node_id: string;
  /** The checked scene's outline path, e.g. "1부 / 1장 / 씬 3". */
  breadcrumb: string;
  content_version: number;
  source: "agent" | "external";
  note: string;
  checked_at: number;
  /** True once the scene's current content_version no longer matches this check's. */
  stale: boolean;
  observations: ContinuityObservation[];
}

/** continuity.open_counts's reply: node_id -> open observation count. */
export interface ContinuityOpenCounts {
  counts: Record<string, number>;
}

/** continuity-changed's payload: an observation's status changed from Story
 *  World (continuity.set_status). ContextPanel's open-observation badge
 *  listens for this rather than relying on an unrelated remount to notice. */
export interface ContinuityChangedPayload {
  project_id: string;
  node_id?: string;
}

// ---------- Suggestions inbox (story-assist Phase 3) ----------
//
// An agent's linetta_propose_story_ops candidates, and the writer's
// item-by-item accept/reject of them. Op reuses ProposalOp (the same
// storyops.Op mirror linetta_apply_story_ops' own pending-approval preview
// uses) rather than a second copy of the same 30-odd fields.

export type SuggestionPurpose = "general" | "import_extraction" | "forecast";
export type SuggestionSource = "agent" | "external";
/** "reverting" means proposals.Undo has committed to revert this item but
 *  has not yet confirmed the outcome — the engine's own StatusReverting.
 *  ProposalInbox shows it as "되돌리는 중 · 확인 필요" and disables its
 *  accept checkbox; proposals.accept refuses it outright either way. */
export type SuggestionItemStatus = "pending" | "accepted" | "rejected" | "failed" | "reverting";

/** One quote an agent cited, with the scene's breadcrumb proposals.list resolved. */
export interface SuggestionEvidence {
  node_id: string;
  paragraph: number;
  quote: string;
  breadcrumb?: string;
}

/** A possible match against an existing entity (entity_id set) or another
 *  pending item's create_entity (item_id set) — informational only; the
 *  engine never merges on its own. */
export interface SuggestionDuplicate {
  entity_id?: string;
  item_id?: string;
  name: string;
  kind: string;
  matched: "exact" | "exact_kind_mismatch";
}

export interface SuggestionItemResult {
  created_id?: string;
  /** created_id was already in the work: the item's name and kind matched it. */
  linked_existing?: boolean;
  error?: string;
  /** Set on a failure the inbox words itself. */
  code?: "dependency_missing" | "name_taken";
}

/** One stored proposal item, from proposals.list. */
export interface SuggestionItem {
  id: string;
  proposal_id: string;
  ordinal: number;
  op_type: ProposalOpType;
  op: ProposalOp;
  /** Human-readable headline, e.g. "새 인물: 하린 — 검술 사범". */
  summary: string;
  depends_on: string[];
  /** True when a dependency's item was rejected — "의존 항목이 거절됨". */
  blocked_by_rejected: boolean;
  evidence: SuggestionEvidence[];
  from_node_ids: string[];
  duplicates: SuggestionDuplicate[];
  status: SuggestionItemStatus;
  result: SuggestionItemResult;
  status_at: number;
  /** Set once this item is accepted as part of a purpose=forecast
   *  structural batch (or an add_beat that depended on one) — pass it to
   *  proposals.undo to revert the whole batch together. */
  undo_batch_id?: string;
}

/** One submitted batch plus its items, from proposals.list. */
export interface SuggestionBatch {
  id: string;
  project_id: string;
  purpose: SuggestionPurpose;
  /** Set when purpose is "forecast": the forecast this batch reflects
   *  into the outline. */
  forecast_id?: string;
  source: SuggestionSource;
  summary: string;
  created_at: number;
  items: SuggestionItem[];
}

/** proposals.accept's reply: each requested item's outcome. */
export interface AcceptSuggestionsResult {
  items: SuggestionItem[];
  /** Every storyops undo batch this call created — usually at most one.
   *  Show a "되돌리기" button keyed to the first entry when non-empty. */
  undo_batch_ids?: string[];
}

/** proposals.extraction_progress's reply: how far a purpose=import_extraction
 *  sweep has gotten, and which episode to continue from. */
export interface ExtractionProgress {
  total_episodes: number;
  covered_episodes: number;
  next_episode?: {
    node_id: string;
    label: string;
    title: string;
    ordinal: number;
    whole?: boolean;
  };
  pending_items: number;
}

/** proposals.extraction_estimate's reply: the whole manuscript's size and a
 *  rough input-token budget, shown before a sweep starts. An estimate, not
 *  a billed cost. */
export interface ExtractionEstimate {
  episodes: number;
  chars: number;
  input_tokens: number;
}

/** proposals-changed's payload: the inbox itself changed — an agent
 *  submitted a batch, or the writer accepted/rejected an item. Separate
 *  from mcp-changed, which means an MCP client wrote the manuscript. */
export interface ProposalsChangedPayload {
  project_id: string;
  kinds: string[];
}

// ---------------------------------------------------------------------------
// Forecasts (story-assist Phase 4): possible continuations an agent drafted
// for the writer to compare. Never canon — see forecast.list's own doc
// comment. Named Forecast* throughout, distinct from the Suggestion* names
// the inbox uses (Phase 3's own convention).

export type ForecastSource = "agent" | "external";
export type ForecastRiskKind = "continuity" | "causality" | "character" | "direction" | "pacing" | "other";

export interface ForecastBranchBeat {
  episode_offset: number;
  summary: string;
}

/** One character's choice at the branch point, with the entity's current
 *  name resolved by forecast.list — empty if the entity was since deleted. */
export interface ForecastDecision {
  entity_id: string;
  decision: string;
  entity_name?: string;
}

export interface ForecastProjectedChanges {
  characters?: string[];
  relationships?: string[];
  world?: string[];
  threads?: string[];
}

/** One way a branch could go wrong, with whichever ref it names resolved
 *  to display text by forecast.list. */
export interface ForecastRisk {
  kind: ForecastRiskKind;
  description: string;
  entity_id?: string;
  fact_id?: string;
  node_id?: string;
  entity_name?: string;
  fact_claim?: string;
  node_breadcrumb?: string;
}

/** One stored continuation. */
export interface ForecastBranch {
  id: string;
  title: string;
  premise: string;
  beats: ForecastBranchBeat[];
  decisions?: ForecastDecision[];
  changes?: ForecastProjectedChanges;
  risks?: ForecastRisk[];
  unknowns?: string[];
  direction_fit?: string;
}

/** What changed since a forecast's input fingerprint was taken —
 *  "characters" with count 2, or "episodes" with the changed episodes'
 *  own labels. */
export interface ForecastStaleReason {
  group: string;
  count?: number;
  episode_labels?: string[];
}

/** One row of forecast.list: a saved comparison plus its staleness,
 *  recomputed fresh on every call — never a cached flag. */
export interface Forecast {
  id: string;
  project_id: string;
  divergence: string;
  base_node_id?: string;
  horizon: number;
  branches: ForecastBranch[];
  source: ForecastSource;
  selected_branch?: string;
  selected_at?: number;
  created_at: number;
  stale: boolean;
  reasons?: ForecastStaleReason[];
  /** The episode these branches were drawn from, e.g. "3화". */
  base_episode_label?: string;
}

/** forecast.select's reply: confirmation plus the staleness warning — a
 *  stale forecast may still be selected (the engine does not refuse it). */
export interface SelectForecastResult {
  ok: boolean;
  stale: boolean;
  reasons?: ForecastStaleReason[];
}
export interface ArtStyle {
 project_id: string;
 style: string;
 negative_prompt: string;
 updated_at: number;
}

/** The private vault's view of one work (engine/internal/vault). */
export type VaultWorkState = "not_uploaded" | "synced" | "local_changes" | "remote_changes" | "conflict";

export type VaultAccountMode = "full" | "terms_outdated" | "read_only" | "none";

export interface VaultAccount {
  mode: VaultAccountMode;
  status: string;
  retrieve_until: string;
  terms_accepted: boolean;
  usage: { bytes: number; quota_bytes: number };
  limits: { blob_bytes: number; restore_points: number; items: number; blobs_per_revision: number };
}

/** Title and project_id stay empty until this device holds a key that opens the item. */
export interface VaultItemView {
  item_id: string;
  project_id: string;
  title: string;
  head_revision: number;
  base_revision: number;
  updated: string;
  bytes: number;
  on_this_device: boolean;
}

export interface VaultOverview {
  connected: boolean;
  key_stored: boolean;
  key_mismatch: boolean;
  account: VaultAccount | null;
  items: VaultItemView[];
}

export interface VaultRestorePoint {
  revision: number;
  created: string;
  bytes: number;
}

export interface VaultWorkStatus {
  project_id: string;
  state: VaultWorkState;
  item_id?: string;
  base_revision: number;
  head_revision: number;
  last_synced_at: number;
  restore_points: VaultRestorePoint[];
}

/** recovery_key is present only on the call that created it, which has not stored it. */
export interface VaultSetupResult {
  key_stored: boolean;
  recovery_key?: string;
}

export interface VaultPushResult {
  item_id: string;
  head_revision: number;
  unchanged: boolean;
}

export interface VaultPullResult {
  project_id: string;
  item_id: string;
  revision: number;
  head_revision: number;
  snapshot_path?: string;
  copy_project_id?: string;
  files_restored: boolean;
}

/** One step of a push or a pull, in blobs (the vault-progress event). */
export interface VaultProgress {
  project_id: string;
  item_id: string;
  phase: "upload" | "download" | "apply";
  done: number;
  total: number;
}

export interface VaultResolveResult {
  push?: VaultPushResult;
  pull?: VaultPullResult;
}
