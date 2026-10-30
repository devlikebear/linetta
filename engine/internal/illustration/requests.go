package illustration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/illustref"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/google/uuid"
)

// What a requested picture is for, once drawn.
const (
	// PurposeInsert puts the picture at the end of the target scene; the
	// writer moves it where it belongs. An agent does not know where the
	// writer's cursor is, and guessing a paragraph would be worse.
	PurposeInsert = "insert"
	// PurposeRepresentative makes it the picture that stands for the target
	// node, a scene or an episode (Repo.SetRepresentative).
	PurposeRepresentative = "representative"
	// PurposeGallery only keeps it in the gallery.
	PurposeGallery = "gallery"
)

// Request states.
const (
	RequestPending    = "pending"
	RequestGenerating = "generating"
	RequestDone       = "done"
	RequestRejected   = "rejected"
	RequestFailed     = "failed"
	RequestCancelled  = "cancelled"
)

// Who asked, as mcphost names it.
const (
	SourceAgent    = "agent"
	SourceExternal = "external"
)

// MaxPendingRequests bounds the requests waiting in one work, so an agent
// cannot fill the writer's queue faster than they can read it.
const MaxPendingRequests = 20

var (
	ErrRequestNotFound = errors.New("illustration request not found")
	// ErrNotPending: the request was already decided (or is being drawn).
	ErrNotPending = errors.New("illustration request is not waiting for the writer")
	// ErrTooManyPending: MaxPendingRequests are already waiting.
	ErrTooManyPending = errors.New("at most 20 illustration requests can wait in a work")
)

// Request is a picture an agent asked for and what became of it.
type Request struct {
	ID                string   `json:"id"`
	ProjectID         string   `json:"project_id"`
	TargetNodeID      string   `json:"target_node_id,omitempty"`
	Purpose           string   `json:"purpose"`
	Prompt            string   `json:"prompt"`
	CharacterIDs      []string `json:"character_ids"`
	IncludeReferences bool     `json:"include_references"`
	// PriorIllustrationIDs are the earlier gallery pictures the drawing is
	// sent with (GenerateInput.PriorIllustrationIDs).
	PriorIllustrationIDs []string `json:"prior_illustration_ids"`
	Aspect               string   `json:"aspect"`
	Caption              string   `json:"caption"`
	Note                 string   `json:"note"`
	Source               string   `json:"source"`
	Status               string   `json:"status"`
	IllustrationID       string   `json:"illustration_id,omitempty"`
	// PlacedAt is set once the drawing went where Purpose said. A done
	// request without it was drawn but could not be placed (the scene
	// changed under it, say); Error says why.
	PlacedAt  *int64 `json:"placed_at,omitempty"`
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
	DecidedAt *int64 `json:"decided_at,omitempty"`
}

// SubmitInput is a request as an agent leaves it. Prompt is final: an agent
// that only describes the scene has mcphost compose the prompt first.
type SubmitInput struct {
	ProjectID         string
	TargetNodeID      string
	Purpose           string
	Prompt            string
	CharacterIDs      []string
	IncludeReferences bool
	// PriorIllustrationIDs are earlier gallery pictures to send along. Nil
	// asks for SuggestPrior's choice for these characters; empty sends none.
	PriorIllustrationIDs []string
	Aspect               string
	Caption              string
	Note                 string
	Source               string
}

// ApproveEdits are what the writer changed on the card before drawing. Nil
// keeps the request's value.
type ApproveEdits struct {
	Prompt            *string `json:"prompt,omitempty"`
	Aspect            *string `json:"aspect,omitempty"`
	Caption           *string `json:"caption,omitempty"`
	IncludeReferences *bool   `json:"include_references,omitempty"`
	// PriorIllustrationIDs replaces the request's earlier pictures; nil
	// keeps them, empty sends none.
	PriorIllustrationIDs []string `json:"prior_illustration_ids,omitempty"`
}

