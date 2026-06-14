-- User-visible companion transcript with Linetta-owned scene/project scope
-- metadata. TARS worker transcripts remain an internal model-replay detail.
CREATE TABLE companion_messages (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id    TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  run_id     TEXT,
  role       TEXT NOT NULL,
  scope      TEXT NOT NULL DEFAULT 'project',
  intent     TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT 'done',
  content    TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE INDEX idx_companion_messages_project_time
  ON companion_messages(project_id, created_at);

CREATE INDEX idx_companion_messages_node_time
  ON companion_messages(project_id, node_id, created_at);

CREATE INDEX idx_companion_messages_run
  ON companion_messages(run_id);

CREATE TABLE companion_history_imports (
  project_id   TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  imported_at  INTEGER NOT NULL
);
