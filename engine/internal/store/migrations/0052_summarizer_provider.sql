-- The BYOK summarizer (#135). A summary it writes replaces the one before,
-- which is kept here once: a summary is not the manuscript, so there is no
-- undo for it, but a writer who preferred the old one can still read it.
ALTER TABLE nodes ADD COLUMN summary_previous TEXT NOT NULL DEFAULT '';

-- What the summarizer has spent on the writer's provider, all told, for the
-- usage line in Settings. One row.
CREATE TABLE summarizer_usage (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  calls         INTEGER NOT NULL DEFAULT 0,
  input_tokens  INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0
);
