-- A scene the writer marks as a flashback (longform Phase 8). It happens
-- earlier in story time than the scenes around it, so the ledger leaves it out
-- of comparisons in story order (a date going back, an elapsed time shrinking)
-- and its facts do not change the current state of the scenes after it.
ALTER TABLE nodes ADD COLUMN flashback INTEGER NOT NULL DEFAULT 0;
