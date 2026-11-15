-- Each scene's prose cut into runs of whole paragraphs, so a search can say
-- which paragraphs it found something in, not only which scene. Derived from
-- nodes like manuscript_fts, and kept by the same indexer in the same
-- transaction; never edited on its own. para_from and para_to are 1-based and
-- inclusive, numbered as style.Paragraphs numbers them (the numbers
-- linetta_read_scene numbered=true prints). Chunks of a scene do not overlap.
-- text_hash lets a later derived index (embeddings) key on the text itself,
-- so an unchanged chunk is never processed twice, wherever it moves.
CREATE TABLE manuscript_chunks (
  project_id TEXT    NOT NULL,
  node_id    TEXT    NOT NULL,
  para_from  INTEGER NOT NULL,
  para_to    INTEGER NOT NULL,
  text       TEXT    NOT NULL,
  text_hash  TEXT    NOT NULL,
  PRIMARY KEY (node_id, para_from)
);
CREATE INDEX manuscript_chunks_project ON manuscript_chunks (project_id);
