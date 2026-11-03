-- A settled entry can say what it is about and how long it holds. planted_node_id
-- is where a fact starts; valid_to_node_id is the scene it stops being true at.
-- Strength tells what the narrator stated (hard) from what a character said or a
-- rumour carried (soft), which the story may yet turn over.
ALTER TABLE story_ledger ADD COLUMN attribute TEXT NOT NULL DEFAULT '';
ALTER TABLE story_ledger ADD COLUMN valid_to_node_id TEXT REFERENCES nodes(id) ON DELETE SET NULL;
ALTER TABLE story_ledger ADD COLUMN superseded_by TEXT NOT NULL DEFAULT '';
ALTER TABLE story_ledger ADD COLUMN strength TEXT NOT NULL DEFAULT 'hard';
ALTER TABLE story_ledger ADD COLUMN value_num REAL;

CREATE INDEX story_ledger_fact ON story_ledger(project_id, subject, attribute);
