CREATE TABLE drafts (
  id INTEGER PRIMARY KEY,
  body TEXT NOT NULL DEFAULT '',
  platform TEXT NOT NULL,
  kind TEXT NOT NULL,
  content_version INTEGER NOT NULL DEFAULT 1,
  published_at INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX idx_drafts_updated ON drafts(updated_at);
