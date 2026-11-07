-- What only the writer knows, and what a person's knowing is about. answer is
-- the truth behind a question, kept from the prose until the story pays it.
-- about points a knows fact at the question it answers, so two knows facts
-- about one secret can be compared. Deleting the question leaves the knows
-- fact as a free note.
ALTER TABLE story_ledger ADD COLUMN answer TEXT NOT NULL DEFAULT '';
ALTER TABLE story_ledger ADD COLUMN about TEXT REFERENCES story_ledger(id) ON DELETE SET NULL;

CREATE INDEX story_ledger_about ON story_ledger(project_id, about);
