-- The two curated documents every agent reads: one global writer profile and
-- one set of notes per work. They live in the database rather than beside the
-- experiences.jsonl log because the daily backup is VACUUM INTO on this file
-- only -- anything else under LINETTA_HOME is not backed up, and a memory the
-- writer shaped over months has to survive a restore.
--
-- The global row is the one with project_id IS NULL. SQLite exempts NULL from
-- the foreign key, which is why it is not the empty string: '' would have to
-- match a project id.
CREATE TABLE agent_memory (
  scope      TEXT NOT NULL,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  body       TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX idx_agent_memory_global
  ON agent_memory(scope) WHERE project_id IS NULL;

CREATE UNIQUE INDEX idx_agent_memory_project
  ON agent_memory(scope, project_id) WHERE project_id IS NOT NULL;
