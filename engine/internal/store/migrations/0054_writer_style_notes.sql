-- The writer's default style in prose (#182): how the writing should read in
-- every work, which goes into every brief as the writer's own direction ahead
-- of the work's style notes. Only the writer sets it.
ALTER TABLE writer_style ADD COLUMN notes TEXT NOT NULL DEFAULT '';

-- An agent's proposal for it (#183), waiting for the writer. Nothing reads it
-- when a brief is built; only the app's approve action moves it into notes.
CREATE TABLE writer_style_draft (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  body       TEXT NOT NULL,
  author     TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
