-- What the writer holds fixed. A pinned ledger entry reaches every brief and an
-- agent cannot change it. The mark is where the writer started the conversation
-- over: replay begins after that message, and nothing before it is deleted.
ALTER TABLE story_ledger ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;

CREATE TABLE agent_history_marks (
  project_id       TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  start_message_id TEXT NOT NULL,
  set_at           INTEGER NOT NULL
);
