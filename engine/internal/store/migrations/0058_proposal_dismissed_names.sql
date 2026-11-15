-- #387 Phase 1: names the writer turned down. Rejecting a create_entity
-- proposal records its name here, and a tidy-up batch (purpose=organize) may
-- not propose it again. normalized is proposals.normalizeForDedupe(name), so
-- "김 민호" and "김민호" are one name. The writer can let a name back in.
CREATE TABLE proposal_dismissed_names (
  project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind         TEXT NOT NULL,
  normalized   TEXT NOT NULL,
  name         TEXT NOT NULL,
  dismissed_at INTEGER NOT NULL,
  PRIMARY KEY (project_id, kind, normalized)
);
