-- How each scene's last settlement pass went (longform Phase 6). One row per
-- scene: the version of the prose the pass read, and whether it ran to the end
-- (done), ran out of time or calls (timeout), or failed. A scene whose prose
-- has moved past the version settled "done" is waiting for a settlement again.
CREATE TABLE scene_settlements (
  node_id         TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  project_id      TEXT NOT NULL,
  content_version INTEGER NOT NULL,
  outcome         TEXT NOT NULL, -- done | timeout | failed
  settled_at      INTEGER NOT NULL
);
CREATE INDEX scene_settlements_project ON scene_settlements(project_id);
