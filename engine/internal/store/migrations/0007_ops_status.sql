CREATE TABLE IF NOT EXISTS ops_status (
  job_name         TEXT PRIMARY KEY,
  last_started_at  INTEGER,
  last_finished_at INTEGER,
  last_ok          INTEGER NOT NULL DEFAULT 0,
  last_error       TEXT NOT NULL DEFAULT '',
  metadata_json    TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_ops_status_finished
ON ops_status(last_finished_at);
