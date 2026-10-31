// Package vaultbundle turns one work into the plain contents the private vault
// stores, and back. It knows nothing about encryption or the network: the
// vault package seals the parts and moves them.
//
// A bundle is a manifest and a set of named parts, one for each table and one
// for each picture and work-scoped file, so that changing a scene re-uploads
// the scenes and not the pictures. Rows are written in a canonical order and
// the hash covers only the parts, so the same work hashes the same on every
// device: a work that was just pulled does not look locally changed.
//
// The format outlives the app version that wrote it. FormatVersion is bumped
// for a change to this layout, and code that reads an older one is never
// removed. A manifest also records the schema it was written under, and a
// library older than that refuses it instead of losing columns it cannot hold.
package vaultbundle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FormatVersion is the layout this package writes and the newest it reads.
const FormatVersion = 1

const (
	tablePrefix  = "tables/"
	imagePrefix  = "images/"
	skillPrefix  = "files/skills/"
	memoryPrefix = "files/memory/"
	// imageRefKey marks a row value that lives in an image part.
	imageRefKey = "$part"
)

var (
	// ErrNewerFormat means a newer app wrote this bundle.
	ErrNewerFormat = errors.New("vaultbundle: bundle format is newer than this app")
	// ErrNewerSchema means the bundle holds columns this library cannot store.
	ErrNewerSchema = errors.New("vaultbundle: bundle schema is newer than this library")
	// ErrCorrupt means the parts do not match the manifest.
	ErrCorrupt = errors.New("vaultbundle: bundle is corrupt")
)

