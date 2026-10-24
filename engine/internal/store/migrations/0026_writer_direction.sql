-- The writer's own direction for a work (story-assist Phase 1). Kept out
-- of agent_memory on purpose: work notes are what an agent records, and
-- nothing an agent can edit may hold an instruction the writer gave it.
CREATE TABLE project_focus (
  project_id     TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  body           TEXT NOT NULL,
  anchor_node_id TEXT,              -- episode node the focus was set at
  set_at         INTEGER NOT NULL
);
CREATE TABLE scene_intents (
  node_id    TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  goal       TEXT NOT NULL DEFAULT '',
  keep_json  TEXT NOT NULL DEFAULT '[]',
  avoid_json TEXT NOT NULL DEFAULT '[]',
  updated_at INTEGER NOT NULL
);
CREATE INDEX scene_intents_project ON scene_intents(project_id);
