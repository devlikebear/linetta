-- The prose a settlement pass read, as a hash of its plain text (longform
-- Phase 6 follow-up). Relinking mentions rewrites a scene's document and bumps
-- its content_version without changing a word; comparing this hash lets such a
-- scene stay settled instead of waiting for another pass. Empty when the scene
-- had already moved on by the time the pass was recorded.
ALTER TABLE scene_settlements ADD COLUMN text_hash TEXT NOT NULL DEFAULT '';
