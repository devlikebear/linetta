-- The rules a style check can decide without a model: phrases the writer
-- wants kept out of a work, and how long a sentence may run (#162). Style
-- notes stay prose on the projects row; these are the part of a writer's
-- style that a machine can hold a draft against.
--
-- avoid_phrases is a JSON array of strings. max_sentence_chars is in
-- characters (runes), 0 meaning no limit.
CREATE TABLE project_style_rules (
  project_id         TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  avoid_phrases      TEXT NOT NULL DEFAULT '[]',
  max_sentence_chars INTEGER NOT NULL DEFAULT 0,
  updated_at         INTEGER NOT NULL
);
