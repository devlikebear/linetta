-- What this library last knew about its works on Linetta Works (Phase 3).
-- The site is the source of truth; these rows are the copy the app shows
-- while offline and the last outcome of each publish. Nothing here is the
-- manuscript, and losing it costs one status fetch.
--
-- blurb is the reader-facing introduction the writer typed for the site. It
-- is deliberately not projects.synopsis, which is a summary written for the
-- AI's context and is never published.
CREATE TABLE publish_works (
  project_id     TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  remote_url     TEXT NOT NULL DEFAULT '',
  slug           TEXT NOT NULL DEFAULT '',
  blurb          TEXT NOT NULL DEFAULT '',
  serial_status  TEXT NOT NULL DEFAULT 'ongoing',
  last_synced_at INTEGER NOT NULL DEFAULT 0
);

-- One row per episode the site has. node_id is not a foreign key: an episode
-- whose node the writer deleted stays on the site until they take it down,
-- and the app has to keep showing it as "only on the site".
CREATE TABLE publish_episodes (
  node_id           TEXT PRIMARY KEY,
  project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  remote_url        TEXT NOT NULL DEFAULT '',
  number            INTEGER NOT NULL DEFAULT 0,
  title             TEXT NOT NULL DEFAULT '',
  content_hash      TEXT NOT NULL DEFAULT '',
  remote_status     TEXT NOT NULL DEFAULT '',
  moderation_reason TEXT NOT NULL DEFAULT '',
  published_at      TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_publish_episodes_project ON publish_episodes(project_id, number);
