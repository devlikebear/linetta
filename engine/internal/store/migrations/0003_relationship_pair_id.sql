-- Plan 8: bidirectional relationship pairing.
-- pair_id groups two rows that were created together (A→B and B→A).
-- NULL = singleton (no inverse). SQLite-safe: nullable, no default.
ALTER TABLE relationships ADD COLUMN pair_id TEXT;

CREATE INDEX IF NOT EXISTS idx_relationships_from ON relationships(from_id);
CREATE INDEX IF NOT EXISTS idx_relationships_pair ON relationships(pair_id);
