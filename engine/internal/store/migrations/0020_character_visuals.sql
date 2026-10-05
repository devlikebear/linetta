-- Reference images live as BLOBs because daily backups VACUUM INTO library.db
-- only; loose image files would disappear when a writer restores a work.
CREATE TABLE entity_visuals (
 entity_id TEXT PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
 age_range TEXT NOT NULL DEFAULT '', build TEXT NOT NULL DEFAULT '',
 hair TEXT NOT NULL DEFAULT '', outfit TEXT NOT NULL DEFAULT '',
 signature TEXT NOT NULL DEFAULT '', palette TEXT NOT NULL DEFAULT '',
 notes TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL
);
CREATE TABLE entity_reference_images (
 id TEXT PRIMARY KEY, entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
 mime TEXT NOT NULL, caption TEXT NOT NULL DEFAULT '', byte_size INTEGER NOT NULL,
 data BLOB NOT NULL, ordinal INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL
);
CREATE INDEX entity_reference_images_entity_id ON entity_reference_images(entity_id);
CREATE TABLE project_art_style (
 project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
 style TEXT NOT NULL DEFAULT '', negative_prompt TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL
);
