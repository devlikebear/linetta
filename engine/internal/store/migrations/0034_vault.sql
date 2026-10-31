-- 0034_vault.sql — which vault item holds each work, as this device last saw it.
--
-- The vault lives on Linetta Works; this row is only this device's bookmark.
-- item_id is random and says nothing about the project, so the site cannot tell
-- which work an item is. base_revision is the server revision this library last
-- matched, and last_hash the content hash at that moment: comparing them with
-- the server and with the work is how push and pull tell who changed what.
CREATE TABLE vault_items (
  project_id     TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  item_id        TEXT NOT NULL UNIQUE,
  base_revision  INTEGER NOT NULL DEFAULT 0,
  last_hash      TEXT NOT NULL DEFAULT '',
  last_synced_at INTEGER NOT NULL DEFAULT 0
);
