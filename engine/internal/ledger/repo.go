package ledger

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/google/uuid"
)

// Repo stores the ledger and the part arcs in library.db.
type Repo struct {
	s     *store.Store
	nodes *node.Repo
}

func NewRepo(s *store.Store, nodes *node.Repo) *Repo { return &Repo{s: s, nodes: nodes} }

const entryColumns = `id, project_id, kind, subject, body, status,
       COALESCE(planted_node_id, ''), planted_quote, COALESCE(due_node_id, ''),
       COALESCE(paid_node_id, ''), paid_quote, source, created_at, updated_at`

func scanEntry(row interface{ Scan(...any) error }) (Entry, error) {
	var e Entry
	err := row.Scan(&e.ID, &e.ProjectID, &e.Kind, &e.Subject, &e.Body, &e.Status,
		&e.PlantedNodeID, &e.PlantedQuote, &e.DueNodeID, &e.PaidNodeID, &e.PaidQuote,
		&e.Source, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

// Add records an entry. A quote has to be in the scene it names: the point of
// the ledger is a record that can be checked against the prose.
func (r *Repo) Add(ctx context.Context, now int64, e Entry) (Entry, error) {
	e.Body = strings.TrimSpace(e.Body)
	e.Subject = strings.TrimSpace(e.Subject)
	if e.Status == "" {
		e.Status = StatusOpen
	}
	if e.Source == "" {
		e.Source = SourceWriter
	}
	if !ValidKind(e.Kind) {
		return Entry{}, ErrInvalidKind
	}
	if err := r.validate(ctx, e); err != nil {
		return Entry{}, err
	}
	e.ID = uuid.NewString()
	e.CreatedAt, e.UpdatedAt = now, now
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO story_ledger (id, project_id, kind, subject, body, status, planted_node_id, planted_quote,
                          due_node_id, paid_node_id, paid_quote, source, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.ProjectID, e.Kind, e.Subject, e.Body, e.Status, nullable(e.PlantedNodeID), e.PlantedQuote,
		nullable(e.DueNodeID), nullable(e.PaidNodeID), e.PaidQuote, e.Source, now, now); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// Get returns one entry.
func (r *Repo) Get(ctx context.Context, id string) (Entry, error) {
	e, err := scanEntry(r.s.DB().QueryRowContext(ctx, `SELECT `+entryColumns+` FROM story_ledger WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	return e, err
}

// Update applies a patch.
func (r *Repo) Update(ctx context.Context, now int64, id string, p Patch) (Entry, error) {
	e, err := r.Get(ctx, id)
	if err != nil {
		return Entry{}, err
	}
	for _, f := range []struct {
		to   *string
		from *string
	}{
		{&e.Subject, p.Subject}, {&e.Body, p.Body}, {&e.Status, p.Status},
		{&e.PlantedNodeID, p.PlantedNodeID}, {&e.PlantedQuote, p.PlantedQuote},
		{&e.DueNodeID, p.DueNodeID}, {&e.PaidNodeID, p.PaidNodeID}, {&e.PaidQuote, p.PaidQuote},
	} {
		if f.from != nil {
			*f.to = strings.TrimSpace(*f.from)
		}
	}
	if err := r.validate(ctx, e); err != nil {
		return Entry{}, err
	}
	e.UpdatedAt = now
	if _, err := r.s.DB().ExecContext(ctx, `
UPDATE story_ledger
   SET subject = ?, body = ?, status = ?, planted_node_id = ?, planted_quote = ?,
       due_node_id = ?, paid_node_id = ?, paid_quote = ?, updated_at = ?
 WHERE id = ?`, e.Subject, e.Body, e.Status, nullable(e.PlantedNodeID), e.PlantedQuote,
		nullable(e.DueNodeID), nullable(e.PaidNodeID), e.PaidQuote, now, id); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// Delete removes an entry.
func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.s.DB().ExecContext(ctx, `DELETE FROM story_ledger WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Filter narrows List. Empty fields mean everything.
type Filter struct {
	ProjectID string
	Kinds     []string
	Status    string
}

// List returns a work's entries, oldest first.
func (r *Repo) List(ctx context.Context, f Filter) ([]Entry, error) {
	q := `SELECT ` + entryColumns + ` FROM story_ledger WHERE project_id = ?`
	args := []any{f.ProjectID}
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, f.Status)
	}
	if len(f.Kinds) > 0 {
		q += ` AND kind IN (?` + strings.Repeat(`,?`, len(f.Kinds)-1) + `)`
		for _, k := range f.Kinds {
			args = append(args, k)
		}
	}
	rows, err := r.s.DB().QueryContext(ctx, q+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RecordCutPlans keeps the plans of deleted nodes and returns their labels. It
// is the one record of them: an empty scene has no snapshot, and the undo window
// lasts only as long as the app stays open.
func (r *Repo) RecordCutPlans(ctx context.Context, now int64, cut []node.Node) ([]string, error) {
	var labels []string
	for _, n := range cut {
		plan := strings.TrimSpace(n.Plan)
		if plan == "" {
			continue
		}
		if _, err := r.Add(ctx, now, Entry{ProjectID: n.ProjectID, Kind: KindCutPlan, Subject: n.Label, Body: plan, Source: SourceAgent}); err != nil {
			return labels, err
		}
		labels = append(labels, n.Label)
	}
	return labels, nil
}

func (r *Repo) validate(ctx context.Context, e Entry) error {
	if e.Body == "" {
		return ErrEmptyBody
	}
	if e.Status != StatusOpen && e.Status != StatusPaid && e.Status != StatusDropped {
		return ErrInvalidStatus
	}
	if e.Status == StatusPaid && e.PaidNodeID == "" {
		return ErrPaidNeedsNode
	}
	if err := r.checkNode(ctx, e.ProjectID, e.DueNodeID, ""); err != nil {
		return err
	}
	if err := r.checkNode(ctx, e.ProjectID, e.PlantedNodeID, e.PlantedQuote); err != nil {
		return err
	}
	return r.checkNode(ctx, e.ProjectID, e.PaidNodeID, e.PaidQuote)
}

func (r *Repo) checkNode(ctx context.Context, projectID, nodeID, quote string) error {
	if nodeID == "" {
		return nil
	}
	n, err := r.nodes.Get(ctx, nodeID)
	if err != nil {
		return err
	}
	if n.ProjectID != projectID {
		return ErrWrongWork
	}
	if quote == "" {
		return nil
	}
	if !strings.Contains(strip(storycontext.PlainText(n.ContentDoc)), strip(quote)) {
		return ErrQuoteNotFound
	}
	return nil
}

// strip removes what differs between a scene and a copy of it: the @ on a linked
// name, and how paragraphs were broken.
func strip(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "@", "")), " ")
}

func nullable(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// SetArc replaces a part's arc.
func (r *Repo) SetArc(ctx context.Context, now int64, a Arc) (Arc, error) {
	n, err := r.nodes.Get(ctx, a.NodeID)
	if err != nil {
		return Arc{}, err
	}
	if n.Kind != node.KindContainer {
		return Arc{}, ErrNotAPart
	}
	if a.Tension < 0 || a.Tension > 10 {
		return Arc{}, ErrTension
	}
	a.ProjectID = n.ProjectID
	a.Role, a.Goal = strings.TrimSpace(a.Role), strings.TrimSpace(a.Goal)
	a.UpdatedAt = now
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO part_arcs (node_id, project_id, role, goal, tension, updated_at) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(node_id) DO UPDATE SET role = excluded.role, goal = excluded.goal,
       tension = excluded.tension, updated_at = excluded.updated_at`,
		a.NodeID, a.ProjectID, a.Role, a.Goal, a.Tension, now); err != nil {
		return Arc{}, err
	}
	return a, nil
}

// ListArcs returns a work's part arcs, keyed by part.
func (r *Repo) ListArcs(ctx context.Context, projectID string) (map[string]Arc, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		`SELECT node_id, project_id, role, goal, tension, updated_at FROM part_arcs WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Arc{}
	for rows.Next() {
		var a Arc
		if err := rows.Scan(&a.NodeID, &a.ProjectID, &a.Role, &a.Goal, &a.Tension, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out[a.NodeID] = a
	}
	return out, rows.Err()
}
