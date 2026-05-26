package snapshot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when no snapshot exists for a query.
var ErrNotFound = errors.New("snapshot not found")

// Repo persists node_snapshots rows.
type Repo struct {
	s *store.Store
}

// NewRepo returns a Repo backed by the given Store.
func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create inserts a snapshot and returns it with a generated id.
func (r *Repo) Create(ctx context.Context, nodeID, doc, reason string, now int64) (Snapshot, error) {
	id := uuid.NewString()
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO node_snapshots (id, node_id, content_doc, reason, created_at)
VALUES (?, ?, ?, ?, ?)`, id, nodeID, doc, reason, now); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{ID: id, NodeID: nodeID, ContentDoc: doc, Reason: reason, CreatedAt: now}, nil
}

// LatestForNode returns the most recent snapshot for the node (any reason).
func (r *Repo) LatestForNode(ctx context.Context, nodeID string) (Snapshot, error) {
	row := r.s.DB().QueryRowContext(ctx, `
SELECT id, node_id, content_doc, reason, created_at
  FROM node_snapshots
 WHERE node_id = ?
 ORDER BY created_at DESC
 LIMIT 1`, nodeID)
	var s Snapshot
	if err := row.Scan(&s.ID, &s.NodeID, &s.ContentDoc, &s.Reason, &s.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, err
	}
	return s, nil
}

// Entry is a thin summary of a snapshot for the timeline UI. doc_preview is the
// first 200 plaintext characters (mention atoms rendered as @label, paragraph
// boundaries as \n).
type Entry struct {
	ID         string `json:"id"`
	Reason     string `json:"reason"`
	CreatedAt  int64  `json:"created_at"`
	DocPreview string `json:"doc_preview"`
}

// ListForNode returns every snapshot for the node ordered newest-first.
// doc_preview is computed in Go (cannot do it inline in SQL for Tiptap JSON).
func (r *Repo) ListForNode(ctx context.Context, nodeID string) ([]Entry, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT id, content_doc, reason, created_at
  FROM node_snapshots
 WHERE node_id = ?
 ORDER BY created_at DESC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var (
			id, doc, reason string
			createdAt       int64
		)
		if err := rows.Scan(&id, &doc, &reason, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, Entry{
			ID:         id,
			Reason:     reason,
			CreatedAt:  createdAt,
			DocPreview: trimRunes(plaintextFromDoc(doc), 200),
		})
	}
	return out, rows.Err()
}

// GetByID returns the full snapshot row.
func (r *Repo) GetByID(ctx context.Context, id string) (Snapshot, error) {
	row := r.s.DB().QueryRowContext(ctx, `
SELECT id, node_id, content_doc, reason, created_at
  FROM node_snapshots
 WHERE id = ?`, id)
	var s Snapshot
	if err := row.Scan(&s.ID, &s.NodeID, &s.ContentDoc, &s.Reason, &s.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, err
	}
	return s, nil
}

// plaintextFromDoc walks the Tiptap doc and concatenates text. Mentions render
// as @label; paragraph/heading/blockquote insert "\n". Same shape as
// ai.docToPlainText but inlined here to avoid an import cycle.
func plaintextFromDoc(raw string) string {
	if raw == "" {
		return ""
	}
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return ""
	}
	var sb strings.Builder
	walkPlaintext(v, &sb)
	return sb.String()
}

func walkPlaintext(v interface{}, sb *strings.Builder) {
	switch t := v.(type) {
	case map[string]interface{}:
		kind, _ := t["type"].(string)
		if kind == "mention" {
			attrs, _ := t["attrs"].(map[string]interface{})
			if l, _ := attrs["label"].(string); l != "" {
				sb.WriteString("@")
				sb.WriteString(l)
			}
			return
		}
		if kind == "text" {
			if s, ok := t["text"].(string); ok {
				sb.WriteString(s)
			}
			return
		}
		if content, ok := t["content"].([]interface{}); ok {
			for _, c := range content {
				walkPlaintext(c, sb)
			}
		}
		if kind == "paragraph" || kind == "heading" || kind == "blockquote" {
			sb.WriteString("\n")
		}
	case []interface{}:
		for _, c := range t {
			walkPlaintext(c, sb)
		}
	}
}

func trimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// LatestAutosaveTime returns (created_at, true) of the most recent autosave
// for the node, or (0, false) if none.
func (r *Repo) LatestAutosaveTime(ctx context.Context, nodeID string) (int64, bool, error) {
	var t sql.NullInt64
	err := r.s.DB().QueryRowContext(ctx, `
SELECT MAX(created_at) FROM node_snapshots
 WHERE node_id = ? AND reason = 'autosave'`, nodeID).Scan(&t)
	if err != nil {
		return 0, false, err
	}
	if !t.Valid {
		return 0, false, nil
	}
	return t.Int64, true, nil
}
