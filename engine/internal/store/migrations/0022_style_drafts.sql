-- A style profile an agent drafted from the writer's own scenes, waiting for
-- the writer to approve it (#163). It is deliberately not a column on
-- projects: style_notes is what the story brief injects as the writer's
-- wishes, and nothing an agent wrote may sit there until the writer says so.
-- One pending draft per work; a new proposal replaces the old one.
CREATE TABLE project_style_drafts (
  project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  body       TEXT NOT NULL,
  author     TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
