-- 작품
CREATE TABLE projects (
  id            TEXT PRIMARY KEY,
  title         TEXT NOT NULL,
  genres        TEXT NOT NULL,
  length_target TEXT NOT NULL,
  default_pov   TEXT NOT NULL,
  style_notes   TEXT NOT NULL DEFAULT '',
  word_count    INTEGER NOT NULL DEFAULT 0,
  last_opened_node_id TEXT,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  archived_at   INTEGER
);

-- 재귀 Node 트리 (자유 깊이)
CREATE TABLE nodes (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  parent_id   TEXT REFERENCES nodes(id) ON DELETE CASCADE,
  ordinal     INTEGER NOT NULL,
  kind        TEXT NOT NULL,
  label       TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  content_doc TEXT,
  status      TEXT NOT NULL DEFAULT 'draft',
  word_count  INTEGER NOT NULL DEFAULT 0,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
CREATE INDEX idx_nodes_project ON nodes(project_id, parent_id, ordinal);

-- Entity (인물·장소·물건·개념)
CREATE TABLE entities (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL,
  name        TEXT NOT NULL,
  aliases     TEXT NOT NULL DEFAULT '[]',
  role        TEXT NOT NULL DEFAULT '',
  summary     TEXT NOT NULL DEFAULT '',
  attributes  TEXT NOT NULL DEFAULT '{}',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  UNIQUE(project_id, name)
);

-- Mention: Node ↔ Entity
CREATE TABLE mentions (
  id         TEXT PRIMARY KEY,
  node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  entity_id  TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  surface    TEXT NOT NULL
);
CREATE INDEX idx_mentions_node ON mentions(node_id);
CREATE INDEX idx_mentions_entity ON mentions(entity_id);

-- Relationship: Entity ↔ Entity
CREATE TABLE relationships (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  from_id    TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  to_id      TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  label      TEXT NOT NULL,
  notes      TEXT NOT NULL DEFAULT ''
);

-- Thread (스토리라인)
CREATE TABLE threads (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  color      TEXT NOT NULL DEFAULT '#666',
  summary    TEXT NOT NULL DEFAULT '',
  closed_at  INTEGER
);

-- Beat: Thread의 마디
CREATE TABLE beats (
  id         TEXT PRIMARY KEY,
  thread_id  TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  node_id    TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  ordinal    INTEGER NOT NULL,
  label      TEXT NOT NULL DEFAULT '',
  intensity  INTEGER NOT NULL DEFAULT 1
);

-- Note: 마진 노트
CREATE TABLE notes (
  id         TEXT PRIMARY KEY,
  node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  anchor     INTEGER NOT NULL,
  body       TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

-- Version snapshot
CREATE TABLE node_snapshots (
  id          TEXT PRIMARY KEY,
  node_id     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  content_doc TEXT NOT NULL,
  reason      TEXT NOT NULL,
  created_at  INTEGER NOT NULL
);
CREATE INDEX idx_snapshots_node ON node_snapshots(node_id, created_at);

-- AI 호출 이력
CREATE TABLE ai_runs (
  id           TEXT PRIMARY KEY,
  project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id      TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  provider     TEXT NOT NULL,
  prompt       TEXT NOT NULL,
  context_json TEXT NOT NULL,
  output       TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL,
  error        TEXT,
  started_at   INTEGER NOT NULL,
  ended_at     INTEGER
);
