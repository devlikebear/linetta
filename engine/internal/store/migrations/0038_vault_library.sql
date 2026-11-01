-- 0038_vault_library.sql — what travels outside any work.
--
-- sync_selection is the writer's choice of what to carry, the same on every
-- device: it is sealed into the library item, and this row is the copy every
-- export reads. omit is a JSON array of category ids; updated_at decides between
-- two devices that changed it.
--
-- vault_library is this device's bookmark into the library item, one row per
-- place it is kept. base_parts is a JSON object of part name to hash as of
-- base_revision: comparing a part with it says which side changed, so a skill
-- deleted on one device is not brought back by the device that never saw it go.
CREATE TABLE sync_selection (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  omit       TEXT NOT NULL DEFAULT '[]',
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE vault_library (
  target         TEXT PRIMARY KEY,
  base_revision  INTEGER NOT NULL DEFAULT 0,
  last_hash      TEXT NOT NULL DEFAULT '',
  base_parts     TEXT NOT NULL DEFAULT '{}',
  last_synced_at INTEGER NOT NULL DEFAULT 0
);
