// Package backup creates SQLite VACUUM INTO snapshots once per day under
// $LINETTA_HOME/backups/YYYY-MM-DD/library-HHMMSS.db and prunes directories
// older than 14 days.
package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const retentionDays = 14

const RecoveryFormatVersion = 1

type ManualRecoveryResult struct {
	Path          string `json:"path"`
	FormatVersion int    `json:"format_version"`
}

type recoveryManifest struct {
	Format        string `json:"format"`
	FormatVersion int    `json:"format_version"`
	Database      string `json:"database"`
	CreatedAt     int64  `json:"created_at"`
}

// RunDailyIfNeeded creates one backup if today's completed marker is absent.
// Returns the backup file path (or "" if skipped), didRun, and any error.
// The function is safe to call repeatedly on the same day.
func RunDailyIfNeeded(ctx context.Context, db *sql.DB, home string, now time.Time) (string, bool, error) {
	day := now.Format("2006-01-02")
	root := filepath.Join(home, "backups")
	dir := filepath.Join(root, day)

	if completed, ok, err := completedBackupPath(dir); err != nil {
		return "", false, err
	} else if ok {
		_ = completed
		return "", false, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("mkdir: %w", err)
	}

	filename := fmt.Sprintf("library-%s.db", now.Format("150405"))
	dst, err := createPublishedBackup(ctx, db, dir, filename)
	if err != nil {
		return "", false, err
	}
	return dst, true, nil
}

// RunPreMigration snapshots the current database before applying targetVersion.
// The same completion marker used by daily backups makes it discoverable by
// the startup recovery screen.
func RunPreMigration(ctx context.Context, db *sql.DB, home string, targetVersion int, now time.Time) (string, error) {
	dir := filepath.Join(home, "backups", now.Format("2006-01-02"))
	filename := fmt.Sprintf("library-pre-migration-v%04d-%s.db", targetVersion, now.Format("150405"))
	return createPublishedBackup(ctx, db, dir, filename)
}

// RunManualRecovery creates a complete, versioned SQLite library snapshot.
// Unlike the project Markdown export, this archive preserves every table and
// can be restored by the startup recovery screen.
func RunManualRecovery(ctx context.Context, db *sql.DB, home string, now time.Time) (ManualRecoveryResult, error) {
	dir := filepath.Join(home, "backups", "recovery-"+now.Format("20060102-150405.000"))
	filename := "library.linetta"
	path, err := createPublishedBackup(ctx, db, dir, filename)
	if err != nil {
		return ManualRecoveryResult{}, err
	}
	manifest, err := json.Marshal(recoveryManifest{
		Format:        "linetta-library-backup",
		FormatVersion: RecoveryFormatVersion,
		Database:      filename,
		CreatedAt:     now.UnixMilli(),
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		return ManualRecoveryResult{}, fmt.Errorf("marshal recovery manifest: %w", err)
	}
	marker := filepath.Join(dir, ".complete")
	markerTmp := marker + ".tmp"
	if err := os.WriteFile(markerTmp, append(manifest, '\n'), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return ManualRecoveryResult{}, fmt.Errorf("write recovery manifest: %w", err)
	}
	if err := os.Rename(markerTmp, marker); err != nil {
		_ = os.RemoveAll(dir)
		return ManualRecoveryResult{}, fmt.Errorf("publish recovery manifest: %w", err)
	}
	return ManualRecoveryResult{Path: path, FormatVersion: RecoveryFormatVersion}, nil
}

func createPublishedBackup(ctx context.Context, db *sql.DB, dir, filename string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	dst := filepath.Join(dir, filename)
	tmp := dst + ".tmp"
	_ = os.Remove(tmp)
	// VACUUM INTO accepts a single-quoted string literal; sanitize is not strictly
	// needed (we control the path) but escape any embedded single quotes.
	stmt := "VACUUM INTO '" + escapeSQLString(tmp) + "'"
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("VACUUM INTO: %w", err)
	}
	info, err := os.Stat(tmp)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(tmp)
		if err != nil {
			return "", fmt.Errorf("stat backup temp file: %w", err)
		}
		return "", errors.New("backup temp file is empty")
	}
	if err := verifyBackup(ctx, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("verify backup: %w", err)
	}
	file, err := os.OpenFile(tmp, os.O_RDWR, 0)
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("open backup for sync: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("sync backup: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("close backup after sync: %w", err)
	}
	// A prior attempt may have published the database but failed before the
	// completion marker. Replace that incomplete artifact on retry.
	_ = os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("publish backup: %w", err)
	}
	marker := filepath.Join(dir, ".complete")
	markerTmp := marker + ".tmp"
	if err := os.WriteFile(markerTmp, []byte(filename+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write backup marker: %w", err)
	}
	if err := os.Rename(markerTmp, marker); err != nil {
		_ = os.Remove(markerTmp)
		return "", fmt.Errorf("publish backup marker: %w", err)
	}
	return dst, nil
}

func verifyBackup(ctx context.Context, path string) error {
	slashPath := filepath.ToSlash(path)
	if volume := filepath.VolumeName(path); volume != "" && !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	dsn := (&url.URL{Scheme: "file", Path: slashPath, RawQuery: "mode=ro"}).String()
	checkDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer checkDB.Close()
	checkDB.SetMaxOpenConns(1)

	rows, err := checkDB.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return fmt.Errorf("quick_check: %w", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		found = true
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("scan quick_check: %w", err)
		}
		if result != "ok" {
			return fmt.Errorf("quick_check: %s", result)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("quick_check rows: %w", err)
	}
	if !found {
		return errors.New("quick_check returned no result")
	}
	return nil
}

func completedBackupPath(dir string) (string, bool, error) {
	marker := filepath.Join(dir, ".complete")
	raw, err := os.ReadFile(marker)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read backup marker: %w", err)
	}
	filename := strings.TrimSpace(string(raw))
	if filename == "" || filepath.Base(filename) != filename || filepath.Ext(filename) != ".db" {
		return "", false, nil
	}
	path := filepath.Join(dir, filename)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat completed backup: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return "", false, nil
	}
	return path, true, nil
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