// Generator draws a picture into the gallery; *Service is one.
type Generator interface {
	Generate(ctx context.Context, in GenerateInput) (Meta, error)
}

// RequestService keeps the requests and draws one when the writer approves.
type RequestService struct {
	repo   *Repo
	gen    Generator
	nodes  *node.Repo
	now    func() int64
	notify func(method string, params any)
}

// NewRequestService wires the service. notify may be nil (tests); it is
// called with "illustrations.changed" after a request changes.
func NewRequestService(repo *Repo, gen Generator, nodes *node.Repo, now func() int64, notify func(string, any)) *RequestService {
	return &RequestService{repo: repo, gen: gen, nodes: nodes, now: now, notify: notify}
}

// ChangedEvent is the illustrations.changed payload. NodeIDs are scenes
// whose text changed (a picture was put in), so an open editor reloads.
type ChangedEvent struct {
	ProjectID string   `json:"project_id"`
	RequestID string   `json:"request_id"`
	Status    string   `json:"status"`
	NodeIDs   []string `json:"node_ids,omitempty"`
}

func (s *RequestService) changed(r Request, nodeIDs []string) {
	if s.notify != nil {
		s.notify("illustrations.changed", ChangedEvent{ProjectID: r.ProjectID, RequestID: r.ID, Status: r.Status, NodeIDs: nodeIDs})
	}
}

// Submit records a request. Nothing is drawn and nothing leaves the device.
func (s *RequestService) Submit(ctx context.Context, in SubmitInput) (Request, error) {
	if err := s.checkSubmit(ctx, &in); err != nil {
		return Request{}, err
	}
	db := s.repo.s.DB()
	var pending int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM illustration_requests WHERE project_id = ? AND status = ?",
		in.ProjectID, RequestPending).Scan(&pending); err != nil {
		return Request{}, err
	}
	if pending >= MaxPendingRequests {
		return Request{}, ErrTooManyPending
	}
	ids, err := json.Marshal(nonNil(in.CharacterIDs))
	if err != nil {
		return Request{}, err
	}
	prior, err := json.Marshal(nonNil(in.PriorIllustrationIDs))
	if err != nil {
		return Request{}, err
	}
	r := Request{
		ID: uuid.NewString(), ProjectID: in.ProjectID, TargetNodeID: in.TargetNodeID, Purpose: in.Purpose,
		Prompt: in.Prompt, CharacterIDs: nonNil(in.CharacterIDs), IncludeReferences: in.IncludeReferences,
		PriorIllustrationIDs: nonNil(in.PriorIllustrationIDs),
		Aspect:               in.Aspect, Caption: in.Caption, Note: in.Note, Source: in.Source,
		Status: RequestPending, CreatedAt: s.now(),
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO illustration_requests
		(id, project_id, target_node_id, purpose, prompt, character_ids_json, include_references, prior_illustration_ids_json, aspect, caption, note, source, status, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.ProjectID, nullString(r.TargetNodeID), r.Purpose, r.Prompt, string(ids), r.IncludeReferences, string(prior),
		r.Aspect, r.Caption, r.Note, r.Source, r.Status, r.CreatedAt); err != nil {
		return Request{}, err
	}
	s.changed(r, nil)
	return r, nil
}

