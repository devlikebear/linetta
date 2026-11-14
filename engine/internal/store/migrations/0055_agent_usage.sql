-- What every request the built-in agent sends has cost, one row per request:
-- the turn's own round trips and each background pass after it. Kept on this
-- device for the usage table in Settings, and to tell what made a turn slow or
-- expensive. input_tokens is what the provider reported as input; the cache
-- columns keep the provider's split as it came (see tars llm.Usage):
-- cached_tokens is a part of input_tokens, cache_read_tokens and
-- cache_write_tokens are on top of it.
CREATE TABLE agent_usage (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id             TEXT    NOT NULL DEFAULT '',
  project_id         TEXT    NOT NULL DEFAULT '',
  kind               TEXT    NOT NULL,
  provider           TEXT    NOT NULL DEFAULT '',
  model              TEXT    NOT NULL DEFAULT '',
  input_tokens       INTEGER NOT NULL DEFAULT 0,
  output_tokens      INTEGER NOT NULL DEFAULT 0,
  cached_tokens      INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
  cache_write_tokens INTEGER NOT NULL DEFAULT 0,
  created_at         INTEGER NOT NULL
);
CREATE INDEX agent_usage_created_at ON agent_usage (created_at);
