package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// sqliteConstraintForeignKey is SQLITE_CONSTRAINT_FOREIGNKEY, the numeric
// result code for a write that would leave a foreign key dangling.
const sqliteConstraintForeignKey = 787

// Slice names some rows of one table.
type Slice struct {
	Table string
	Where string
	Args  []any
}

type tableRows struct {
	table   string
	columns []string
	rows    [][]any
}

// RowSet is a record and everything that hangs off it, as they stood before a
// delete: a foreign-key cascade leaves nothing to undo from, so what a delete
// will take has to be read first. It lives in memory for as long as the undo
// window that holds it.
type RowSet struct {
	s      *Store
	tables []tableRows
}

// Capture reads slices, parents first: Restore inserts in this order, so each
// foreign key's target is back before the row that points at it. The first
// slice is the record itself and the rest are its dependents.
func (s *Store) Capture(ctx context.Context, slices ...Slice) (RowSet, error) {
	out := RowSet{s: s, tables: make([]tableRows, 0, len(slices))}
	for _, slice := range slices {
		t, err := s.captureSlice(ctx, slice)
		if err != nil {
			return RowSet{}, fmt.Errorf("capture %s: %w", slice.Table, err)
		}
		out.tables = append(out.tables, t)
	}
	return out, nil
}

func (s *Store) captureSlice(ctx context.Context, slice Slice) (tableRows, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM `+slice.Table+` WHERE `+slice.Where, slice.Args...)
	if err != nil {
		return tableRows{}, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return tableRows{}, err
	}
	t := tableRows{table: slice.Table, columns: columns}
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return tableRows{}, err
		}
		t.rows = append(t.rows, values)
	}
	return t, rows.Err()
}

// Count reports how many captured rows came from table.
func (rs RowSet) Count(table string) int {
	n := 0
	for _, t := range rs.tables {
		if t.table == table {
			n += len(t.rows)
		}
	}
	return n
}

// Restore puts the rows back in one transaction. A dependent whose other
// parent has gone since (the far end of a relationship, a deleted scene) stays
// gone, because the record is what the writer asked to have back; a refusal on
// the record itself, such as its name having been taken, fails the whole
// restore.
func (rs RowSet) Restore(ctx context.Context) error {
	tx, err := rs.s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, t := range rs.tables {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(t.columns)), ",")
		query := `INSERT INTO ` + t.table + ` (` + strings.Join(t.columns, ",") + `) VALUES (` + placeholders + `)`
		for _, row := range t.rows {
			if _, err := tx.ExecContext(ctx, query, row...); err != nil {
				if i > 0 && isForeignKeyViolation(err) {
					continue
				}
				return fmt.Errorf("restore %s: %w", t.table, err)
			}
		}
	}
	return tx.Commit()
}

func isForeignKeyViolation(err error) bool {
	var coded interface{ Code() int }
	return errors.As(err, &coded) && coded.Code() == sqliteConstraintForeignKey
}