func (s *RequestService) checkSubmit(ctx context.Context, in *SubmitInput) error {
	in.Prompt = strings.TrimSpace(in.Prompt)
	in.Caption = strings.TrimSpace(in.Caption)
	in.Note = strings.TrimSpace(in.Note)
	if in.Prompt == "" {
		return fmt.Errorf("%w: prompt is empty", ErrInvalidInput)
	}
	aspect, err := normalAspect(in.Aspect)
	if err != nil {
		return err
	}
	in.Aspect = aspect
	switch in.Source {
	case SourceAgent, SourceExternal:
	default:
		return fmt.Errorf("%w: source must be agent or external", ErrInvalidInput)
	}
	switch in.Purpose {
	case PurposeInsert, PurposeRepresentative, PurposeGallery:
	default:
		return fmt.Errorf("%w: purpose must be insert, representative or gallery", ErrInvalidInput)
	}
	if in.Purpose == PurposeInsert && in.TargetNodeID == "" {
		return fmt.Errorf("%w: a picture to insert needs the scene's node_id", ErrInvalidInput)
	}
	if in.Purpose == PurposeRepresentative && in.TargetNodeID == "" {
		return fmt.Errorf("%w: a representative picture needs the scene's or episode's node_id", ErrInvalidInput)
	}
	if err := s.repo.checkPlace(ctx, in.ProjectID, in.TargetNodeID, in.CharacterIDs); err != nil {
		return err
	}
	if in.PriorIllustrationIDs == nil {
		suggested, err := s.repo.SuggestPrior(ctx, in.ProjectID, in.CharacterIDs, MaxPriorReferences)
		if err != nil {
			return err
		}
		in.PriorIllustrationIDs = []string{}
		for _, m := range suggested {
			in.PriorIllustrationIDs = append(in.PriorIllustrationIDs, m.ID)
		}
	} else if err := s.repo.checkPrior(ctx, in.ProjectID, in.PriorIllustrationIDs); err != nil {
		return err
	}
	if in.Purpose == PurposeInsert {
		n, err := s.nodes.Get(ctx, in.TargetNodeID)
		if err != nil {
			return err
		}
		if n.Kind != node.KindLeaf {
			return fmt.Errorf("%w: a picture can only be inserted into a scene, not an episode or part", ErrInvalidInput)
		}
	}
	return nil
}

func normalAspect(aspect string) (string, error) {
	switch aspect {
	case "":
		return AspectSquare, nil
	case AspectSquare, AspectPortrait, AspectLandscape:
		return aspect, nil
	}
	return "", fmt.Errorf("%w: aspect must be square, portrait or landscape", ErrInvalidInput)
}

// Get returns one request.
func (s *RequestService) Get(ctx context.Context, id string) (Request, error) {
	row := s.repo.s.DB().QueryRowContext(ctx, `SELECT `+requestColumns+` FROM illustration_requests WHERE id = ?`, id)
	r, err := scanRequest(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, ErrRequestNotFound
	}
	return r, err
}

