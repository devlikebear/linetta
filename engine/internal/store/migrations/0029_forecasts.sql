-- Possible continuations an agent drafted for the writer to compare
-- (story-assist Phase 4). Never canon: nothing that builds the outline, the
-- manuscript or Story World reads this table. The writer's chosen branch
-- reaches an agent only as a labelled plan in the brief's direction section
-- (storycontext.DirectionBrief.PlannedBranch), never as a write an agent
-- made on its own.
CREATE TABLE forecasts (
  id                TEXT PRIMARY KEY,
  project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  divergence        TEXT NOT NULL,      -- the question the writer asked
  base_node_id      TEXT,               -- last written episode at save time (episode.LastWritten)
  horizon           INTEGER NOT NULL,   -- episodes ahead, 1..10
  branches_json     TEXT NOT NULL,
  fingerprint       TEXT NOT NULL,      -- sha256 over input_hashes_json, order-independent
  input_hashes_json TEXT NOT NULL,      -- {group: hash, episodes: {node_id: hash}} — forecast.InputHashes
  source            TEXT NOT NULL,      -- agent | external
  selected_branch   TEXT,               -- b1..bN; one selection per project (see forecasts_selected_project)
  selected_at       INTEGER,
  created_at        INTEGER NOT NULL
);
CREATE INDEX forecasts_project ON forecasts(project_id, created_at);
-- At most one selected forecast per project: Select clears any previous
-- selection in the same project before setting this one (forecast.Repo.Select),
-- and this partial index is the storage-level backstop for that rule.
CREATE UNIQUE INDEX forecasts_selected_project ON forecasts(project_id) WHERE selected_branch IS NOT NULL;
