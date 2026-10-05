// Package visual keeps the writer's character designs with the library backup.
package visual

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// Limits on reference images. They are BLOBs in library.db, which every daily
// backup copies whole, so an unbounded upload is an unbounded backup.
const (
	MaxImageBytes      = 5 * 1024 * 1024
	MaxImagesPerEntity = 8
)

var (
	ErrEntityNotFound       = errors.New("entity not found")
	ErrProjectNotFound      = errors.New("project not found")
	ErrImageTooLarge        = errors.New("reference image exceeds 5 MiB")
	ErrUnsupportedImageType = errors.New("unsupported or mismatched image type")
	ErrTooManyImages        = errors.New("at most 8 reference images per entity")
	ErrImageNotFound        = errors.New("reference image not found")
)

// Sheet is one character's fixed appearance — the words an illustration
// prompt repeats every time the character is drawn (#154).
type Sheet struct {
	EntityID  string `json:"entity_id"`
	AgeRange  string `json:"age_range"`
	Build     string `json:"build"`
	Hair      string `json:"hair"`
	Outfit    string `json:"outfit"`
	Signature string `json:"signature"`
	Palette   string `json:"palette"`
	Notes     string `json:"notes"`
	UpdatedAt int64  `json:"updated_at"`
}

// ImageMeta describes a stored reference image without its bytes.
type ImageMeta struct {
	ID        string `json:"id"`
	EntityID  string `json:"entity_id"`
	MIME      string `json:"mime"`
	Caption   string `json:"caption"`
	ByteSize  int    `json:"byte_size"`
	Ordinal   int    `json:"ordinal"`
	CreatedAt int64  `json:"created_at"`
}

// Image is a reference image with its bytes.
type Image struct {
	ImageMeta
	Data []byte `json:"-"`
}

// ArtStyle is the work-wide illustration guide: the drawing style every
// picture shares and the negative prompt every picture avoids.
type ArtStyle struct {
	ProjectID      string `json:"project_id"`
	Style          string `json:"style"`
	NegativePrompt string `json:"negative_prompt"`
	UpdatedAt      int64  `json:"updated_at"`
}

// Repo persists sheets, reference images and art styles in SQLite.
type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

type scanner interface{ Scan(...any) error }

const sheetColumns = "entity_id, age_range, build, hair, outfit, signature, palette, notes, updated_at"

func scanSheet(row scanner) (Sheet, error) {
	var s Sheet
	err := row.Scan(&s.EntityID, &s.AgeRange, &s.Build, &s.Hair, &s.Outfit, &s.Signature, &s.Palette, &s.Notes, &s.UpdatedAt)
	return s, err
}

// GetSheet gives the editor an empty draft until the writer saves a design.
func (r *Repo) GetSheet(ctx context.Context, entityID string) (Sheet, error) {
	s, err := scanSheet(r.s.DB().QueryRowContext(ctx, "SELECT "+sheetColumns+" FROM entity_visuals WHERE entity_id = ?", entityID))
	if errors.Is(err, sql.ErrNoRows) {
		return Sheet{EntityID: entityID}, nil
	}
	return s, err
}

// SetSheet upserts a character's sheet, trimming every field.
func (r *Repo) SetSheet(ctx context.Context, now int64, s Sheet) (Sheet, error) {
	var exists bool
	if err := r.s.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM entities WHERE id = ?)", s.EntityID).Scan(&exists); err != nil {
		return Sheet{}, err
	}
	if !exists {
		return Sheet{}, ErrEntityNotFound
	}
	s.AgeRange = strings.TrimSpace(s.AgeRange)
	s.Build = strings.TrimSpace(s.Build)
	s.Hair = strings.TrimSpace(s.Hair)
	s.Outfit = strings.TrimSpace(s.Outfit)
	s.Signature = strings.TrimSpace(s.Signature)
	s.Palette = strings.TrimSpace(s.Palette)
	s.Notes = strings.TrimSpace(s.Notes)
	s.UpdatedAt = now
	_, err := r.s.DB().ExecContext(ctx, `INSERT INTO entity_visuals (`+sheetColumns+`) VALUES (?,?,?,?,?,?,?,?,?)
 ON CONFLICT(entity_id) DO UPDATE SET age_range=excluded.age_range, build=excluded.build, hair=excluded.hair, outfit=excluded.outfit, signature=excluded.signature, palette=excluded.palette, notes=excluded.notes, updated_at=excluded.updated_at`,
		s.EntityID, s.AgeRange, s.Build, s.Hair, s.Outfit, s.Signature, s.Palette, s.Notes, s.UpdatedAt)
	return s, err
}

