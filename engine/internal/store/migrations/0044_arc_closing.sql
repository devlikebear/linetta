-- How far a promise reaches, so closing an arc can take its promises out of the
-- brief. What is already owed stays in sight: only a writer or an arc's closing
-- narrows it.
ALTER TABLE story_ledger ADD COLUMN reach TEXT NOT NULL DEFAULT 'arc';
UPDATE story_ledger SET reach = 'work';

-- Which scene pairs a sample check has compared, at which versions, so a later
-- one spends its calls on what it has not seen. Rebuilt by checking again, so it
-- stays on this device.
CREATE TABLE continuity_sample_pairs (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_a     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  version_a  INTEGER NOT NULL,
  node_b     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  version_b  INTEGER NOT NULL,
  checked_at INTEGER NOT NULL,
  PRIMARY KEY (node_a, version_a, node_b, version_b)
);
CREATE INDEX continuity_sample_pairs_project ON continuity_sample_pairs(project_id);
