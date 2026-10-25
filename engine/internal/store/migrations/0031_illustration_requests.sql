-- Pictures an agent asked to have drawn (illustration Phase 4). An agent
-- cannot draw: drawing costs the writer's image provider quota, so a tool
-- only leaves a request here and nothing is sent until the writer approves
-- it in the app (illustration.RequestService.Approve).
--
-- target_node_id is not a foreign key: a request whose scene or episode is
-- deleted stays as a record and is marked cancelled the next time the work's
-- requests are read (RequestService.cancelOrphans).
CREATE TABLE illustration_requests (
  id                 TEXT PRIMARY KEY,
  project_id         TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  target_node_id     TEXT,               -- the scene or episode; NULL only for purpose gallery
  purpose            TEXT NOT NULL CHECK (purpose IN ('insert', 'representative', 'gallery')),
  prompt             TEXT NOT NULL,      -- what will be sent, unless the writer edits it
  character_ids_json TEXT NOT NULL DEFAULT '[]',
  include_references INTEGER NOT NULL DEFAULT 0,
  aspect             TEXT NOT NULL,
  caption            TEXT NOT NULL DEFAULT '',
  note               TEXT NOT NULL DEFAULT '', -- the agent's one line to the writer
  source             TEXT NOT NULL CHECK (source IN ('agent', 'external')),
  status             TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'generating', 'done', 'rejected', 'failed', 'cancelled')),
  illustration_id    TEXT,               -- the drawing, once done
  placed_at          INTEGER,            -- when the drawing went where the purpose said
  error              TEXT NOT NULL DEFAULT '',
  created_at         INTEGER NOT NULL,
  decided_at         INTEGER
);

CREATE INDEX idx_illustration_requests_project ON illustration_requests(project_id, status, created_at);
