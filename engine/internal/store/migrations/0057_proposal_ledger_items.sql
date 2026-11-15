-- #387 Phase 1: a proposal item can carry a story ledger entry instead of a
-- storyops op — a promise, gag or question to add, or one to mark paid or
-- dropped. ledger_json holds proposals.LedgerItem; '' for an op item.
ALTER TABLE story_proposal_items ADD COLUMN ledger_json TEXT NOT NULL DEFAULT '';