// PartInfo describes one part. SHA256 is over the part's plain bytes.
type PartInfo struct {
	Name   string `json:"name"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest lists what a bundle holds. CreatedAt is unix millis.
type Manifest struct {
	Format        int    `json:"format"`
	SchemaVersion int    `json:"schema_version"`
	ProjectID     string `json:"project_id"`
	CreatedAt     int64  `json:"created_at"`
	// ProjectUpdatedAt is projects.updated_at, unix millis.
	ProjectUpdatedAt int64          `json:"project_updated_at"`
	Tables           map[string]int `json:"tables"`
	Parts            []PartInfo     `json:"parts"`
}

// ContentHash identifies the work's contents. It leaves out when the bundle was
// made and under which schema, so only a change to the work changes it.
func (m Manifest) ContentHash() string {
	sum := sha256.New()
	for _, part := range m.Parts {
		fmt.Fprintf(sum, "%s %s\n", part.Name, part.SHA256)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// Bundle is a manifest and the bytes of its parts.
type Bundle struct {
	Manifest Manifest
	Parts    map[string][]byte
}

func hashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (b *Bundle) add(name string, data []byte) {
	b.Parts[name] = data
}

func (b *Bundle) seal() {
	names := make([]string, 0, len(b.Parts))
	for name := range b.Parts {
		names = append(names, name)
	}
	sort.Strings(names)
	b.Manifest.Parts = make([]PartInfo, 0, len(names))
	for _, name := range names {
		data := b.Parts[name]
		b.Manifest.Parts = append(b.Manifest.Parts, PartInfo{Name: name, Size: len(data), SHA256: hashHex(data)})
	}
}

// SchemaVersion is the library's current schema version.
func SchemaVersion(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (int, error) {
	var version sql.NullInt64
	if err := q.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, err
	}
	return int(version.Int64), nil
}

// Export reads one work into a bundle, from a single consistent view of the
// library. home is the Linetta data directory.
func Export(ctx context.Context, db *sql.DB, home, projectID string, now time.Time) (*Bundle, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var updatedAt int64
	err = tx.QueryRowContext(ctx, `SELECT updated_at FROM projects WHERE id = ?`, projectID).Scan(&updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("vaultbundle: project %s not found", projectID)
	}
	if err != nil {
		return nil, err
	}
	schema, err := SchemaVersion(ctx, tx)
	if err != nil {
		return nil, err
	}
	bundle := &Bundle{
		Manifest: Manifest{
			Format:           FormatVersion,
			SchemaVersion:    schema,
			ProjectID:        projectID,
			CreatedAt:        now.UnixMilli(),
			ProjectUpdatedAt: updatedAt,
			Tables:           map[string]int{},
		},
		Parts: map[string][]byte{},
	}
	for _, table := range allTables() {
		count, err := exportTable(ctx, tx, bundle, table, projectID)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", table.Name, err)
		}
		bundle.Manifest.Tables[table.Name] = count
	}
	if err := exportDir(bundle, skillDir(home, projectID), skillPrefix); err != nil {
		return nil, err
	}
	if err := exportDir(bundle, memoryDir(home, projectID), memoryPrefix); err != nil {
		return nil, err
	}
	bundle.seal()
	return bundle, nil
}

func exportTable(ctx context.Context, tx *sql.Tx, bundle *Bundle, table Table, projectID string) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT * FROM `+table.Name+` WHERE `+table.Scope, projectID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	var lines [][]byte
	for rows.Next() {
		raw := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range raw {
			pointers[i] = &raw[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return 0, err
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			if deviceColumns[table.Name][column] {
				continue
			}
			if imageColumns[table.Name] == column {
				data, ok := raw[i].([]byte)
				if !ok {
					return 0, fmt.Errorf("column %s is not a blob", column)
				}
				name := imagePrefix + hashHex(data)
				bundle.add(name, append([]byte(nil), data...))
				row[column] = map[string]string{imageRefKey: name}
				continue
			}
			if text, ok := raw[i].([]byte); ok {
				row[column] = string(text)
				continue
			}
			row[column] = raw[i]
		}
		line, err := json.Marshal(row)
		if err != nil {
			return 0, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	sort.Slice(lines, func(i, j int) bool { return bytes.Compare(lines[i], lines[j]) < 0 })
	bundle.add(tablePrefix+table.Name, bytes.Join(lines, []byte("\n")))
	return len(lines), nil
}

func skillDir(home, projectID string) string {
	return filepath.Join(home, "skills", "works", projectID)
}

func memoryDir(home, projectID string) string {
	return filepath.Join(home, projectID, "memory")
}

// exportDir adds every regular file under dir. A work without the directory
// simply has no such parts.
func exportDir(bundle *Bundle, dir, prefix string) error {
	err := filepath.WalkDir(dir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		bundle.add(prefix+filepath.ToSlash(rel), data)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Validate checks that this library can hold the bundle and that every part
// matches the manifest. Nothing is touched until it passes.
func Validate(bundle *Bundle, librarySchema int) error {
	manifest := bundle.Manifest
	if manifest.Format > FormatVersion {
		return ErrNewerFormat
	}
	if manifest.SchemaVersion > librarySchema {
		return ErrNewerSchema
	}
	if manifest.ProjectID == "" {
		return fmt.Errorf("%w: no project id", ErrCorrupt)
	}
	for _, part := range manifest.Parts {
		data, ok := bundle.Parts[part.Name]
		if !ok {
			return fmt.Errorf("%w: part %s is missing", ErrCorrupt, part.Name)
		}
		if len(data) != part.Size || hashHex(data) != part.SHA256 {
			return fmt.Errorf("%w: part %s does not match", ErrCorrupt, part.Name)
		}
		if err := checkPartName(part.Name); err != nil {
			return err
		}
	}
	if _, ok := bundle.Parts[tablePrefix+"projects"]; !ok {
		return fmt.Errorf("%w: no project row", ErrCorrupt)
	}
	return nil
}

// checkPartName keeps a part from writing outside the work's directories.
func checkPartName(name string) error {
	for _, prefix := range []string{skillPrefix, memoryPrefix} {
		if rel, ok := strings.CutPrefix(name, prefix); ok && (rel == "" || !filepath.IsLocal(filepath.FromSlash(rel)) || path.Clean(rel) != rel) {
			return fmt.Errorf("%w: unsafe path %q", ErrCorrupt, name)
		}
	}
	return nil
}

// Apply replaces the work in the library with the bundle, inside the caller's
// transaction, keeping every id. Validate first. Rows of other works and
// tables that stay on this device are left alone; rebuilding the search index
// and reconciling publish state are the caller's job.
func Apply(ctx context.Context, tx *sql.Tx, bundle *Bundle) error {
	projectID := bundle.Manifest.ProjectID
	library, err := SchemaVersion(ctx, tx)
	if err != nil {
		return err
	}
	for i := len(dependents) - 1; i >= 0; i-- {
		table := dependents[i]
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table.Name+` WHERE `+table.Scope, projectID); err != nil {
			return fmt.Errorf("clear %s: %w", table.Name, err)
		}
	}
	reader := rowReader{ctx: ctx, tx: tx, bundle: bundle, relaxed: bundle.Manifest.SchemaVersion < library}

	projects, err := reader.rows("projects")
	if err != nil {
		return err
	}
	if len(projects) != 1 || projects[0]["id"] != projectID {
		return fmt.Errorf("%w: project row does not match the manifest", ErrCorrupt)
	}
	projects[0]["updated_at"] = bundle.Manifest.ProjectUpdatedAt
	if err := reader.upsert("projects", projects[0]); err != nil {
		return err
	}
	if err := applyEntities(ctx, tx, &reader, projectID); err != nil {
		return err
	}
	if err := applyNodes(ctx, tx, &reader, projectID); err != nil {
		return err
	}
	for _, table := range dependents {
		rows, err := reader.rows(table.Name)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := reader.insert(table.Name, row); err != nil {
				return fmt.Errorf("restore %s: %w", table.Name, err)
			}
		}
	}
	return nil
}

func applyEntities(ctx context.Context, tx *sql.Tx, reader *rowReader, projectID string) error {
	rows, err := reader.rows("entities")
	if err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, row := range rows {
		wanted[asString(row["id"])] = true
	}
	existing, err := idsOf(ctx, tx, "entities", projectID)
	if err != nil {
		return err
	}
	for _, id := range existing {
		if !wanted[id] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM entities WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}
	// UNIQUE(project_id, name) would trip on two characters that swapped names,
	// so kept rows step out of the way first. Their id is unique by definition.
	if _, err := tx.ExecContext(ctx, `UPDATE entities SET name = id WHERE project_id = ?`, projectID); err != nil {
		return err
	}
	for _, row := range rows {
		if err := reader.upsert("entities", row); err != nil {
			return fmt.Errorf("restore entities: %w", err)
		}
	}
	return nil
}

func applyNodes(ctx context.Context, tx *sql.Tx, reader *rowReader, projectID string) error {
	rows, err := reader.rows("nodes")
	if err != nil {
		return err
	}
	ordered, err := parentsFirst(rows)
	if err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, row := range ordered {
		wanted[asString(row["id"])] = true
		if err := reader.upsert("nodes", row); err != nil {
			return fmt.Errorf("restore nodes: %w", err)
		}
	}
	// After the upserts: a node that moved has left its old parent by now, so
	// deleting that parent cannot cascade onto it.
	existing, err := idsOf(ctx, tx, "nodes", projectID)
	if err != nil {
		return err
	}
	for _, id := range existing {
		if !wanted[id] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// parentsFirst orders nodes so parent_id's foreign key holds at every insert.
func parentsFirst(rows []map[string]any) ([]map[string]any, error) {
	children := map[string][]map[string]any{}
	for _, row := range rows {
		parent := asString(row["parent_id"])
		children[parent] = append(children[parent], row)
	}
	ordered := make([]map[string]any, 0, len(rows))
	var walk func(parent string)
	walk = func(parent string) {
		for _, row := range children[parent] {
			ordered = append(ordered, row)
			walk(asString(row["id"]))
		}
	}
	walk("")
	if len(ordered) != len(rows) {
		return nil, fmt.Errorf("%w: nodes do not form a tree", ErrCorrupt)
	}
	return ordered, nil
}

func idsOf(ctx context.Context, tx *sql.Tx, table, projectID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM `+table+` WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}

// rowReader decodes table parts into rows the library can take.
type rowReader struct {
	ctx    context.Context
	tx     *sql.Tx
	bundle *Bundle
	// relaxed is set for a bundle from an older schema, whose columns a later
	// migration may have dropped. From the same schema an unknown column means
	// the bundle is wrong.
	relaxed bool
	columns map[string]map[string]bool
}

func (r *rowReader) columnsOf(table string) (map[string]bool, error) {
	if known, ok := r.columns[table]; ok {
		return known, nil
	}
	rows, err := r.tx.QueryContext(r.ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		known[name] = true
	}
	if r.columns == nil {
		r.columns = map[string]map[string]bool{}
	}
	r.columns[table] = known
	return known, rows.Err()
}

func (r *rowReader) rows(table string) ([]map[string]any, error) {
	data := r.bundle.Parts[tablePrefix+table]
	if len(data) == 0 {
		return nil, nil
	}
	known, err := r.columnsOf(table)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, line := range bytes.Split(data, []byte("\n")) {
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.UseNumber()
		row := map[string]any{}
		if err := decoder.Decode(&row); err != nil {
			return nil, fmt.Errorf("%w: table %s: %v", ErrCorrupt, table, err)
		}
		for column, value := range row {
			if !known[column] {
				if !r.relaxed {
					return nil, fmt.Errorf("%w: table %s has no column %s", ErrCorrupt, table, column)
				}
				delete(row, column)
				continue
			}
			decoded, err := r.decodeValue(value)
			if err != nil {
				return nil, fmt.Errorf("table %s column %s: %w", table, column, err)
			}
			row[column] = decoded
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *rowReader) decodeValue(value any) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		if number, err := strconv.ParseInt(typed.String(), 10, 64); err == nil {
			return number, nil
		}
		return typed.Float64()
	case map[string]any:
		name, _ := typed[imageRefKey].(string)
		data, ok := r.bundle.Parts[name]
		if !ok || !strings.HasPrefix(name, imagePrefix) {
			return nil, fmt.Errorf("%w: image part %q is missing", ErrCorrupt, name)
		}
		return data, nil
	}
	return value, nil
}

func sortedColumns(row map[string]any) []string {
	columns := make([]string, 0, len(row))
	for column := range row {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

func insertSQL(table string, columns []string) string {
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", ")
	return `INSERT INTO ` + table + ` (` + strings.Join(columns, ", ") + `) VALUES (` + marks + `)`
}

func valuesOf(row map[string]any, columns []string) []any {
	values := make([]any, len(columns))
	for i, column := range columns {
		values[i] = row[column]
	}
	return values
}

func (r *rowReader) insert(table string, row map[string]any) error {
	columns := sortedColumns(row)
	_, err := r.tx.ExecContext(r.ctx, insertSQL(table, columns), valuesOf(row, columns)...)
	return err
}

// upsert keeps a row's identity, and so everything pointing at it. A row whose
// id belongs to another work does not match the WHERE and fails the insert's
// constraint instead of being taken over.
func (r *rowReader) upsert(table string, row map[string]any) error {
	columns := sortedColumns(row)
	sets := make([]string, 0, len(columns))
	for _, column := range columns {
		if column != "id" {
			sets = append(sets, column+" = excluded."+column)
		}
	}
	statement := insertSQL(table, columns) + ` ON CONFLICT(id) DO UPDATE SET ` + strings.Join(sets, ", ")
	if table != "projects" {
		statement += ` WHERE ` + table + `.project_id = excluded.project_id`
	}
	result, err := r.tx.ExecContext(r.ctx, statement, valuesOf(row, columns)...)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("%w: %s %v belongs to another work", ErrCorrupt, table, row["id"])
	}
	return nil
}

// ApplyFiles replaces the work's skills and memory with the bundle's. Call it
// after Apply commits: the database is what the writer's text lives in, and a
// file that failed to land leaves a complete work.
func ApplyFiles(home string, bundle *Bundle) error {
	projectID := bundle.Manifest.ProjectID
	if err := replaceDir(bundle, skillDir(home, projectID), skillPrefix); err != nil {
		return err
	}
	return replaceDir(bundle, memoryDir(home, projectID), memoryPrefix)
}

// replaceDir builds the new directory beside the old one and swaps them, so a
// failure part way leaves the old files in place.
func replaceDir(bundle *Bundle, dir, prefix string) error {
	staging := dir + ".vault-incoming"
	retired := dir + ".vault-retired"
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := os.RemoveAll(retired); err != nil {
		return err
	}
	wrote := false
	for name, data := range bundle.Parts {
		rel, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		if err := checkPartName(name); err != nil {
			return err
		}
		target := filepath.Join(staging, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return err
		}
		wrote = true
	}
	if err := os.Rename(dir, retired); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if wrote {
		if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
			return err
		}
		if err := os.Rename(staging, dir); err != nil {
			return err
		}
	}
	return os.RemoveAll(retired)
}
