-- Fact Book: source-backed real-world research cards for a project/scene.
CREATE TABLE fact_cards (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id     TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  claim       TEXT NOT NULL,
  result      TEXT NOT NULL,
  status      TEXT NOT NULL,
  category    TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
CREATE INDEX idx_fact_cards_project ON fact_cards(project_id, updated_at);
CREATE INDEX idx_fact_cards_node ON fact_cards(node_id, updated_at);

CREATE TABLE fact_sources (
  id          TEXT PRIMARY KEY,
  card_id     TEXT NOT NULL REFERENCES fact_cards(id) ON DELETE CASCADE,
  url         TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  snippet     TEXT NOT NULL DEFAULT '',
  accessed_at INTEGER NOT NULL
);
CREATE INDEX idx_fact_sources_card ON fact_sources(card_id);
