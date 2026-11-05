-- Which characters a work counts: every one ('with_spaces', what every work
-- counted until now) or all but whitespace ('without_spaces'). Scenes'
-- word_count and the work's total follow it.
ALTER TABLE projects ADD COLUMN char_count_basis TEXT NOT NULL DEFAULT 'with_spaces';
