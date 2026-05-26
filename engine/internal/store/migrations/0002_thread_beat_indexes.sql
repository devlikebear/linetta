CREATE INDEX IF NOT EXISTS idx_beats_thread ON beats(thread_id, ordinal);
CREATE INDEX IF NOT EXISTS idx_beats_node   ON beats(node_id);
