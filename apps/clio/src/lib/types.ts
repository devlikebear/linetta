// Wire types for Clio's RPC methods. Each mirrors a Go struct 1:1:
//   post.*        engine/internal/clio/post/post.go
//   voice.*       engine/internal/clio/voice/profile.go
//   import/preview engine/internal/clio/handlers/posts.go

export type Platform = "x" | "threads" | "blog";
export type PostKind = "post" | "reply";

/** post.NewPost: a post not yet stored. */
export interface NewPost {
  body: string;
  platform: Platform;
  kind: PostKind;
  source: "import" | "written";
  external_id: string;
  posted_at: number;
}

/** post.Post: a stored post. */
export interface Post extends NewPost {
  id: string;
  created_at: number;
}

/** post.ContextCount */
export interface ContextCount {
  platform: Platform;
  kind: PostKind;
  count: number;
}

/** handlers.clioImportParams (posts.import_preview and posts.import). */
export interface ImportParams {
  file_name: string;
  content: string;
  default_kind: PostKind;
  platform: Platform;
  own_user_id: string;
}

/** posts.import_preview result. `skipped` is keyed by reason: retweet, short, duplicate. */
export interface ImportPreview {
  total: number;
  contexts: ContextCount[];
  skipped: Record<string, number>;
  preview: NewPost[];
}

/** posts.import result. */
export interface ImportResult {
  inserted: number;
  skipped: number;
}

/** posts.list params. Empty platform/kind match any; limit 0 means the default (50), max 500. */
export interface ListPostsParams {
  platform?: Platform | "";
  kind?: PostKind | "";
  limit?: number;
  offset?: number;
}

/** voice.profile params. platform and kind accept "*" for all. */
export interface VoiceProfileParams {
  platform: Platform | "*";
  kind: PostKind | "*";
}

/** voice.Stat */
export interface VoiceStat {
  mean: number;
  p10: number;
  p50: number;
  p90: number;
}

/** voice.ContextProfile. `key` is the context asked for, `fallback` the one used. */
export interface ContextProfile {
  key: string;
  fallback: string;
  n: number;
  features: Record<string, VoiceStat>;
  endings: Record<string, number>;
  top: { ending: string; share: number }[];
  polite_share: number;
  mixed_share: number;
  /** Ids of up to five example posts (resolve bodies with posts.list). */
  examples: string[];
}

/** voice.FeatureNames, in engine order. */
export const FEATURE_NAMES = [
  "length",
  "lines",
  "segment_length",
  "punctuated",
  "polite",
  "emoji",
  "laugh",
  "cry",
  "hashtag",
  "ellipsis",
  "exclaim",
  "ai_tells",
] as const;
export type FeatureName = (typeof FEATURE_NAMES)[number];

/** engine_status Tauri command. */
export interface EngineStatus {
  ok: boolean;
  error: string | null;
}

/** draft.Draft (engine/internal/clio/draft/draft.go). Timestamps are Unix milliseconds. */
export interface Draft {
  id: number;
  body: string;
  platform: Platform;
  kind: PostKind;
  content_version: number;
  published_at: number;
  created_at: number;
  updated_at: number;
}

/** drafts.create params. */
export interface CreateDraftParams {
  platform: Platform;
  kind: PostKind;
  body?: string;
}

/** drafts.update params. A stale content_version fails with RPC code -32009. */
export interface UpdateDraftParams {
  id: number;
  content_version: number;
  body: string;
  platform: Platform;
  kind: PostKind;
}

export type Severity = "strong" | "medium" | "weak";

/**
 * check.Finding. `start`/`end` are rune (code point) offsets into the body,
 * not UTF-16 indices; convert with lib/offsets before touching the DOM.
 * start == end == 0 means a whole-post finding.
 */
export interface Finding {
  rule: string;
  severity: Severity;
  start: number;
  end: number;
  message: string;
  evidence: string;
}

/** check.run params. */
export interface CheckParams {
  body: string;
  platform: Platform;
  kind: PostKind;
}

/** check.run result. `context_used` names the profile context the check ran against. */
export interface CheckResult {
  findings: Finding[];
  context_used: string;
}
