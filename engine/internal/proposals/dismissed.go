package proposals

import (
	"context"
	"fmt"
	"strings"
)

// DismissedName is a name the writer turned down (#387): rejecting a proposal
// to create it puts it here, and an automatic tidy-up may not offer it again.
type DismissedName struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	DismissedAt int64  `json:"dismissed_at"`
}

// rememberDismissed records the name a rejected create_entity item proposed.
func (s *Service) rememberDismissed(ctx context.Context, tx dbTx, now int64, projectID string, it Item) error {
	if it.Op.Type != "create_entity" {
		return nil
	}
	normalized := normalizeForDedupe(it.Op.Name)
	if normalized == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO proposal_dismissed_names (project_id, kind, normalized, name, dismissed_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (project_id, kind, normalized) DO UPDATE SET name = excluded.name, dismissed_at = excluded.dismissed_at`,
		projectID, it.Op.Kind, normalized, strings.TrimSpace(it.Op.Name), now)
	return err
}

// Dismissed lists the names the writer turned down in a work, newest first.
func (s *Service) Dismissed(ctx context.Context, projectID string) ([]DismissedName, error) {
	rows, err := s.store.DB().QueryContext(ctx, `
SELECT kind, name, dismissed_at FROM proposal_dismissed_names
 WHERE project_id = ? ORDER BY dismissed_at DESC, name ASC`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DismissedName{}
	for rows.Next() {
		var d DismissedName
		if err := rows.Scan(&d.Kind, &d.Name, &d.DismissedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ForgetDismissed lets a turned-down name be proposed again. Forgetting a
// name that is not there is not an error.
func (s *Service) ForgetDismissed(ctx context.Context, projectID, kind, name string) error {
	_, err := s.store.DB().ExecContext(ctx, `
DELETE FROM proposal_dismissed_names WHERE project_id = ? AND kind = ? AND normalized = ?`,
		strings.TrimSpace(projectID), strings.TrimSpace(kind), normalizeForDedupe(name))
	if err == nil {
		s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: strings.TrimSpace(projectID), Kinds: []string{"proposals"}})
	}
	return err
}

// dismissedSet is a work's turned-down names, keyed kind + "\x00" + normalized.
func (s *Service) dismissedSet(ctx context.Context, projectID string) (map[string]string, error) {
	rows, err := s.store.DB().QueryContext(ctx, `
SELECT kind, normalized, name FROM proposal_dismissed_names WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var kind, normalized, name string
		if err := rows.Scan(&kind, &normalized, &name); err != nil {
			return nil, err
		}
		out[kind+"\x00"+normalized] = name
	}
	return out, rows.Err()
}

func dismissedMessage(num int, name string) string {
	return fmt.Sprintf("item %d: the writer turned %q down before; leave it out of a tidy-up batch", num, name)
}
