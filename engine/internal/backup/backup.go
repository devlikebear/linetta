// Package backup creates SQLite VACUUM INTO snapshots once per day under
// $LINETTA_HOME/backups/YYYY-MM-DD/library-HHMMSS.db and prunes directories
// older than 14 days.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const retentionDays = 14

// RunDailyIfNeeded creates one backup if today's directory does not yet exist.
// Returns the backup file path (or "" if skipped), didRun, and any error.
// The function is safe to call repeatedly on the same day.
func RunDailyIfNeeded(ctx context.Context, db *sql.DB, home string, now time.Time) (string, bool, error) {
	day := now.Format("2006-01-02")
	root := filepath.Join(home, "backups")
	dir := filepath.Join(root, day)

	if _, err := os.Stat(dir); err == nil {
		return "", false, nil // already ran today
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("stat backup dir: %w", err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("mkdir: %w", err)
	}

	filename := fmt.Sprintf("library-%s.db", now.Format("150405"))
	dst := filepath.Join(dir, filename)
	// VACUUM INTO accepts a single-quoted string literal; sanitize is not strictly
	// needed (we control the path) but escape any embedded single quotes.
	stmt := "VACUUM INTO '" + escapeSQLString(dst) + "'"
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return "", false, fmt.Errorf("VACUUM INTO: %w", err)
	}
	return dst, true, nil
}

// Prune deletes backups/YYYY-MM-DD directories whose date is older than 14 days.
// Non-date directory names are left alone.
func Prune(home string, now time.Time) error {
	root := filepath.Join(home, "backups")
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	cutoff := now.AddDate(0, 0, -retentionDays)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		day, err := time.Parse("2006-01-02", e.Name())
		if err != nil {
			continue // skip non-date names
		}
		if day.Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
				return fmt.Errorf("remove %s: %w", e.Name(), err)
			}
		}
	}
	return nil
}

func escapeSQLString(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'')
		}
		out = append(out, s[i])
	}
	return string(out)
}
