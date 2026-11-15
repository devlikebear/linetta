// Package semantic is meaning search over a work's prose (#384 Phase 2): each
// paragraph embedded on the writer's own machine by a small multilingual
// model, so "누나가 피아노 치던 기억" finds the paragraph about the sister at the
// keys though it shares no word with it. The manuscript never leaves the
// device. The model is downloaded once, when the writer switches it on.
//
// The 2026-11 spike (#384) settled the shape: one vector per paragraph, not
// per 300–600 character chunk (top-10 recall 70% against 49% on the eval
// set), and the pure-Go runtime, at about a fifth of ONNX Runtime's speed,
// for one build on every platform.
package semantic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
)

// ErrUnsupported: this build cannot run the model (the mobile engine, for now).
var ErrUnsupported = errors.New("semantic: meaning search is not available on this device yet")

// Embedder turns texts into unit vectors of one model. Texts carry the
// model's own prefix ("query: ", "passage: "); Passage and Query add it.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Close() error
}

// The e5 family wants to know which side of a search a text is on.
func passage(text string) string { return "passage: " + text }
func query(text string) string   { return "query: " + text }

// Paragraph is one paragraph of a scene, numbered as linetta_read_scene
// numbered=true numbers it.
type Paragraph struct {
	NodeID string
	Para   int
	Text   string
	Hash   string
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}

// paragraphs reads a work's paragraphs back from manuscript_chunks. Blank
// ones are left out: they say nothing to find.
func paragraphs(ctx context.Context, db *sql.DB, projectID string) ([]Paragraph, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT node_id, para_from, text FROM manuscript_chunks WHERE project_id = ? ORDER BY node_id, para_from`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Paragraph
	for rows.Next() {
		var nodeID, text string
		var from int
		if err := rows.Scan(&nodeID, &from, &text); err != nil {
			return nil, err
		}
		for i, p := range manuscript.ChunkParagraphs(text) {
			if strings.TrimSpace(p) == "" {
				continue
			}
			out = append(out, Paragraph{NodeID: nodeID, Para: from + i, Text: p, Hash: hashText(p)})
		}
	}
	return out, rows.Err()
}

// quantize keeps a unit vector in a byte a component: every component of a
// unit vector is within [-1, 1], and ranking by the dot product survives the
// rounding (the spike's paragraph ranks did not move).
func quantize(v []float32) []byte {
	out := make([]byte, len(v))
	for i, x := range v {
		q := math.Round(float64(x) * 127)
		out[i] = byte(int8(max(-127, min(127, q))))
	}
	return out
}

// dot is the similarity of a query vector and a stored one.
func dot(q []float32, v []byte) float32 {
	var s float32
	for i := range min(len(q), len(v)) {
		s += q[i] * float32(int8(v[i]))
	}
	return s / 127
}

// vectors reads every stored vector of a work, by paragraph hash.
func vectors(ctx context.Context, db *sql.DB, projectID, model string) (map[string][]byte, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT text_hash, vec FROM paragraph_vectors WHERE project_id = ? AND model = ?`, projectID, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var h string
		var v []byte
		if err := rows.Scan(&h, &v); err != nil {
			return nil, err
		}
		out[h] = v
	}
	return out, rows.Err()
}
