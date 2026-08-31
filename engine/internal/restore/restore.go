// Package restore merges works out of a library backup into the live library.
//
// The startup recovery screen replaces the whole database file and only exists
// when the engine cannot boot. This package is the other half of #83: while the
// app is running normally, the writer picks a backup, picks a work inside it,
// and gets that work back as a NEW project — the live library is never
// overwritten, so a merge can never damage what they are writing now.
package restore

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// BackupEntry is one restorable snapshot file under $LINETTA_HOME/backups.
type BackupEntry struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"` // daily | pre_migration | recovery
	SizeBytes int64  `json:"size_bytes"`
	CreatedAt int64  `json:"created_at"` // file modification time, unix millis
}

// ListBackups scans home/backups for snapshot files, newest first.
// Layout produced by the backup package:
//
//	backups/YYYY-MM-DD/library-HHMMSS.db                  (daily)
//	backups/YYYY-MM-DD/library-pre-migration-…-HHMMSS.db  (pre-migration)
//	backups/recovery-…/library.linetta                    (manual recovery)
func ListBackups(home string) ([]BackupEntry, error) {
	root := filepath.Join(home, "backups")
	dirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupEntry{}, nil
		}
		return nil, err
	}
	entries := []BackupEntry{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			var kind string
			switch {
			case strings.HasSuffix(name, ".linetta"):
				kind = "recovery"
			case strings.HasPrefix(name, "library-pre-migration") && strings.HasSuffix(name, ".db"):
				kind = "pre_migration"
			case strings.HasPrefix(name, "library-") && strings.HasSuffix(name, ".db"):
				kind = "daily"
			default:
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			entries = append(entries, BackupEntry{
				Path:      filepath.Join(root, d.Name(), name),
				Kind:      kind,
				SizeBytes: info.Size(),
				CreatedAt: info.ModTime().UnixMilli(),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt > entries[j].CreatedAt })
	return entries, nil
}

// ValidateBackupPath rejects paths outside home/backups. RPC callers hand us a
// path string; without this the endpoint would open any file on disk.
func ValidateBackupPath(home, backupPath string) error {
	root, err := filepath.Abs(filepath.Join(home, "backups"))
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(backupPath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("backup path is outside the backups folder")
	}
	return nil
}

// BackupProject is a work inside a backup, enough for the picker UI.
type BackupProject struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	WordCount  int    `json:"word_count"`
	UpdatedAt  int64  `json:"updated_at"`
	ArchivedAt *int64 `json:"archived_at,omitempty"`
}

// openBackupCopy copies the backup into tempDir and opens the copy through the
// normal store path, which migrates it up to the current schema. The original
// backup file is never touched — restoring from an old backup must not mutate
// it. Caller must call the returned cleanup.
func openBackupCopy(ctx context.Context, backupPath, tempDir string) (*store.Store, func(), error) {
	dir, err := os.MkdirTemp(tempDir, "restore-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	dst := filepath.Join(dir, "backup.db")
	if err := copyFile(backupPath, dst); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("copy backup: %w", err)
	}
	st, err := store.Open(ctx, dst)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("open backup copy: %w", err)
	}
	closeAll := func() {
		_ = st.Close()
		cleanup()
	}
	return st, closeAll, nil
}

