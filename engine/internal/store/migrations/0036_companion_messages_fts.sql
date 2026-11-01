-- What the built-in agent can search when its conversation no longer fits in
-- the request. A copy of what the writer and the agent said, kept in step with
-- companion_messages by the triggers below.
--
-- Only the agent's own user and assistant rows are indexed. Tool rows hold a
-- cut-off summary, and the 1.0 companion's rows are words the writer never
-- said to this agent.
--
-- The link is message_id rather than rowid: companion_messages has an implicit
-- rowid, which VACUUM may renumber, and the daily backup is VACUUM INTO.
CREATE VIRTUAL TABLE companion_messages_fts USING fts5(
  content,
  message_id UNINDEXED,
  project_id UNINDEXED,
  tokenize = 'trigram'
);

CREATE TRIGGER companion_messages_fts_insert
AFTER INSERT ON companion_messages
WHEN new.intent = 'agent' AND new.role IN ('user', 'assistant') AND trim(new.content) <> ''
BEGIN
  INSERT INTO companion_messages_fts (content, message_id, project_id)
  VALUES (new.content, new.id, new.project_id);
END;

CREATE TRIGGER companion_messages_fts_delete
AFTER DELETE ON companion_messages
WHEN old.intent = 'agent' AND old.role IN ('user', 'assistant')
BEGIN
  DELETE FROM companion_messages_fts WHERE message_id = old.id;
END;

CREATE TRIGGER companion_messages_fts_update
AFTER UPDATE OF content ON companion_messages
WHEN new.intent = 'agent' AND new.role IN ('user', 'assistant')
BEGIN
  DELETE FROM companion_messages_fts WHERE message_id = old.id;
  INSERT INTO companion_messages_fts (content, message_id, project_id)
  SELECT new.content, new.id, new.project_id WHERE trim(new.content) <> '';
END;

INSERT INTO companion_messages_fts (content, message_id, project_id)
SELECT content, id, project_id
  FROM companion_messages
 WHERE intent = 'agent' AND role IN ('user', 'assistant') AND trim(content) <> '';
