CREATE VIRTUAL TABLE manuscript_fts USING fts5(
  plain,
  node_id UNINDEXED,
  project_id UNINDEXED,
  tokenize = 'trigram'
);
