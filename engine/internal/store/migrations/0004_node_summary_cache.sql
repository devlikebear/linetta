-- Plan 9: LLM-cached previous-scene summary.
ALTER TABLE nodes ADD COLUMN summary TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN content_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN summary_for_version INTEGER NOT NULL DEFAULT 0;
