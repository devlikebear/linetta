-- Story-structure changes an agent suggested and the writer has not applied
-- (story-assist Phase 3). An item becomes part of the work only through the
-- app's proposals.accept RPC, which runs it through storyops like any other
-- change; no MCP tool accepts one. Kept out of storyops' own apply path on
-- purpose: linetta_apply_story_ops stays the small-change, immediate-apply
-- tool it always was, and a proposal batch pulled from the manuscript or
-- from several world elements at once goes through this review queue
-- instead, one item at a time, before anything is applied.
CREATE TABLE story_proposals (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  purpose    TEXT NOT NULL,          -- general | import_extraction (Phase 4 adds forecast)
  source     TEXT NOT NULL,          -- agent | external
  summary    TEXT NOT NULL DEFAULT '',
  ref_bindings_json TEXT NOT NULL DEFAULT '{}', -- ref -> created id, filled as items are accepted
  created_at INTEGER NOT NULL
);
CREATE INDEX story_proposals_project ON story_proposals(project_id);

CREATE TABLE story_proposal_items (
  id             TEXT PRIMARY KEY,
  proposal_id    TEXT NOT NULL REFERENCES story_proposals(id) ON DELETE CASCADE,
  project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  ordinal        INTEGER NOT NULL,
  op_json        TEXT NOT NULL,              -- one storyops.Op
  depends_on_json TEXT NOT NULL DEFAULT '[]', -- item ids whose ref this op uses
  evidence_json  TEXT NOT NULL DEFAULT '[]', -- [{node_id, paragraph, quote}] verified at propose time
  from_node_ids_json TEXT NOT NULL DEFAULT '[]', -- scenes it was drawn from (import progress)
  duplicates_json TEXT NOT NULL DEFAULT '[]', -- [{entity_id, name, kind, matched}]
  status         TEXT NOT NULL DEFAULT 'pending', -- pending|accepted|rejected|failed
  result_json    TEXT NOT NULL DEFAULT '{}',      -- created id or failure message
  status_at      INTEGER NOT NULL
);
CREATE INDEX story_proposal_items_project ON story_proposal_items(project_id, status);
CREATE INDEX story_proposal_items_proposal ON story_proposal_items(proposal_id);