// PeekProjects lists the works inside a backup without touching the live DB.
func PeekProjects(ctx context.Context, backupPath, tempDir string) ([]BackupProject, error) {
	st, done, err := openBackupCopy(ctx, backupPath, tempDir)
	if err != nil {
		return nil, err
	}
	defer done()
	rows, err := st.DB().QueryContext(ctx,
		`SELECT id, title, word_count, updated_at, archived_at FROM projects ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupProject{}
	for rows.Next() {
		var p BackupProject
		var archived sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Title, &p.WordCount, &p.UpdatedAt, &archived); err != nil {
			return nil, err
		}
		if archived.Valid {
			p.ArchivedAt = &archived.Int64
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// MergeResult reports the project the merge created.
type MergeResult struct {
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
}

// MergeProject copies one work out of the backup into the live library as a
// brand-new project. Every row gets a fresh id, so nothing in the live library
// is ever updated or deleted — the merge is purely additive. Copied tables:
// projects, nodes, entities, mentions, relationships, threads, beats, notes,
// node_snapshots, fact_cards, fact_sources, writing_stats. Library-level
// history (companion transcripts, AI runs, MCP activity) stays with the
// library it belongs to.
func MergeProject(ctx context.Context, live *store.Store, backupPath, tempDir, projectID, titleSuffix string, now time.Time) (MergeResult, error) {
	src, done, err := openBackupCopy(ctx, backupPath, tempDir)
	if err != nil {
		return MergeResult{}, err
	}
	defer done()

	sdb := src.DB()

	// --- read everything from the backup copy first ---
	var (
		title            string
		lastOpenedNodeID sql.NullString
	)
	if err := sdb.QueryRowContext(ctx, `SELECT title, last_opened_node_id FROM projects WHERE id = ?`, projectID).
		Scan(&title, &lastOpenedNodeID); err != nil {
		if err == sql.ErrNoRows {
			return MergeResult{}, fmt.Errorf("project %s not found in backup", projectID)
		}
		return MergeResult{}, err
	}

	newProjectID := uuid.NewString()
	nodeMap := map[string]string{}
	entityMap := map[string]string{}
	threadMap := map[string]string{}
	cardMap := map[string]string{}
	pairMap := map[string]string{}

	newTitle := title + titleSuffix

	tx, err := live.DB().BeginTx(ctx, nil)
	if err != nil {
		return MergeResult{}, err
	}
	defer tx.Rollback()

	nowMillis := now.UnixMilli()

	// projects — last_opened_node_id is remapped after nodes exist.
	if err := copyRows(ctx, sdb, tx, "projects", "id = ?", []any{projectID}, func(row map[string]any) (map[string]any, bool) {
		row["id"] = newProjectID
		row["title"] = newTitle
		row["last_opened_node_id"] = nil
		row["updated_at"] = nowMillis
		return row, true
	}); err != nil {
		return MergeResult{}, fmt.Errorf("copy project: %w", err)
	}

	// nodes in parent-before-child order (parent_id has an FK).
	nodeOrder, err := nodePreOrder(ctx, sdb, projectID)
	if err != nil {
		return MergeResult{}, err
	}
	for _, oldID := range nodeOrder {
		nodeMap[oldID] = uuid.NewString()
	}
	for _, oldID := range nodeOrder {
		if err := copyRows(ctx, sdb, tx, "nodes", "id = ?", []any{oldID}, func(row map[string]any) (map[string]any, bool) {
			row["id"] = nodeMap[asString(row["id"])]
			row["project_id"] = newProjectID
			if p := asString(row["parent_id"]); p != "" {
				row["parent_id"] = nodeMap[p]
			}
			return row, true
		}); err != nil {
			return MergeResult{}, fmt.Errorf("copy nodes: %w", err)
		}
	}

	if err := copyRows(ctx, sdb, tx, "entities", "project_id = ?", []any{projectID}, func(row map[string]any) (map[string]any, bool) {
		newID := uuid.NewString()
		entityMap[asString(row["id"])] = newID
		row["id"] = newID
		row["project_id"] = newProjectID
		return row, true
	}); err != nil {
		return MergeResult{}, fmt.Errorf("copy entities: %w", err)
	}

	if err := copyRows(ctx, sdb, tx, "mentions",
		"node_id IN (SELECT id FROM nodes WHERE project_id = ?)", []any{projectID},
		func(row map[string]any) (map[string]any, bool) {
			n, okN := nodeMap[asString(row["node_id"])]
			e, okE := entityMap[asString(row["entity_id"])]
			if !okN || !okE {
				return nil, false
			}
			row["id"] = uuid.NewString()
			row["node_id"] = n
			row["entity_id"] = e
			return row, true
		}); err != nil {
		return MergeResult{}, fmt.Errorf("copy mentions: %w", err)
	}

	if err := copyRows(ctx, sdb, tx, "relationships", "project_id = ?", []any{projectID}, func(row map[string]any) (map[string]any, bool) {
		from, okF := entityMap[asString(row["from_id"])]
		to, okT := entityMap[asString(row["to_id"])]
		if !okF || !okT {
			return nil, false
		}
		row["id"] = uuid.NewString()
		row["project_id"] = newProjectID
		row["from_id"] = from
		row["to_id"] = to
		if p := asString(row["pair_id"]); p != "" {
			if _, ok := pairMap[p]; !ok {
				pairMap[p] = uuid.NewString()
			}
			row["pair_id"] = pairMap[p]
		}
		return row, true
	}); err != nil {
		return MergeResult{}, fmt.Errorf("copy relationships: %w", err)
	}

	if err := copyRows(ctx, sdb, tx, "threads", "project_id = ?", []any{projectID}, func(row map[string]any) (map[string]any, bool) {
		newID := uuid.NewString()
		threadMap[asString(row["id"])] = newID
		row["id"] = newID
		row["project_id"] = newProjectID
		return row, true
	}); err != nil {
		return MergeResult{}, fmt.Errorf("copy threads: %w", err)
	}

	if err := copyRows(ctx, sdb, tx, "beats",
		"thread_id IN (SELECT id FROM threads WHERE project_id = ?)", []any{projectID},
		func(row map[string]any) (map[string]any, bool) {
			th, ok := threadMap[asString(row["thread_id"])]
			if !ok {
				return nil, false
			}
			row["id"] = uuid.NewString()
			row["thread_id"] = th
			if n := asString(row["node_id"]); n != "" {
				if mapped, ok := nodeMap[n]; ok {
					row["node_id"] = mapped
				} else {
					row["node_id"] = nil
				}
			}
			return row, true
		}); err != nil {
		return MergeResult{}, fmt.Errorf("copy beats: %w", err)
	}

	if err := copyRows(ctx, sdb, tx, "notes",
		"node_id IN (SELECT id FROM nodes WHERE project_id = ?)", []any{projectID},
		func(row map[string]any) (map[string]any, bool) {
			n, ok := nodeMap[asString(row["node_id"])]
			if !ok {
				return nil, false
			}
			row["id"] = uuid.NewString()
			row["node_id"] = n
			return row, true
		}); err != nil {
		return MergeResult{}, fmt.Errorf("copy notes: %w", err)
	}

	if err := copyRows(ctx, sdb, tx, "node_snapshots",
		"node_id IN (SELECT id FROM nodes WHERE project_id = ?)", []any{projectID},
		func(row map[string]any) (map[string]any, bool) {
			n, ok := nodeMap[asString(row["node_id"])]
			if !ok {
				return nil, false
			}
			row["id"] = uuid.NewString()
			row["node_id"] = n
			return row, true
		}); err != nil {
		return MergeResult{}, fmt.Errorf("copy node_snapshots: %w", err)
	}

	if err := copyRows(ctx, sdb, tx, "fact_cards", "project_id = ?", []any{projectID}, func(row map[string]any) (map[string]any, bool) {
		newID := uuid.NewString()
		cardMap[asString(row["id"])] = newID
		row["id"] = newID
		row["project_id"] = newProjectID
		if n := asString(row["node_id"]); n != "" {
			if mapped, ok := nodeMap[n]; ok {
				row["node_id"] = mapped
			} else {
				row["node_id"] = nil
			}
		}
		return row, true
	}); err != nil {
		return MergeResult{}, fmt.Errorf("copy fact_cards: %w", err)
	}

	if err := copyRows(ctx, sdb, tx, "fact_sources",
		"card_id IN (SELECT id FROM fact_cards WHERE project_id = ?)", []any{projectID},
		func(row map[string]any) (map[string]any, bool) {
			c, ok := cardMap[asString(row["card_id"])]
			if !ok {
				return nil, false
			}
			row["id"] = uuid.NewString()
			row["card_id"] = c
			return row, true
		}); err != nil {
		return MergeResult{}, fmt.Errorf("copy fact_sources: %w", err)
	}

	if err := copyRows(ctx, sdb, tx, "writing_stats", "project_id = ?", []any{projectID}, func(row map[string]any) (map[string]any, bool) {
		row["project_id"] = newProjectID
		return row, true
	}); err != nil {
		return MergeResult{}, fmt.Errorf("copy writing_stats: %w", err)
	}

	if lastOpenedNodeID.Valid {
		if mapped, ok := nodeMap[lastOpenedNodeID.String]; ok {
			if _, err := tx.ExecContext(ctx,
				`UPDATE projects SET last_opened_node_id = ? WHERE id = ?`, mapped, newProjectID); err != nil {
				return MergeResult{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return MergeResult{}, err
	}
	return MergeResult{ProjectID: newProjectID, Title: newTitle}, nil
}

// nodePreOrder returns the project's node ids parent-before-child, siblings by
// ordinal, matching the outline's document order.
func nodePreOrder(ctx context.Context, db *sql.DB, projectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, parent_id FROM nodes WHERE project_id = ? ORDER BY ordinal`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	children := map[string][]string{}
	for rows.Next() {
		var id string
		var parent sql.NullString
		if err := rows.Scan(&id, &parent); err != nil {
			return nil, err
		}
		key := ""
		if parent.Valid {
			key = parent.String
		}
		children[key] = append(children[key], id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []string
	var walk func(parent string)
	walk = func(parent string) {
		for _, id := range children[parent] {
			out = append(out, id)
			walk(id)
		}
	}
	walk("")
	return out, nil
}

// copyRows streams matching rows from the backup into the live transaction,
// resolving columns by name so schema growth does not silently drop data:
// both sides sit at the current schema version (the copy was migrated on
// open), so the column set always matches. remap edits one row's values and
// may veto the row by returning false.
func copyRows(ctx context.Context, src *sql.DB, tx *sql.Tx, table, where string, args []any, remap func(map[string]any) (map[string]any, bool)) error {
	rows, err := src.QueryContext(ctx, `SELECT * FROM `+table+` WHERE `+where, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")
	insert := `INSERT INTO ` + table + ` (` + strings.Join(cols, ", ") + `) VALUES (` + placeholders + `)`

	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = normalizeValue(raw[i])
		}
		row, keep := remap(row)
		if !keep {
			continue
		}
		vals := make([]any, len(cols))
		for i, c := range cols {
			vals[i] = row[c]
		}
		if _, err := tx.ExecContext(ctx, insert, vals...); err != nil {
			return err
		}
	}
	return rows.Err()
}

// normalizeValue converts []byte (how modernc/sqlite hands back TEXT) into
// string so remap callbacks can compare and reassign values naturally.
func normalizeValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
