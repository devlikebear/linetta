-- Lines moved out of a work's notes. The notes are in every turn's prompt and
-- their budget is deliberately small, so a settled decision has to leave them
-- for a long work to keep going; this is where it goes rather than being lost.
-- Nothing here reaches a prompt until something searches for it.
--
-- source_message_id has no foreign key: an archived line travels with its work,
-- and the conversation it came from stays on the device that had it.
CREATE TABLE agent_memory_archive (
  id                TEXT PRIMARY KEY,
  project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  body              TEXT NOT NULL,
  source_message_id TEXT NOT NULL DEFAULT '',
  archived_at       INTEGER NOT NULL
);

CREATE INDEX idx_agent_memory_archive_project
  ON agent_memory_archive(project_id, archived_at);

-- Linked by id, not rowid, for the reason 0036 gives.
CREATE VIRTUAL TABLE agent_memory_archive_fts USING fts5(
  body,
  archive_id UNINDEXED,
  project_id UNINDEXED,
  tokenize = 'trigram'
);

CREATE TRIGGER agent_memory_archive_fts_insert
AFTER INSERT ON agent_memory_archive
BEGIN
  INSERT INTO agent_memory_archive_fts (body, archive_id, project_id)
  VALUES (new.body, new.id, new.project_id);
END;

CREATE TRIGGER agent_memory_archive_fts_delete
AFTER DELETE ON agent_memory_archive
BEGIN
  DELETE FROM agent_memory_archive_fts WHERE archive_id = old.id;
END;
