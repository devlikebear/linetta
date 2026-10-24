// Package illustration keeps the pictures a writer draws for a work: the
// gallery, and the service that draws them from the character visual sheets.
package illustration

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png" // DecodeConfig and the PNG re-encode
	"net/http"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// Limits on the gallery. Like reference images these are BLOBs in
// library.db, which every daily backup copies whole.
const (
	MaxBytes      = 5 * 1024 * 1024
	MaxPerProject = 200
	// reencodeQuality is the JPEG quality a too-large PNG is re-encoded at.
	reencodeQuality = 90
)

var (
	ErrNotFound        = errors.New("illustration not found")
	ErrProjectNotFound = errors.New("project not found")
	ErrNodeNotFound    = errors.New("source scene not found in this work")
	ErrTooLarge        = errors.New("illustration exceeds 5 MiB")
	ErrBadContent      = errors.New("illustration is not a JPEG, PNG or WebP image")
	ErrTooMany         = errors.New("at most 200 illustrations per work")
)

// Meta describes a stored illustration without its bytes.
type Meta struct {
	ID           string  `json:"id"`
	ProjectID    string  `json:"project_id"`
	SourceNodeID *string `json:"source_node_id,omitempty"`
	Caption      string  `json:"caption"`
	// Prompt is exactly what was sent to the provider.
	Prompt    string `json:"prompt"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	MIME      string `json:"mime"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	ByteSize  int    `json:"byte_size"`
	CreatedAt int64  `json:"created_at"`
}

// Illustration is one picture with its bytes.
type Illustration struct {
	Meta
	Data []byte `json:"-"`
}

// NewInput is what Add stores.
type NewInput struct {
	ProjectID    string
	SourceNodeID string // "" when the picture was not drawn from a scene
	Caption      string
	Prompt       string
	Provider     string
	Model        string
	Data         []byte
}

// Repo persists illustrations in SQLite.
type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

const metaColumns = "id, project_id, source_node_id, caption, prompt, provider, model, mime, width, height, byte_size, created_at"

type scanner interface{ Scan(...any) error }

func scanMeta(row scanner, extra ...any) (Meta, error) {
	var m Meta
	var source sql.NullString
	dest := append([]any{&m.ID, &m.ProjectID, &source, &m.Caption, &m.Prompt, &m.Provider, &m.Model, &m.MIME, &m.Width, &m.Height, &m.ByteSize, &m.CreatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return Meta{}, err
	}
	if source.Valid {
		m.SourceNodeID = &source.String
	}
	return m, nil
}

// normalize checks the bytes are an image a browser shows and returns them
// with their real type and size. A PNG over the limit is re-encoded as JPEG:
// Gemini 2.5 answers in PNG, and a 2K one runs past 5 MiB where the same
// picture as JPEG is well under it.
func normalize(data []byte) ([]byte, string, int, int, error) {
	if len(data) == 0 {
		return nil, "", 0, 0, ErrBadContent
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return nil, "", 0, 0, ErrBadContent
	}
	if len(data) > MaxBytes && mime == "image/png" {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, "", 0, 0, ErrBadContent
		}
		var out bytes.Buffer
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: reencodeQuality}); err != nil {
			return nil, "", 0, 0, err
		}
		data, mime = out.Bytes(), "image/jpeg"
	}
	if len(data) > MaxBytes {
		return nil, "", 0, 0, ErrTooLarge
	}
	// WebP has no decoder in the standard library; its size stays 0.
	width, height := 0, 0
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		width, height = cfg.Width, cfg.Height
	}
	return data, mime, width, height, nil
}

// Add stores a new illustration in a work's gallery.
func (r *Repo) Add(ctx context.Context, now int64, in NewInput) (Meta, error) {
	data, mime, width, height, err := normalize(in.Data)
	if err != nil {
		return Meta{}, err
	}
	// Count and insert share a transaction so two drawings finishing at once
	// cannot both take the last slot.
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Meta{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM projects WHERE id = ?)", in.ProjectID).Scan(&exists); err != nil {
		return Meta{}, err
	}
	if !exists {
		return Meta{}, ErrProjectNotFound
	}
	var source any
	if in.SourceNodeID != "" {
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM nodes WHERE id = ? AND project_id = ?)", in.SourceNodeID, in.ProjectID).Scan(&exists); err != nil {
			return Meta{}, err
		}
		if !exists {
			return Meta{}, ErrNodeNotFound
		}
		source = in.SourceNodeID
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM illustrations WHERE project_id = ?", in.ProjectID).Scan(&count); err != nil {
		return Meta{}, err
	}
	if count >= MaxPerProject {
		return Meta{}, ErrTooMany
	}
	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO illustrations ("+metaColumns+", data) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
		id, in.ProjectID, source, strings.TrimSpace(in.Caption), in.Prompt, in.Provider, in.Model, mime, width, height, len(data), now, data,
	); err != nil {
		return Meta{}, err
	}
	m, err := scanMeta(tx.QueryRowContext(ctx, "SELECT "+metaColumns+" FROM illustrations WHERE id = ?", id))
	if err != nil {
		return Meta{}, err
	}
	return m, tx.Commit()
}

// ListByProject returns a work's gallery, newest first, without bytes.
func (r *Repo) ListByProject(ctx context.Context, projectID string) ([]Meta, error) {
	rows, err := r.s.DB().QueryContext(ctx, "SELECT "+metaColumns+" FROM illustrations WHERE project_id = ? ORDER BY created_at DESC, id", projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Meta{}
	for rows.Next() {
		m, err := scanMeta(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Usage is how many illustrations a work holds and how many bytes they take
// in library.db, for the gallery to say what the backups carry.
func (r *Repo) Usage(ctx context.Context, projectID string) (count int, bytes int64, err error) {
	err = r.s.DB().QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(byte_size), 0) FROM illustrations WHERE project_id = ?", projectID).Scan(&count, &bytes)
	return count, bytes, err
}

// Get returns one illustration with its bytes.
func (r *Repo) Get(ctx context.Context, id string) (Illustration, error) {
	var i Illustration
	m, err := scanMeta(r.s.DB().QueryRowContext(ctx, "SELECT "+metaColumns+", data FROM illustrations WHERE id = ?", id), &i.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return Illustration{}, ErrNotFound
	}
	i.Meta = m
	return i, err
}

// UpdateCaption rewrites one illustration's gallery caption.
func (r *Repo) UpdateCaption(ctx context.Context, id, caption string) (Meta, error) {
	result, err := r.s.DB().ExecContext(ctx, "UPDATE illustrations SET caption = ? WHERE id = ?", strings.TrimSpace(caption), id)
	if err := mutationError(result, err); err != nil {
		return Meta{}, err
	}
	return scanMeta(r.s.DB().QueryRowContext(ctx, "SELECT "+metaColumns+" FROM illustrations WHERE id = ?", id))
}

// Delete removes one illustration.
func (r *Repo) Delete(ctx context.Context, id string) error {
	result, err := r.s.DB().ExecContext(ctx, "DELETE FROM illustrations WHERE id = ?", id)
	return mutationError(result, err)
}

func mutationError(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
