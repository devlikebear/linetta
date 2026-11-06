CREATE TABLE posts (
  id TEXT PRIMARY KEY,
  body TEXT NOT NULL,
  platform TEXT NOT NULL CHECK (platform IN ('x','threads','blog')),
  kind TEXT NOT NULL CHECK (kind IN ('post','reply')),
  source TEXT NOT NULL CHECK (source IN ('import','written')),
  external_id TEXT NOT NULL DEFAULT '',
  posted_at INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX idx_posts_body ON posts(platform, body);
CREATE INDEX idx_posts_context ON posts(platform, kind);
