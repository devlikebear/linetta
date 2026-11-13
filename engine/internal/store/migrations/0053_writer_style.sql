-- The writer's default style (#178, #179): checkable rules that sit under
-- every work's own. One row, kept in the library rather than in any work, so a
-- restored work picks up the library's writer rules, as the writer profile does.
CREATE TABLE writer_style (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  avoid_phrases      TEXT NOT NULL DEFAULT '[]',
  max_sentence_chars INTEGER NOT NULL DEFAULT 0,
  updated_at         INTEGER NOT NULL
);

-- A work's sentence limit can now be its own or the writer's. 1 means the work
-- set one (max_sentence_chars, where 0 is "no limit"); 0 means it follows the
-- writer's. A work that set a limit before this keeps it as its own; one that
-- had none follows the writer's, which until now did not exist, so nothing
-- changes for it.
ALTER TABLE project_style_rules ADD COLUMN sentence_limit_set INTEGER NOT NULL DEFAULT 0;
UPDATE project_style_rules SET sentence_limit_set = 1 WHERE max_sentence_chars > 0;

-- "This work does not follow the writer's default style." A work with no row
-- follows it.
ALTER TABLE project_style_rules ADD COLUMN ignore_writer_style INTEGER NOT NULL DEFAULT 0;
