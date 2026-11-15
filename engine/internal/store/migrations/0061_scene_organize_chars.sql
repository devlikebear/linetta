-- #387 Phase 3: the length of the prose an organize pass read. A pass that
-- runs on its own (leaving an episode, a long idle) reads a scene again only
-- when it grew or shrank by enough to hold something new.
ALTER TABLE scene_organize ADD COLUMN char_count INTEGER NOT NULL DEFAULT 0;
