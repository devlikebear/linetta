-- #386 B1: a relationship holds over a stretch of the story. since_node_id is
-- the scene it starts in (NULL: from the start), until_node_id the scene the
-- next one takes over in (NULL: to the end). change_relationship ends a row at
-- a scene and starts its successor there, which names it in supersedes_id.
-- since_quote is the line the change rests on. A deleted scene leaves the
-- reference NULL; the successor still wins over the row it supersedes.
ALTER TABLE relationships ADD COLUMN since_node_id TEXT REFERENCES nodes(id) ON DELETE SET NULL;
ALTER TABLE relationships ADD COLUMN until_node_id TEXT REFERENCES nodes(id) ON DELETE SET NULL;
ALTER TABLE relationships ADD COLUMN since_quote TEXT NOT NULL DEFAULT '';
ALTER TABLE relationships ADD COLUMN supersedes_id TEXT;