// ListSheetsByProject returns every saved sheet in a work, keyed by entity id.
func (r *Repo) ListSheetsByProject(ctx context.Context, projectID string) (map[string]Sheet, error) {
	rows, err := r.s.DB().QueryContext(ctx, "SELECT "+sheetColumns+" FROM entity_visuals WHERE entity_id IN (SELECT id FROM entities WHERE project_id = ?)", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Sheet{}
	for rows.Next() {
		s, err := scanSheet(rows)
		if err != nil {
			return nil, err
		}
		out[s.EntityID] = s
	}
	return out, rows.Err()
}

const imageColumns = "id, entity_id, mime, caption, byte_size, ordinal, created_at"

func scanMeta(row scanner) (ImageMeta, error) {
	var m ImageMeta
	err := row.Scan(&m.ID, &m.EntityID, &m.MIME, &m.Caption, &m.ByteSize, &m.Ordinal, &m.CreatedAt)
	return m, err
}

// AddImage stores a reference image. The declared MIME type must be one of the
// four browsers and image models all read, and must match what the bytes
// actually are — a mislabelled file would be served back under the wrong type.
func (r *Repo) AddImage(ctx context.Context, now int64, entityID, mime, caption string, data []byte) (ImageMeta, error) {
	if len(data) > MaxImageBytes {
		return ImageMeta{}, ErrImageTooLarge
	}
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
	default:
		return ImageMeta{}, ErrUnsupportedImageType
	}
	if len(data) == 0 || http.DetectContentType(data) != mime {
		return ImageMeta{}, ErrUnsupportedImageType
	}
	// Count and insert share a transaction so simultaneous uploads cannot exceed the limit.
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return ImageMeta{}, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM entities WHERE id = ?)", entityID).Scan(&exists); err != nil {
		return ImageMeta{}, err
	}
	if !exists {
		return ImageMeta{}, ErrEntityNotFound
	}
	var count, ordinal int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(MAX(ordinal)+1,0) FROM entity_reference_images WHERE entity_id = ?", entityID).Scan(&count, &ordinal); err != nil {
		return ImageMeta{}, err
	}
	if count >= MaxImagesPerEntity {
		return ImageMeta{}, ErrTooManyImages
	}
	m := ImageMeta{ID: uuid.NewString(), EntityID: entityID, MIME: mime, Caption: strings.TrimSpace(caption), ByteSize: len(data), Ordinal: ordinal, CreatedAt: now}
	_, err = tx.ExecContext(ctx, "INSERT INTO entity_reference_images ("+imageColumns+",data) VALUES (?,?,?,?,?,?,?,?)", m.ID, m.EntityID, m.MIME, m.Caption, m.ByteSize, m.Ordinal, m.CreatedAt, data)
	if err != nil {
		return ImageMeta{}, err
	}
	return m, tx.Commit()
}

