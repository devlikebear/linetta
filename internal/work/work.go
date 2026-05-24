package work

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/linetta/internal/store"
)

var (
	ErrInvalidInput = errors.New("invalid work input")
	ErrNotFound     = errors.New("work item not found")
)

type WorkStatus string

const (
	WorkStatusActive   WorkStatus = "active"
	WorkStatusArchived WorkStatus = "archived"
)

type EpisodeStatus string

const (
	EpisodeStatusIdea      EpisodeStatus = "idea"
	EpisodeStatusOutlined  EpisodeStatus = "outlined"
	EpisodeStatusDrafting  EpisodeStatus = "drafting"
	EpisodeStatusReviewing EpisodeStatus = "reviewing"
	EpisodeStatusReady     EpisodeStatus = "ready"
	EpisodeStatusPublished EpisodeStatus = "published"
)

type Work struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Genre     string     `json:"genre"`
	Premise   string     `json:"premise"`
	Status    WorkStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type CreateWorkInput struct {
	Title   string `json:"title"`
	Genre   string `json:"genre"`
	Premise string `json:"premise"`
}

type Episode struct {
	ID        string        `json:"id"`
	WorkID    string        `json:"work_id"`
	Title     string        `json:"title"`
	Status    EpisodeStatus `json:"status"`
	Position  int           `json:"position"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type EpisodeBlueprint struct {
	ID             string    `json:"id"`
	WorkID         string    `json:"work_id"`
	EpisodeID      string    `json:"episode_id"`
	Premise        string    `json:"premise"`
	Theme          string    `json:"theme"`
	Situation      string    `json:"situation"`
	MustInclude    string    `json:"must_include"`
	MustAvoid      string    `json:"must_avoid"`
	StructureNotes string    `json:"structure_notes"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SaveBlueprintInput struct {
	Premise        string `json:"premise"`
	Theme          string `json:"theme"`
	Situation      string `json:"situation"`
	MustInclude    string `json:"must_include"`
	MustAvoid      string `json:"must_avoid"`
	StructureNotes string `json:"structure_notes"`
}

type Repository struct {
	db *store.DB
}

func NewRepository(db *store.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateWork(ctx context.Context, input CreateWorkInput) (Work, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return Work{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	now := time.Now().UTC()
	created := Work{
		ID:        newID("work"),
		Title:     title,
		Genre:     strings.TrimSpace(input.Genre),
		Premise:   strings.TrimSpace(input.Premise),
		Status:    WorkStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := r.conn().ExecContext(ctx, `
		INSERT INTO works (id, title, genre, premise, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, created.ID, created.Title, created.Genre, created.Premise, created.Status, formatTime(created.CreatedAt), formatTime(created.UpdatedAt))
	if err != nil {
		return Work{}, err
	}
	return created, nil
}

func (r *Repository) ListWorks(ctx context.Context) ([]Work, error) {
	rows, err := r.conn().QueryContext(ctx, `
		SELECT id, title, genre, premise, status, created_at, updated_at
		FROM works
		ORDER BY updated_at DESC, created_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var works []Work
	for rows.Next() {
		item, err := scanWork(rows)
		if err != nil {
			return nil, err
		}
		works = append(works, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return works, nil
}

func (r *Repository) GetWork(ctx context.Context, id string) (Work, error) {
	row := r.conn().QueryRowContext(ctx, `
		SELECT id, title, genre, premise, status, created_at, updated_at
		FROM works
		WHERE id = ?
	`, strings.TrimSpace(id))
	item, err := scanWork(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, ErrNotFound
	}
	if err != nil {
		return Work{}, err
	}
	return item, nil
}

func (r *Repository) CreateEpisode(ctx context.Context, workID string, title string) (Episode, error) {
	workID = strings.TrimSpace(workID)
	title = strings.TrimSpace(title)
	if workID == "" || title == "" {
		return Episode{}, fmt.Errorf("%w: work id and title are required", ErrInvalidInput)
	}
	if _, err := r.GetWork(ctx, workID); err != nil {
		return Episode{}, err
	}
	position, err := r.nextEpisodePosition(ctx, workID)
	if err != nil {
		return Episode{}, err
	}

	now := time.Now().UTC()
	episode := Episode{
		ID:        newID("episode"),
		WorkID:    workID,
		Title:     title,
		Status:    EpisodeStatusIdea,
		Position:  position,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = r.conn().ExecContext(ctx, `
		INSERT INTO episodes (id, work_id, title, status, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, episode.ID, episode.WorkID, episode.Title, episode.Status, episode.Position, formatTime(episode.CreatedAt), formatTime(episode.UpdatedAt))
	if err != nil {
		return Episode{}, err
	}
	return episode, nil
}

func (r *Repository) ListEpisodes(ctx context.Context, workID string) ([]Episode, error) {
	if _, err := r.GetWork(ctx, workID); err != nil {
		return nil, err
	}
	rows, err := r.conn().QueryContext(ctx, `
		SELECT id, work_id, title, status, position, created_at, updated_at
		FROM episodes
		WHERE work_id = ?
		ORDER BY position ASC, created_at ASC, id ASC
	`, strings.TrimSpace(workID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var episodes []Episode
	for rows.Next() {
		item, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return episodes, nil
}

func (r *Repository) GetEpisode(ctx context.Context, workID, episodeID string) (Episode, error) {
	row := r.conn().QueryRowContext(ctx, `
		SELECT id, work_id, title, status, position, created_at, updated_at
		FROM episodes
		WHERE work_id = ? AND id = ?
	`, strings.TrimSpace(workID), strings.TrimSpace(episodeID))
	item, err := scanEpisode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Episode{}, ErrNotFound
	}
	if err != nil {
		return Episode{}, err
	}
	return item, nil
}

func (r *Repository) UpdateEpisodeStatus(ctx context.Context, workID, episodeID string, status EpisodeStatus) (Episode, error) {
	workID = strings.TrimSpace(workID)
	episodeID = strings.TrimSpace(episodeID)
	if workID == "" || episodeID == "" {
		return Episode{}, fmt.Errorf("%w: work id and episode id are required", ErrInvalidInput)
	}
	if !validEpisodeStatus(status) {
		return Episode{}, fmt.Errorf("%w: unsupported episode status", ErrInvalidInput)
	}
	if _, err := r.GetEpisode(ctx, workID, episodeID); err != nil {
		return Episode{}, err
	}
	now := time.Now().UTC()
	res, err := r.conn().ExecContext(ctx, `
		UPDATE episodes
		SET status = ?, updated_at = ?
		WHERE work_id = ? AND id = ?
	`, status, formatTime(now), workID, episodeID)
	if err != nil {
		return Episode{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Episode{}, err
	}
	if affected == 0 {
		return Episode{}, ErrNotFound
	}
	return r.GetEpisode(ctx, workID, episodeID)
}

func (r *Repository) SaveBlueprint(ctx context.Context, workID, episodeID string, input SaveBlueprintInput) (EpisodeBlueprint, error) {
	workID = strings.TrimSpace(workID)
	episodeID = strings.TrimSpace(episodeID)
	if _, err := r.GetEpisode(ctx, workID, episodeID); err != nil {
		return EpisodeBlueprint{}, err
	}
	input = normalizeBlueprintInput(input)
	now := time.Now().UTC()

	existing, err := r.GetBlueprint(ctx, workID, episodeID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return EpisodeBlueprint{}, err
	}
	if errors.Is(err, ErrNotFound) {
		blueprint := EpisodeBlueprint{
			ID:             newID("blueprint"),
			WorkID:         workID,
			EpisodeID:      episodeID,
			Premise:        input.Premise,
			Theme:          input.Theme,
			Situation:      input.Situation,
			MustInclude:    input.MustInclude,
			MustAvoid:      input.MustAvoid,
			StructureNotes: input.StructureNotes,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		_, err := r.conn().ExecContext(ctx, `
			INSERT INTO episode_blueprints (
				id, work_id, episode_id, premise, theme, situation, must_include, must_avoid, structure_notes, created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, blueprint.ID, blueprint.WorkID, blueprint.EpisodeID, blueprint.Premise, blueprint.Theme, blueprint.Situation, blueprint.MustInclude, blueprint.MustAvoid, blueprint.StructureNotes, formatTime(blueprint.CreatedAt), formatTime(blueprint.UpdatedAt))
		if err != nil {
			return EpisodeBlueprint{}, err
		}
		return blueprint, nil
	}

	_, err = r.conn().ExecContext(ctx, `
		UPDATE episode_blueprints
		SET premise = ?, theme = ?, situation = ?, must_include = ?, must_avoid = ?, structure_notes = ?, updated_at = ?
		WHERE id = ?
	`, input.Premise, input.Theme, input.Situation, input.MustInclude, input.MustAvoid, input.StructureNotes, formatTime(now), existing.ID)
	if err != nil {
		return EpisodeBlueprint{}, err
	}
	return r.GetBlueprint(ctx, workID, episodeID)
}

func (r *Repository) GetBlueprint(ctx context.Context, workID, episodeID string) (EpisodeBlueprint, error) {
	if _, err := r.GetEpisode(ctx, workID, episodeID); err != nil {
		return EpisodeBlueprint{}, err
	}
	row := r.conn().QueryRowContext(ctx, `
		SELECT id, work_id, episode_id, premise, theme, situation, must_include, must_avoid, structure_notes, created_at, updated_at
		FROM episode_blueprints
		WHERE work_id = ? AND episode_id = ?
	`, strings.TrimSpace(workID), strings.TrimSpace(episodeID))
	blueprint, err := scanBlueprint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return EpisodeBlueprint{}, ErrNotFound
	}
	if err != nil {
		return EpisodeBlueprint{}, err
	}
	return blueprint, nil
}

func (r *Repository) nextEpisodePosition(ctx context.Context, workID string) (int, error) {
	var position sql.NullInt64
	if err := r.conn().QueryRowContext(ctx, `SELECT MAX(position) FROM episodes WHERE work_id = ?`, workID).Scan(&position); err != nil {
		return 0, err
	}
	if !position.Valid {
		return 1, nil
	}
	return int(position.Int64) + 1, nil
}

func (r *Repository) conn() *sql.DB {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Conn()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanWork(row scanner) (Work, error) {
	var item Work
	var createdAt, updatedAt string
	if err := row.Scan(&item.ID, &item.Title, &item.Genre, &item.Premise, &item.Status, &createdAt, &updatedAt); err != nil {
		return Work{}, err
	}
	var err error
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Work{}, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Work{}, err
	}
	return item, nil
}

func scanEpisode(row scanner) (Episode, error) {
	var item Episode
	var createdAt, updatedAt string
	if err := row.Scan(&item.ID, &item.WorkID, &item.Title, &item.Status, &item.Position, &createdAt, &updatedAt); err != nil {
		return Episode{}, err
	}
	var err error
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Episode{}, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Episode{}, err
	}
	return item, nil
}

func scanBlueprint(row scanner) (EpisodeBlueprint, error) {
	var item EpisodeBlueprint
	var createdAt, updatedAt string
	if err := row.Scan(
		&item.ID,
		&item.WorkID,
		&item.EpisodeID,
		&item.Premise,
		&item.Theme,
		&item.Situation,
		&item.MustInclude,
		&item.MustAvoid,
		&item.StructureNotes,
		&createdAt,
		&updatedAt,
	); err != nil {
		return EpisodeBlueprint{}, err
	}
	var err error
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return EpisodeBlueprint{}, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return EpisodeBlueprint{}, err
	}
	return item, nil
}

func normalizeBlueprintInput(input SaveBlueprintInput) SaveBlueprintInput {
	input.Premise = strings.TrimSpace(input.Premise)
	input.Theme = strings.TrimSpace(input.Theme)
	input.Situation = strings.TrimSpace(input.Situation)
	input.MustInclude = strings.TrimSpace(input.MustInclude)
	input.MustAvoid = strings.TrimSpace(input.MustAvoid)
	input.StructureNotes = strings.TrimSpace(input.StructureNotes)
	return input
}

func validEpisodeStatus(status EpisodeStatus) bool {
	switch status {
	case EpisodeStatusIdea, EpisodeStatusOutlined, EpisodeStatusDrafting, EpisodeStatusReviewing, EpisodeStatusReady, EpisodeStatusPublished:
		return true
	default:
		return false
	}
}

func newID(prefix string) string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf[:])
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
