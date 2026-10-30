-- The picture that stands for a scene, an episode or a part (illustration
-- Phase 4.3): one per node. A separate table rather than a nodes column, the
-- same way publish_covers holds the work's cover, so the node queries stay as
-- they are and deleting a node takes its choice with it.
--
-- illustration_id is not a foreign key: illustration.Repo.Delete refuses to
-- delete a representative picture (ErrIsRepresentative), the way it refuses
-- one a scene shows.
CREATE TABLE node_representative_illustrations (
  node_id         TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  illustration_id TEXT NOT NULL,
  updated_at      INTEGER NOT NULL
);

CREATE INDEX idx_node_representative_project ON node_representative_illustrations(project_id);
CREATE INDEX idx_node_representative_illustration ON node_representative_illustrations(illustration_id);