// ListImages omits bytes so opening a sheet does not load every reference.
func (r *Repo) ListImages(ctx context.Context, entityID string) ([]ImageMeta, error) {
	rows, err := r.s.DB().QueryContext(ctx, "SELECT "+imageColumns+" FROM entity_reference_images WHERE entity_id = ? ORDER BY ordinal, id", entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ImageMeta{}
	for rows.Next() {
		m, err := scanMeta(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetImage returns one reference image with its bytes.
func (r *Repo) GetImage(ctx context.Context, id string) (Image, error) {
	var i Image
	err := r.s.DB().QueryRowContext(ctx, "SELECT "+imageColumns+", data FROM entity_reference_images WHERE id = ?", id).Scan(&i.ID, &i.EntityID, &i.MIME, &i.Caption, &i.ByteSize, &i.Ordinal, &i.CreatedAt, &i.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return Image{}, ErrImageNotFound
	}
	return i, err
}

// DeleteImage removes one reference image.
func (r *Repo) DeleteImage(ctx context.Context, id string) error {
	result, err := r.s.DB().ExecContext(ctx, "DELETE FROM entity_reference_images WHERE id = ?", id)
	return imageMutationError(result, err)
}

// UpdateImageCaption rewrites one reference image's caption.
func (r *Repo) UpdateImageCaption(ctx context.Context, id, caption string) (ImageMeta, error) {
	result, err := r.s.DB().ExecContext(ctx, "UPDATE entity_reference_images SET caption = ? WHERE id = ?", strings.TrimSpace(caption), id)
	if err := imageMutationError(result, err); err != nil {
		return ImageMeta{}, err
	}
	return scanMeta(r.s.DB().QueryRowContext(ctx, "SELECT "+imageColumns+" FROM entity_reference_images WHERE id = ?", id))
}

func imageMutationError(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrImageNotFound
	}
	return nil
}

// GetArtStyle returns the work's art style, empty until the writer saves one.
func (r *Repo) GetArtStyle(ctx context.Context, projectID string) (ArtStyle, error) {
	s := ArtStyle{ProjectID: projectID}
	err := r.s.DB().QueryRowContext(ctx, "SELECT style,negative_prompt,updated_at FROM project_art_style WHERE project_id = ?", projectID).Scan(&s.Style, &s.NegativePrompt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	return s, err
}

// SetArtStyle upserts the work's art style.
func (r *Repo) SetArtStyle(ctx context.Context, now int64, s ArtStyle) (ArtStyle, error) {
	var exists bool
	if err := r.s.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM projects WHERE id = ?)", s.ProjectID).Scan(&exists); err != nil {
		return ArtStyle{}, err
	}
	if !exists {
		return ArtStyle{}, ErrProjectNotFound
	}
	s.Style = strings.TrimSpace(s.Style)
	s.NegativePrompt = strings.TrimSpace(s.NegativePrompt)
	s.UpdatedAt = now
	_, err := r.s.DB().ExecContext(ctx, `INSERT INTO project_art_style (project_id,style,negative_prompt,updated_at) VALUES (?,?,?,?) ON CONFLICT(project_id) DO UPDATE SET style=excluded.style,negative_prompt=excluded.negative_prompt,updated_at=excluded.updated_at`, s.ProjectID, s.Style, s.NegativePrompt, s.UpdatedAt)
	return s, err
}

// CharacterVisual is one character as BuildPrompt sees it: the entity's name
// and role plus the sheet and reference images the writer attached.
type CharacterVisual struct {
	Name, Role string
	Sheet      Sheet
	Images     []ImageMeta
}

// Empty reports whether the writer has filled in none of the sheet's fields.
func (s Sheet) Empty() bool {
	return s.AgeRange == "" && s.Build == "" && s.Hair == "" && s.Outfit == "" && s.Signature == "" && s.Palette == "" && s.Notes == ""
}

// Formats BuildPrompt lays out.
const (
	FormatSingle    = "single"
	FormatFourPanel = "four_panel"
)

// BuildPrompt composes an image-generation prompt from what the writer wrote.
//
// It is deterministic and calls no model on purpose: the point of a visual
// sheet is that chapter 3's 한도윤 is chapter 30's 한도윤, and the only way
// to promise that is to send the same words every time. So the writer's text
// goes in verbatim, each field under a fixed English label (image models
// follow labelled attributes far more reliably than one run-on sentence), and
// the only text this function adds is layout: section headings and the
// single / four-panel instruction. An empty field or section is left out
// rather than printed as an empty label, and nothing is invented for a
// negative prompt the writer did not write.
func BuildPrompt(style ArtStyle, chars []CharacterVisual, scene, format string) string {
	var b strings.Builder
	section := func(title, body string) {
		body = strings.TrimSpace(body)
		if body == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(title + ":\n" + body)
	}

	section("Art style", style.Style)

	var cast strings.Builder
	for _, c := range chars {
		cast.WriteString("- " + strings.TrimSpace(c.Name))
		if role := strings.TrimSpace(c.Role); role != "" {
			cast.WriteString(" (" + role + ")")
		}
		cast.WriteString("\n")
		for _, f := range []struct{ label, value string }{
			{"age", c.Sheet.AgeRange},
			{"build", c.Sheet.Build},
			{"hair", c.Sheet.Hair},
			{"outfit", c.Sheet.Outfit},
			{"signature expression/props", c.Sheet.Signature},
			{"color palette", c.Sheet.Palette},
			{"always", c.Sheet.Notes},
		} {
			if v := strings.TrimSpace(f.value); v != "" {
				cast.WriteString("  - " + f.label + ": " + v + "\n")
			}
		}
		if n := len(c.Images); n > 0 {
			cast.WriteString("  - reference images: " + strconv.Itoa(n) + " attached; match them\n")
		}
	}
	section("Characters", cast.String())
	section("Scene", scene)

	if format == FormatFourPanel {
		section("Format", "Four-panel comic (4컷만화), panels 1–4 read in order, in a 2x2 grid or a vertical strip. "+
			"If the scene lists panel beats (1컷, 2컷, ...), give each beat its own panel. "+
			"Every character keeps the same face, hair, outfit and colors in all four panels.")
	} else {
		section("Format", "Single illustration.")
	}
	if len(chars) > 0 {
		section("Keep consistent", "Draw each character exactly as described above and in their reference images; do not redesign them.")
	}
	section("Negative prompt", style.NegativePrompt)
	return b.String()
}
