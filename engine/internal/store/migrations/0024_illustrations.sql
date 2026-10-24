-- Generated illustrations, kept per work. BLOBs for the same reason as the
-- reference images in 0020: daily backups copy library.db and nothing else.
-- source_node_id is the scene the picture was drawn from, if any; a scene
-- deleted later leaves the picture in the gallery.
CREATE TABLE illustrations (
 id TEXT PRIMARY KEY,
 project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 source_node_id TEXT REFERENCES nodes(id) ON DELETE SET NULL,
 caption TEXT NOT NULL DEFAULT '',
 prompt TEXT NOT NULL,
 provider TEXT NOT NULL,
 model TEXT NOT NULL,
 mime TEXT NOT NULL,
 width INTEGER NOT NULL DEFAULT 0,
 height INTEGER NOT NULL DEFAULT 0,
 byte_size INTEGER NOT NULL,
 data BLOB NOT NULL,
 created_at INTEGER NOT NULL
);
CREATE INDEX illustrations_project ON illustrations(project_id, created_at);
