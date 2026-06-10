-- Per-episode target for web novel serial drafting progress.
ALTER TABLE projects ADD COLUMN episode_char_target INTEGER NOT NULL DEFAULT 5000;
