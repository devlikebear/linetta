package snapshot

import (
	"context"
	"database/sql"
)

// Thin enforces autosave retention. Manual and companion-before snapshots are
// never touched. Autosaves:
//   - < 24h ago: keep all
//   - 24h–30d: one per (node_id, hour bucket)
//   - > 30d:   one per (node_id, day bucket)
//
// Implementation strategy: compute "keep" ids in two CTE branches and delete
// every autosave row not in that set.
func Thin(ctx context.Context, db *sql.DB, nowMillis int64) error {
	const dayMs = int64(24 * 60 * 60 * 1000)
	cutoff24h := nowMillis - dayMs
	cutoff30d := nowMillis - 30*dayMs

	// Phase 1: bucket = 24h..30d, hour-grouped, keep most recent per (node, hour).
	if _, err := db.ExecContext(ctx, `
DELETE FROM node_snapshots
 WHERE reason = 'autosave'
   AND created_at <= ?
   AND created_at >  ?
   AND id NOT IN (
     SELECT id FROM (
       SELECT id,
              ROW_NUMBER() OVER (
                PARTITION BY node_id, created_at / (60*60*1000)
                ORDER BY created_at DESC
              ) AS rn
         FROM node_snapshots
        WHERE reason = 'autosave'
          AND created_at <= ?
          AND created_at >  ?
     )
     WHERE rn = 1
   )`, cutoff24h, cutoff30d, cutoff24h, cutoff30d); err != nil {
		return err
	}

	// Phase 2: bucket = > 30d, day-grouped.
	if _, err := db.ExecContext(ctx, `
DELETE FROM node_snapshots
 WHERE reason = 'autosave'
   AND created_at <= ?
   AND id NOT IN (
     SELECT id FROM (
       SELECT id,
              ROW_NUMBER() OVER (
                PARTITION BY node_id, created_at / (24*60*60*1000)
                ORDER BY created_at DESC
              ) AS rn
         FROM node_snapshots
        WHERE reason = 'autosave'
          AND created_at <= ?
     )
     WHERE rn = 1
   )`, cutoff30d, cutoff30d); err != nil {
		return err
	}
	return nil
}
