-- What a contest or serial site asks of the whole submission, beyond each
-- episode's length: how many episodes, how long the work runs, and whether a
-- synopsis goes with it. 0 (and 0 for the flag) means the work sets none, so
-- every existing work keeps behaving as it did.
ALTER TABLE projects ADD COLUMN submission_min_episodes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE projects ADD COLUMN submission_total_min INTEGER NOT NULL DEFAULT 0;
ALTER TABLE projects ADD COLUMN submission_total_max INTEGER NOT NULL DEFAULT 0;
ALTER TABLE projects ADD COLUMN submission_needs_synopsis INTEGER NOT NULL DEFAULT 0;