// List returns a work's requests, newest first. With statuses, only those.
// Requests whose scene or episode is gone are cancelled first.
func (s *RequestService) List(ctx context.Context, projectID string, statuses []string) ([]Request, error) {
	if err := s.cancelOrphans(ctx, projectID); err != nil {
		return nil, err
	}
	q := `SELECT ` + requestColumns + ` FROM illustration_requests WHERE project_id = ?`
	args := []any{projectID}
	if len(statuses) > 0 {
		q += ` AND status IN (?` + strings.Repeat(",?", len(statuses)-1) + `)`
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	q += ` ORDER BY created_at DESC, id`
	rows, err := s.repo.s.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Request{}
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// cancelOrphans marks waiting requests whose target node was deleted.
func (s *RequestService) cancelOrphans(ctx context.Context, projectID string) error {
	_, err := s.repo.s.DB().ExecContext(ctx, `UPDATE illustration_requests
		SET status = ?, decided_at = ?, error = 'the scene or episode was deleted'
		WHERE project_id = ? AND status IN (?, ?) AND target_node_id IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM nodes WHERE nodes.id = illustration_requests.target_node_id)`,
		RequestCancelled, s.now(), projectID, RequestPending, RequestFailed)
	return err
}

// Reject closes a waiting (or failed) request without drawing.
func (s *RequestService) Reject(ctx context.Context, id string) (Request, error) {
	res, err := s.repo.s.DB().ExecContext(ctx, `UPDATE illustration_requests SET status = ?, decided_at = ?
		WHERE id = ? AND status IN (?, ?)`, RequestRejected, s.now(), id, RequestPending, RequestFailed)
	if err != nil {
		return Request{}, err
	}
	r, err := s.Get(ctx, id)
	if err != nil {
		return Request{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return r, ErrNotPending
	}
	s.changed(r, nil)
	return r, nil
}

// Approve draws the request with the writer's edits and then puts the
// picture where the request's purpose says. A failed drawing leaves the
// request failed with the reason, and the writer can approve it again.
//
// The request is claimed (pending or failed → generating) before drawing,
// so a second click, or a second window, draws nothing.
func (s *RequestService) Approve(ctx context.Context, id string, edits ApproveEdits) (Request, error) {
	r, err := s.Get(ctx, id)
	if err != nil {
		return Request{}, err
	}
	if err := s.cancelOrphans(ctx, r.ProjectID); err != nil {
		return Request{}, err
	}
	if edits.Prompt != nil {
		r.Prompt = strings.TrimSpace(*edits.Prompt)
	}
	if edits.Caption != nil {
		r.Caption = strings.TrimSpace(*edits.Caption)
	}
	if edits.IncludeReferences != nil {
		r.IncludeReferences = *edits.IncludeReferences
	}
	if edits.PriorIllustrationIDs != nil {
		r.PriorIllustrationIDs = edits.PriorIllustrationIDs
	}
	prior, err := json.Marshal(nonNil(r.PriorIllustrationIDs))
	if err != nil {
		return Request{}, err
	}
	if edits.Aspect != nil {
		if r.Aspect, err = normalAspect(*edits.Aspect); err != nil {
			return Request{}, err
		}
	}
	if r.Prompt == "" {
		return Request{}, fmt.Errorf("%w: prompt is empty", ErrInvalidInput)
	}

	db := s.repo.s.DB()
	res, err := db.ExecContext(ctx, `UPDATE illustration_requests
		SET status = ?, prompt = ?, aspect = ?, caption = ?, include_references = ?, prior_illustration_ids_json = ?, error = '', decided_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		RequestGenerating, r.Prompt, r.Aspect, r.Caption, r.IncludeReferences, string(prior), s.now(), id, RequestPending, RequestFailed)
	if err != nil {
		return Request{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		current, _ := s.Get(ctx, id)
		return current, ErrNotPending
	}
	r.Status = RequestGenerating
	s.changed(r, nil)

	meta, genErr := s.gen.Generate(ctx, GenerateInput{
		ProjectID: r.ProjectID, SourceNodeID: r.TargetNodeID, Prompt: r.Prompt,
		CharacterIDs: r.CharacterIDs, IncludeReferences: r.IncludeReferences, Aspect: r.Aspect, Caption: r.Caption,
		PriorIllustrationIDs: r.PriorIllustrationIDs,
	})
	if genErr != nil {
		// The drawing may have been cancelled with ctx; the record must
		// still say so.
		if _, err := db.ExecContext(context.WithoutCancel(ctx), `UPDATE illustration_requests SET status = ?, error = ? WHERE id = ?`,
			RequestFailed, genErr.Error(), id); err != nil {
			return Request{}, err
		}
		failed, err := s.Get(context.WithoutCancel(ctx), id)
		if err != nil {
			return Request{}, err
		}
		s.changed(failed, nil)
		return failed, genErr
	}

	var placed []string
	placeErr := s.place(ctx, r, meta)
	var placedAt any
	if placeErr == nil {
		placedAt = s.now()
		if r.Purpose == PurposeInsert {
			placed = []string{r.TargetNodeID}
		}
	}
	errText := ""
	if placeErr != nil {
		errText = "drawn but not placed: " + placeErr.Error()
	}
	if _, err := db.ExecContext(context.WithoutCancel(ctx), `UPDATE illustration_requests
		SET status = ?, illustration_id = ?, placed_at = ?, error = ? WHERE id = ?`,
		RequestDone, meta.ID, placedAt, errText, id); err != nil {
		return Request{}, err
	}
	done, err := s.Get(context.WithoutCancel(ctx), id)
	if err != nil {
		return Request{}, err
	}
	s.changed(done, placed)
	return done, nil
}

// place puts a drawn picture where the request's purpose says.
func (s *RequestService) place(ctx context.Context, r Request, meta Meta) error {
	switch r.Purpose {
	case PurposeInsert:
		return s.appendToScene(ctx, r.TargetNodeID, meta.ID, meta.Caption)
	case PurposeRepresentative:
		_, err := s.repo.SetRepresentative(ctx, s.now(), r.TargetNodeID, meta.ID)
		return err
	}
	return nil
}

// appendToScene adds the picture as the scene's last block. It writes only
// over the version it read, so an autosave landing in between is kept and the
// append is tried again.
func (s *RequestService) appendToScene(ctx context.Context, nodeID, illustrationID, caption string) error {
	for attempt := 0; attempt < 3; attempt++ {
		n, err := s.nodes.Get(ctx, nodeID)
		if err != nil {
			return err
		}
		doc, err := appendIllustration(n.ContentDoc, illustrationID, caption)
		if err != nil {
			return err
		}
		err = s.nodes.UpdateContentIfVersion(ctx, nodeID, doc, n.ContentVersion, s.now())
		if !errors.Is(err, node.ErrContentConflict) {
			return err
		}
	}
	return node.ErrContentConflict
}

func appendIllustration(raw *string, id, caption string) (string, error) {
	doc := map[string]any{"type": "doc", "content": []any{}}
	if raw != nil && strings.TrimSpace(*raw) != "" {
		if err := json.Unmarshal([]byte(*raw), &doc); err != nil {
			return "", fmt.Errorf("illustration: parse scene: %w", err)
		}
	}
	content, _ := doc["content"].([]any)
	doc["content"] = append(content, illustref.Node(id, caption))
	out, err := json.Marshal(doc)
	return string(out), err
}

// RecoverInterrupted marks requests left drawing when the app last closed.
// A drawing does not outlive the engine, so they can only be failed.
func (s *RequestService) RecoverInterrupted(ctx context.Context) error {
	_, err := s.repo.s.DB().ExecContext(ctx, `UPDATE illustration_requests SET status = ?, error = 'the app closed while drawing'
		WHERE status = ?`, RequestFailed, RequestGenerating)
	return err
}

const requestColumns = `id, project_id, COALESCE(target_node_id, ''), purpose, prompt, character_ids_json,
	include_references, prior_illustration_ids_json, aspect, caption, note, source, status, COALESCE(illustration_id, ''), placed_at, error, created_at, decided_at`

func scanRequest(row scanner) (Request, error) {
	var (
		r       Request
		ids     string
		prior   string
		placed  sql.NullInt64
		decided sql.NullInt64
	)
	if err := row.Scan(&r.ID, &r.ProjectID, &r.TargetNodeID, &r.Purpose, &r.Prompt, &ids, &r.IncludeReferences, &prior,
		&r.Aspect, &r.Caption, &r.Note, &r.Source, &r.Status, &r.IllustrationID, &placed, &r.Error, &r.CreatedAt, &decided); err != nil {
		return Request{}, err
	}
	if err := json.Unmarshal([]byte(ids), &r.CharacterIDs); err != nil {
		return Request{}, fmt.Errorf("illustration: request %s characters: %w", r.ID, err)
	}
	r.CharacterIDs = nonNil(r.CharacterIDs)
	if err := json.Unmarshal([]byte(prior), &r.PriorIllustrationIDs); err != nil {
		return Request{}, fmt.Errorf("illustration: request %s earlier pictures: %w", r.ID, err)
	}
	r.PriorIllustrationIDs = nonNil(r.PriorIllustrationIDs)
	if placed.Valid {
		r.PlacedAt = &placed.Int64
	}
	if decided.Valid {
		r.DecidedAt = &decided.Int64
	}
	return r, nil
}

func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
