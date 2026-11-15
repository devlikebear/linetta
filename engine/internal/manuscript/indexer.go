package manuscript

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

// Indexer keeps the app-managed search tables synchronized with leaf
// manuscript prose: manuscript_fts holds each scene's plain text, and
// manuscript_chunks the same scene cut into paragraph-aligned chunks. Both are
// written in one transaction, so a scene is never in one and stale in the
// other.
type Indexer struct {
	db *sql.DB
}

func NewIndexer(db *sql.DB) *Indexer {
	return &Indexer{db: db}
}

// indexWriter inserts one scene's rows into both tables. It does not delete:
// each caller clears what it is about to replace.
type indexWriter struct {
	fts   *sql.Stmt
	chunk *sql.Stmt
}

func newIndexWriter(ctx context.Context, tx *sql.Tx) (*indexWriter, error) {
	fts, err := tx.PrepareContext(ctx, `
INSERT INTO manuscript_fts (plain, node_id, project_id)
VALUES (?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	chunk, err := tx.PrepareContext(ctx, `
INSERT INTO manuscript_chunks (project_id, node_id, para_from, para_to, text, text_hash)
VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		fts.Close()
		return nil, err
	}
	return &indexWriter{fts: fts, chunk: chunk}, nil
}

func (w *indexWriter) Close() {
	w.fts.Close()
	w.chunk.Close()
}

func (w *indexWriter) write(ctx context.Context, projectID, nodeID, doc string) error {
	if _, err := w.fts.ExecContext(ctx, DocToPlainText(doc), nodeID, projectID); err != nil {
		return err
	}
	return w.writeChunks(ctx, projectID, nodeID, Chunks(doc))
}

func (w *indexWriter) writeChunks(ctx context.Context, projectID, nodeID string, chunks []Chunk) error {
	for _, c := range chunks {
		if _, err := w.chunk.ExecContext(ctx, projectID, nodeID, c.ParaFrom, c.ParaTo, c.Text, chunkHash(c.Text)); err != nil {
			return err
		}
	}
	return nil
}

func (i *Indexer) Upsert(ctx context.Context, projectID, nodeID, contentDoc string) error {
	if i == nil || i.db == nil {
		return nil
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := deleteNode(ctx, tx, nodeID); err != nil {
		return err
	}
	w, err := newIndexWriter(ctx, tx)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.write(ctx, projectID, nodeID, contentDoc); err != nil {
		return err
	}
	return tx.Commit()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func deleteNode(ctx context.Context, db execer, nodeID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM manuscript_fts WHERE node_id = ?`, nodeID); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `DELETE FROM manuscript_chunks WHERE node_id = ?`, nodeID)
	return err
}

type leafDoc struct {
	nodeID    string
	projectID string
	doc       string
}

func (i *Indexer) leaves(ctx context.Context, where string, args ...any) ([]leafDoc, error) {
	rows, err := i.db.QueryContext(ctx, `
SELECT id, project_id, COALESCE(content_doc, '')
  FROM nodes
 WHERE kind = 'leaf'`+where+`
 ORDER BY project_id, ordinal`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []leafDoc{}
	for rows.Next() {
		var item leafDoc
		if err := rows.Scan(&item.nodeID, &item.projectID, &item.doc); err != nil {
			return nil, err
		}
		entries = append(entries, item)
	}
	return entries, rows.Err()
}

// replace makes the index in scope (clear, a WHERE clause, and its args) match
// entries, in one transaction. manuscript_fts is cleared and rewritten.
// manuscript_chunks is compared first and only a scene whose chunks differ is
// rewritten: startup runs this over the whole library, and rewriting every
// chunk's text there cost about half again the time the scene index takes.
func (i *Indexer) replace(ctx context.Context, entries []leafDoc, clear string, args ...any) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM manuscript_fts`+clear, args...); err != nil {
		return err
	}
	stored, err := chunkSignatures(ctx, tx, clear, args...)
	if err != nil {
		return err
	}
	w, err := newIndexWriter(ctx, tx)
	if err != nil {
		return err
	}
	defer w.Close()
	for _, item := range entries {
		if _, err := w.fts.ExecContext(ctx, DocToPlainText(item.doc), item.nodeID, item.projectID); err != nil {
			return err
		}
		chunks := Chunks(item.doc)
		sig, had := stored[item.nodeID]
		delete(stored, item.nodeID)
		if had && sig == chunkSignature(chunks) {
			continue
		}
		if had {
			if _, err := tx.ExecContext(ctx, `DELETE FROM manuscript_chunks WHERE node_id = ?`, item.nodeID); err != nil {
				return err
			}
		}
		if err := w.writeChunks(ctx, item.projectID, item.nodeID, chunks); err != nil {
			return err
		}
	}
	// What is left belongs to no scene in scope any more.
	for nodeID := range stored {
		if _, err := tx.ExecContext(ctx, `DELETE FROM manuscript_chunks WHERE node_id = ?`, nodeID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// chunkSignatures reads back, per scene, what chunkSignature would say about
// the chunks stored for it.
func chunkSignatures(ctx context.Context, tx *sql.Tx, clear string, args ...any) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT node_id, para_from, para_to, text_hash
  FROM manuscript_chunks`+clear+`
 ORDER BY node_id, para_from`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var nodeID, hash string
		var from, to int
		if err := rows.Scan(&nodeID, &from, &to, &hash); err != nil {
			return nil, err
		}
		out[nodeID] += signaturePart(from, to, hash)
	}
	return out, rows.Err()
}

func chunkSignature(chunks []Chunk) string {
	var b strings.Builder
	for _, c := range chunks {
		b.WriteString(signaturePart(c.ParaFrom, c.ParaTo, chunkHash(c.Text)))
	}
	return b.String()
}

func signaturePart(from, to int, hash string) string {
	return strconv.Itoa(from) + "-" + strconv.Itoa(to) + ":" + hash + ";"
}

// RebuildAll reconciles the entire derived index from committed leaf content.
// It is safe to run at startup and removes orphaned rows left by an
// interrupted or older best-effort indexing path. It is also what fills
// manuscript_chunks for a library that predates it.
func (i *Indexer) RebuildAll(ctx context.Context) error {
	if i == nil || i.db == nil {
		return nil
	}
	entries, err := i.leaves(ctx, "")
	if err != nil {
		return err
	}
	return i.replace(ctx, entries, "")
}

func (i *Indexer) Delete(ctx context.Context, nodeID string) error {
	if i == nil || i.db == nil {
		return nil
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := deleteNode(ctx, tx, nodeID); err != nil {
		return err
	}
	return tx.Commit()
}

func (i *Indexer) Rebuild(ctx context.Context, projectID string) error {
	if i == nil || i.db == nil {
		return nil
	}
	entries, err := i.leaves(ctx, " AND project_id = ?", projectID)
	if err != nil {
		return err
	}
	return i.replace(ctx, entries, " WHERE project_id = ?", projectID)
}

func (i *Indexer) ProjectRowCount(ctx context.Context, projectID string) (int, error) {
	if i == nil || i.db == nil {
		return 0, nil
	}
	var count int
	err := i.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM manuscript_fts WHERE project_id = ?`, projectID).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return count, err
}
