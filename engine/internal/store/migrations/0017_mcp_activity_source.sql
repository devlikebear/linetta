-- Who called the tool. Before the built-in agent (#93) every row came from an
-- external MCP client, so that is the default and existing rows keep their
-- meaning without a backfill.
--
-- run_id groups one agent turn's calls together: the panel puts a single undo
-- button on a turn rather than one per tool call. It is NOT NULL DEFAULT ''
-- rather than nullable so the scan needs no COALESCE — an external row's
-- empty string and a nullable column's NULL say the same thing here.
ALTER TABLE mcp_activity ADD COLUMN source TEXT NOT NULL DEFAULT 'external';
ALTER TABLE mcp_activity ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
