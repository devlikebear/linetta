-- Daily writing progress, grouped by local YYYY-MM-DD.
CREATE TABLE writing_stats (
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  day         TEXT NOT NULL,
  chars_added INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(project_id, day)
);
