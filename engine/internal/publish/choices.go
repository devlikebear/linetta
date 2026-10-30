package publish

import (
	"context"
	"database/sql"
	"errors"
)

// The writer's choices about what goes to the site beside the episodes
// (publish_entities, publish_covers). Unlike the work and episode rows they
// are not a copy of the site, so Clear and Forget leave them alone.

// SheetPublished reports whether the writer turned on publishing for a sheet.
func (r *StateRepo) SheetPublished(ctx context.Context, entityID string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM publish_entities WHERE entity_id = ?`, entityID).Scan(&n)
	return n > 0, err
}

// SetSheetPublished turns publishing for one sheet on or off.
func (r *StateRepo) SetSheetPublished(ctx context.Context, projectID, entityID string, on bool, now int64) error {
	if !on {
		_, err := r.db.ExecContext(ctx, `DELETE FROM publish_entities WHERE entity_id = ?`, entityID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO publish_entities (entity_id, project_id, created_at) VALUES (?, ?, ?)
ON CONFLICT(entity_id) DO NOTHING`, entityID, projectID, now)
	return err
}

// PublishedSheets returns the ids of a work's sheets the writer turned on.
func (r *StateRepo) PublishedSheets(ctx context.Context, projectID string) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT entity_id FROM publish_entities WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// Cover returns the gallery picture chosen as the work's cover, if any.
func (r *StateRepo) Cover(ctx context.Context, projectID string) (string, bool, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `SELECT illustration_id FROM publish_covers WHERE project_id = ?`, projectID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

// SetCover chooses a gallery picture as the cover; an empty id clears it.
func (r *StateRepo) SetCover(ctx context.Context, projectID, illustrationID string, now int64) error {
	if illustrationID == "" {
		_, err := r.db.ExecContext(ctx, `DELETE FROM publish_covers WHERE project_id = ?`, projectID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO publish_covers (project_id, illustration_id, updated_at) VALUES (?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET illustration_id = excluded.illustration_id, updated_at = excluded.updated_at`,
		projectID, illustrationID, now)
	return err
}
