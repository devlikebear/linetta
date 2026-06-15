-- User-managed reference material that the companion can inject as explicit
-- prompt context. The original content stays in SQLite; prompt injection may
-- use a summary when status='summarized'.
CREATE TABLE companion_references (
  id             TEXT PRIMARY KEY,
  project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id        TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  source_type    TEXT NOT NULL,
  purpose        TEXT NOT NULL,
  title          TEXT NOT NULL,
  content        TEXT NOT NULL,
  summary        TEXT NOT NULL DEFAULT '',
  char_count     INTEGER NOT NULL DEFAULT 0,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  status         TEXT NOT NULL DEFAULT 'active',
  created_at     INTEGER NOT NULL,
  updated_at     INTEGER NOT NULL
);

CREATE INDEX idx_companion_references_project
  ON companion_references(project_id, updated_at);

CREATE INDEX idx_companion_references_node
  ON companion_references(project_id, node_id, updated_at);
