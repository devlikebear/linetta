-- What the writer chose to show on Linetta Works beside the episodes
-- (works Phase 8). These are the writer's choices, not a copy of the site, so
-- connecting another account (StateRepo.Clear) keeps them.
--
-- A sheet is published only while it has a row here: sheets can hold spoilers
-- and private notes, so nothing goes out unless the writer turned it on.
CREATE TABLE publish_entities (
  entity_id  TEXT PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL
);

CREATE INDEX idx_publish_entities_project ON publish_entities(project_id);

-- The gallery picture the writer chose as the work's cover. illustration_id
-- is not a foreign key: illustration.Repo.Delete refuses to delete a cover
-- (ErrIsCover), the same way it refuses a picture a scene shows.
CREATE TABLE publish_covers (
  project_id      TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  illustration_id TEXT NOT NULL,
  updated_at      INTEGER NOT NULL
);
