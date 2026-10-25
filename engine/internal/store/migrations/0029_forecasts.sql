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

-- A proposal batch submitted for purpose=forecast (story-assist Phase 4)
-- names which forecast and selected branch it is turning into outline
-- structure, so Submit can require that branch to still be selected and
-- Accept's undo (proposals.undo) has something to key its own undo window
-- on alongside storyops' own batch id. NULL for every other purpose.
ALTER TABLE story_proposals ADD COLUMN forecast_id TEXT REFERENCES forecasts(id) ON DELETE SET NULL;

-- One structural batch (storyops.ApplyOpsResult.UndoBatchID) can cover
-- several story_proposal_items at once: the create_outline_node/
-- create_scene items applied together, plus any add_beat item that
-- depended on a node one of them created. proposals.undo reads this
-- column to find every item a single storyops undo must carry back to
-- pending together, and to delete the beats those add_beat items created
-- (RestoreOutline only puts the outline back; beats.node_id is
-- ON DELETE SET NULL, not CASCADE, so an orphaned beat would otherwise
-- survive its scene's own undo). NULL for every item accepted outside a
-- forecast-purpose structural batch.
ALTER TABLE story_proposal_items ADD COLUMN undo_batch_id TEXT;
CREATE INDEX story_proposal_items_undo_batch ON story_proposal_items(undo_batch_id) WHERE undo_batch_id IS NOT NULL;
