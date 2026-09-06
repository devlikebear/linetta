-- Version history for SKILL.md documents. The files under <home>/skills/ are
-- authoritative -- that is what lets a writer point another agent at the same
-- folder -- but the daily backup is VACUUM INTO on this database alone, so a
-- file-only store would be silently unbacked. Every write lands a row here,
-- which is what the backup actually carries.
--
-- node_snapshots is not reused: its node_id is NOT NULL with an FK to nodes,
-- its previews walk Tiptap JSON, and its retention partitions by node. A skill
-- has none of those.
CREATE TABLE skill_snapshots (
  id         TEXT PRIMARY KEY,
  scope      TEXT NOT NULL,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  body       TEXT NOT NULL,
  descript   TEXT NOT NULL DEFAULT '',
  author     TEXT NOT NULL DEFAULT 'writer',
  reason     TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE INDEX idx_skill_snapshots_skill
  ON skill_snapshots(scope, project_id, name, created_at);

-- No retention/thinning here, unlike node_snapshots' snapshot.Thin. Skills
-- change rarely (a writer or agent edits a SKILL.md far less often than prose)
-- and each row is small (an 8000-rune body cap), so there is no accumulation
-- problem to solve yet. A thinning pass for this table is tracked as #99,
-- not forgotten.
