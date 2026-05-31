-- Plan 25 data-integrity hardening.
-- Enum-like constraints remain enforced in Go repos/handlers because SQLite
-- cannot add CHECK constraints to existing tables in place without a table-copy
-- migration. These indexes support the new project/node ownership validation.
CREATE INDEX IF NOT EXISTS idx_nodes_project_id ON nodes(project_id, id);
CREATE INDEX IF NOT EXISTS idx_snapshots_reason ON node_snapshots(reason);
