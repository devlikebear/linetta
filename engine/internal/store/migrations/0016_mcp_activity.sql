-- Audit trail for tools called by external MCP clients. This is the writer's
-- answer to "what did the agent do while I was asleep": every tool call, read
-- or write, success or failure, lands here and is shown in Settings.
--
-- project_id is intentionally NOT a foreign key: the log must survive the work
-- it refers to, so deleting a project never erases the record of what was done
-- to it.
CREATE TABLE mcp_activity (
  id         TEXT PRIMARY KEY,
  at         INTEGER NOT NULL,
  tool       TEXT NOT NULL,
  project_id TEXT NOT NULL DEFAULT '',
  target_id  TEXT NOT NULL DEFAULT '',
  ok         INTEGER NOT NULL DEFAULT 1,
  detail     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_mcp_activity_at ON mcp_activity(at DESC);
