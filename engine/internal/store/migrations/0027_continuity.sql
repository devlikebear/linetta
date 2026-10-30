-- An agent's continuity reading of one scene (story-assist Phase 2). Nothing
-- here is canon and nothing here changes the manuscript: it is a list of
-- places the writer may want to look at, each tied to the exact text and the
-- scene version it was read from. Kept out of fact_cards on purpose: a fact
-- card is the writer's own record of the work, while a continuity
-- observation is an agent's judgement about whether the prose agrees with
-- that record — a reading, not a fact.
CREATE TABLE continuity_checks (
  id              TEXT PRIMARY KEY,
  project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id         TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  content_version INTEGER NOT NULL,
  source          TEXT NOT NULL,      -- "agent" | "external"
  note            TEXT NOT NULL DEFAULT '',
  checked_at      INTEGER NOT NULL
);
CREATE INDEX continuity_checks_node ON continuity_checks(node_id, checked_at);

CREATE TABLE continuity_observations (
  id             TEXT PRIMARY KEY,
  check_id       TEXT NOT NULL REFERENCES continuity_checks(id) ON DELETE CASCADE,
  project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id        TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  category       TEXT NOT NULL,   -- character|place|item|relationship|timeline|fact|plot|direction|other
  kind           TEXT NOT NULL,   -- contradiction|gap|question
  severity       TEXT NOT NULL,   -- high|medium|low
  summary        TEXT NOT NULL,
  suggestion     TEXT NOT NULL DEFAULT '',
  paragraph_from INTEGER NOT NULL,
  paragraph_to   INTEGER NOT NULL,
  quote          TEXT NOT NULL,
  refs_json      TEXT NOT NULL DEFAULT '[]',  -- [{type:entity|fact|beat|scene, id, paragraph?, quote?}]
  status         TEXT NOT NULL DEFAULT 'open', -- open|dismissed|resolved|replaced
  status_at      INTEGER NOT NULL,
  created_at     INTEGER NOT NULL
);
CREATE INDEX continuity_obs_project ON continuity_observations(project_id, status);
CREATE INDEX continuity_obs_check ON continuity_observations(check_id);
