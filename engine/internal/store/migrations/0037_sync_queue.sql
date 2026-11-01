-- 0037_sync_queue.sql — what automatic sync has to remember between runs.
--
-- vault_deletions holds works deleted here while they were on Linetta Works. The
-- link goes with the project, so the item id moves here first, and the next run
-- commits the marker that tells other devices. It waits here through any time
-- offline. title is what a device that never held the work shows for it.
--
-- sync_removed holds works the writer took off the server by hand. Automatic
-- sync must not upload them straight back; a manual upload clears the row.
CREATE TABLE vault_deletions (
  item_id    TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  queued_at  INTEGER NOT NULL
);

CREATE TABLE sync_removed (
  project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  removed_at INTEGER NOT NULL
);
