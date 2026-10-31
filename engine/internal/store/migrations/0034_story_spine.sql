-- What a long work needs kept apart from its prose: an episode's plan, the
-- ledger of what the story owes and has settled, and each part's dramatic job.
--
-- plan is separate from summary so writing a scene no longer overwrites what it
-- was meant to do. Plans an agent kept in the summary of an unwritten scene move
-- across here; summaries of scenes that have been written stay where they are.
ALTER TABLE nodes ADD COLUMN plan TEXT NOT NULL DEFAULT '';

UPDATE nodes
   SET plan = summary, summary = '', summary_for_version = 0, summary_written = 0
 WHERE kind = 'leaf' AND word_count = 0 AND summary LIKE '[계획%';

CREATE TABLE story_ledger (
  id              TEXT PRIMARY KEY,
  project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL,            -- promise|gag|question|decision|number|timeline|knowledge|cut_plan
  subject         TEXT NOT NULL DEFAULT '', -- who or what it is about
  body            TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'open', -- open|paid|dropped
  planted_node_id TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  planted_quote   TEXT NOT NULL DEFAULT '',
  due_node_id     TEXT REFERENCES nodes(id) ON DELETE SET NULL, -- a scene, or a part to pay it by the end of
  paid_node_id    TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  paid_quote      TEXT NOT NULL DEFAULT '',
  source          TEXT NOT NULL,            -- writer|agent|external
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL
);
CREATE INDEX story_ledger_project ON story_ledger(project_id, status);

CREATE TABLE part_arcs (
  node_id    TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  role       TEXT NOT NULL DEFAULT '', -- the part's job in the whole: 기, 승, 전, 결, or the writer's own word
  goal       TEXT NOT NULL DEFAULT '', -- what must be true by its end
  tension    INTEGER NOT NULL DEFAULT 0, -- 0 unset, 1..10
  updated_at INTEGER NOT NULL
);
CREATE INDEX part_arcs_project ON part_arcs(project_id);

ALTER TABLE project_style_rules ADD COLUMN anchor_node_id TEXT NOT NULL DEFAULT '';
