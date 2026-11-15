-- #384 Phase 2: meaning search. One vector per paragraph of a work's prose,
-- keyed by the paragraph's text: a paragraph that did not change keeps its
-- vector wherever it moves, and one that did gets a new one. Derived from
-- manuscript_chunks and rebuilt from it; never edited on its own. vec is the
-- unit vector quantized to int8 (round(x*127)), dim bytes long.
CREATE TABLE paragraph_vectors (
  project_id TEXT NOT NULL,
  model      TEXT NOT NULL,
  text_hash  TEXT NOT NULL,
  vec        BLOB NOT NULL,
  PRIMARY KEY (project_id, model, text_hash)
);
