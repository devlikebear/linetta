-- Whether nodes.summary was written by someone (an agent's
-- linetta_write_summary, an imported manuscript's own summary) rather than
-- derived by the summarizer from the scene's opening or its children. A
-- written summary that has gone stale is still the best account of the scene
-- there is, so the summarizer must not replace it with an opening cut.
-- Existing rows cannot be told apart and stay 0: derived, as before.
ALTER TABLE nodes ADD COLUMN summary_written INTEGER NOT NULL DEFAULT 0;
