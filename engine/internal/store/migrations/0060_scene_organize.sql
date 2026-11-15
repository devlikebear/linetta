-- #387 Phase 2: how each scene's last organize pass went, the same shape as
-- scene_settlements. The pass reads the writer's own prose and proposes, into
-- the suggestions inbox, the people, places, ties and foreshadowing it finds.
-- A scene whose words (text_hash) match a done row is not read again.
CREATE TABLE scene_organize (
  node_id         TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  project_id      TEXT NOT NULL,
  content_version INTEGER NOT NULL,
  text_hash       TEXT NOT NULL DEFAULT '',
  outcome         TEXT NOT NULL, -- done | timeout | failed
  organized_at    INTEGER NOT NULL
);
CREATE INDEX scene_organize_project ON scene_organize(project_id);
