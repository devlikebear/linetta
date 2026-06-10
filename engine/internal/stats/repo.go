// Package stats tracks writing progress derived from saved node content.
package stats

import (
	"context"
	"database/sql"
	"time"

	"github.com/devlikebear/linetta/engine/internal/store"
)

// Today is the on-wire shape for today's writing progress.
type Today struct {
	CharsAdded int `json:"chars_added"`
}

// DayStat is a single calendar bucket for charting.
type DayStat struct {
	Day        string `json:"day"`
	CharsAdded int    `json:"chars_added"`
}

// Summary is the compact stats row shown in the writing sidebar.
type Summary struct {
	Today     int `json:"today"`
	WeekAvg   int `json:"week_avg"`
	TotalDays int `json:"total_days"`
}

// Repo persists per-project daily writing stats.
type Repo struct {
	s     *store.Store
	loc   *time.Location
	clock func() time.Time
}

// NewRepo returns a stats repo using the process local timezone.
func NewRepo(s *store.Store) *Repo {
	return NewRepoWithClock(s, time.Local, time.Now)
}

// NewRepoWithLocation returns a stats repo using loc for day bucketing.
func NewRepoWithLocation(s *store.Store, loc *time.Location) *Repo {
	return NewRepoWithClock(s, loc, time.Now)
}

// NewRepoWithClock returns a stats repo with deterministic time injection.
func NewRepoWithClock(s *store.Store, loc *time.Location, clock func() time.Time) *Repo {
	if loc == nil {
		loc = time.Local
	}
	if clock == nil {
		clock = time.Now
	}
	return &Repo{s: s, loc: loc, clock: clock}
}

// RecordNodeDelta adds only positive growth to the local day bucket. It accepts
// the caller's transaction so content and stats stay in lockstep.
func (r *Repo) RecordNodeDelta(ctx context.Context, tx *sql.Tx, projectID string, oldCount int, newCount int, nowMillis int64) error {
	delta := newCount - oldCount
	if delta <= 0 {
		return nil
	}
	day := time.UnixMilli(nowMillis).In(r.loc).Format("2006-01-02")
	_, err := tx.ExecContext(ctx, `
INSERT INTO writing_stats (project_id, day, chars_added)
VALUES (?, ?, ?)
ON CONFLICT(project_id, day) DO UPDATE
   SET chars_added = chars_added + excluded.chars_added`,
		projectID, day, delta)
	return err
}

// Today returns the current local day's accumulated positive writing progress.
func (r *Repo) Today(ctx context.Context, projectID string) (Today, error) {
	return r.GetDay(ctx, projectID, r.clock().In(r.loc).Format("2006-01-02"))
}

// GetDay returns the accumulated progress for a specific YYYY-MM-DD bucket.
func (r *Repo) GetDay(ctx context.Context, projectID string, day string) (Today, error) {
	var out Today
	err := r.s.DB().QueryRowContext(ctx, `
SELECT chars_added
  FROM writing_stats
 WHERE project_id = ? AND day = ?`, projectID, day).Scan(&out.CharsAdded)
	if err == sql.ErrNoRows {
		return Today{}, nil
	}
	return out, err
}

// Range returns one row per day from fromDay to toDay inclusive, filling blank
// calendar days with zero progress.
func (r *Repo) Range(ctx context.Context, projectID string, fromDay string, toDay string) ([]DayStat, error) {
	from, err := time.ParseInLocation("2006-01-02", fromDay, r.loc)
	if err != nil {
		return nil, err
	}
	to, err := time.ParseInLocation("2006-01-02", toDay, r.loc)
	if err != nil {
		return nil, err
	}
	if from.After(to) {
		return []DayStat{}, nil
	}

	rows, err := r.s.DB().QueryContext(ctx, `
SELECT day, chars_added
  FROM writing_stats
 WHERE project_id = ? AND day >= ? AND day <= ?
 ORDER BY day`, projectID, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byDay := map[string]int{}
	for rows.Next() {
		var stat DayStat
		if err := rows.Scan(&stat.Day, &stat.CharsAdded); err != nil {
			return nil, err
		}
		byDay[stat.Day] = stat.CharsAdded
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]DayStat, 0, int(to.Sub(from).Hours()/24)+1)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		out = append(out, DayStat{Day: key, CharsAdded: byDay[key]})
	}
	return out, nil
}

// Summary returns today's progress, recent 7-day calendar average, and the
// count of days where positive writing was recorded.
func (r *Repo) Summary(ctx context.Context, projectID string) (Summary, error) {
	todayKey := r.clock().In(r.loc).Format("2006-01-02")
	weekStart := r.clock().In(r.loc).AddDate(0, 0, -6).Format("2006-01-02")
	var out Summary
	err := r.s.DB().QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(CASE WHEN day = ? THEN chars_added ELSE 0 END), 0) AS today,
  COALESCE(SUM(CASE WHEN day >= ? AND day <= ? THEN chars_added ELSE 0 END), 0) AS week_sum,
  COUNT(CASE WHEN chars_added > 0 THEN 1 END) AS total_days
FROM writing_stats
WHERE project_id = ?`, todayKey, weekStart, todayKey, projectID).Scan(&out.Today, &out.WeekAvg, &out.TotalDays)
	if err != nil {
		return Summary{}, err
	}
	out.WeekAvg = out.WeekAvg / 7
	return out, nil
}
