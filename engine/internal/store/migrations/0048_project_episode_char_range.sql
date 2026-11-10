-- The shortest and longest an episode may run, in the work's character basis
-- (serial sites and contests ask for "5,000자 이상" or "4,500~5,500자").
-- 0 means no limit, so every existing work keeps behaving as it did;
-- episode_char_target stays the length to aim for.
ALTER TABLE projects ADD COLUMN episode_char_min INTEGER NOT NULL DEFAULT 0;
ALTER TABLE projects ADD COLUMN episode_char_max INTEGER NOT NULL DEFAULT 0;
