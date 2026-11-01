-- 0040_sync_targets.sql — a work can be linked to more than one place.
--
-- GitHub and a folder keep the same revisions Linetta Works does, so each target
-- gets its own bookmark for a work: its own item, base revision and hash.
-- Existing rows are Linetta Works.
--
-- head_mark is the manifest of the revision a link last matched. Linetta Works
-- gives every commit its own number, so the number is enough there. A folder has
-- no server to hand numbers out, two devices can both write revision 12, and only
-- the manifest tells them apart.
CREATE TABLE vault_items_targets (
  project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  target         TEXT NOT NULL DEFAULT 'works',
  item_id        TEXT NOT NULL,
  base_revision  INTEGER NOT NULL DEFAULT 0,
  last_hash      TEXT NOT NULL DEFAULT '',
  last_synced_at INTEGER NOT NULL DEFAULT 0,
  head_mark      TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project_id, target),
  UNIQUE (target, item_id)
);
INSERT INTO vault_items_targets (project_id, target, item_id, base_revision, last_hash, last_synced_at)
  SELECT project_id, 'works', item_id, base_revision, last_hash, last_synced_at FROM vault_items;
DROP TABLE vault_items;
ALTER TABLE vault_items_targets RENAME TO vault_items;

ALTER TABLE vault_library ADD COLUMN head_mark TEXT NOT NULL DEFAULT '';

CREATE TABLE vault_deletions_targets (
  target     TEXT NOT NULL DEFAULT 'works',
  item_id    TEXT NOT NULL,
  project_id TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  queued_at  INTEGER NOT NULL,
  PRIMARY KEY (target, item_id)
);
INSERT INTO vault_deletions_targets (target, item_id, project_id, title, queued_at)
  SELECT 'works', item_id, project_id, title, queued_at FROM vault_deletions;
DROP TABLE vault_deletions;
ALTER TABLE vault_deletions_targets RENAME TO vault_deletions;

CREATE TABLE sync_removed_targets (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  target     TEXT NOT NULL DEFAULT 'works',
  removed_at INTEGER NOT NULL,
  PRIMARY KEY (project_id, target)
);
INSERT INTO sync_removed_targets (project_id, target, removed_at)
  SELECT project_id, 'works', removed_at FROM sync_removed;
DROP TABLE sync_removed;
ALTER TABLE sync_removed_targets RENAME TO sync_removed;
