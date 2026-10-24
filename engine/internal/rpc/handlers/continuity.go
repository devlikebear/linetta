package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/continuity"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// continuity.* serve story-assist Phase 2's continuity checks: an agent's
// engine-verified reading of one scene against the work's own record. The
// screen reads and acts on them here; nothing records one except the
// linetta_report_continuity MCP tool.

func continuityError(err error) error {
	switch {
	case errors.Is(err, continuity.ErrNotFound):
		return rpc.NotFound(rpc.ReasonContinuityObservationNotFound, err.Error())
	case errors.Is(err, continuity.ErrObservationReplaced):
		return rpc.NotFound(rpc.ReasonContinuityObservationReplaced, err.Error())
	case errors.Is(err, continuity.ErrNodeRequired), errors.Is(err, continuity.ErrInvalidStatus), errors.Is(err, continuity.ErrCannotSetReplaced):
		return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
	default:
		return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
	}
}

// continuityRefOut is one ref plus the display name the screen shows on its
// chip, resolved here so the screen never makes a second round trip per ref.
type continuityRefOut struct {
	continuity.Ref
	// Label is the entity's name, the fact's claim, the beat's label, or
	// the scene's breadcrumb — empty when the ref no longer resolves (it
	// was deleted after this observation was recorded; Validate guaranteed
	// it existed at record time, not forever after).
	Label string `json:"label,omitempty"`
}

type continuityObservationOut struct {
	ID            string             `json:"id"`
	CheckID       string             `json:"check_id"`
	ProjectID     string             `json:"project_id"`
	NodeID        string             `json:"node_id"`
	Category      string             `json:"category"`
	Kind          string             `json:"kind"`
	Severity      string             `json:"severity"`
	Summary       string             `json:"summary"`
	Suggestion    string             `json:"suggestion"`
	ParagraphFrom int                `json:"paragraph_from"`
	ParagraphTo   int                `json:"paragraph_to"`
	Quote         string             `json:"quote"`
	Refs          []continuityRefOut `json:"refs"`
	Status        string             `json:"status"`
	StatusAt      int64              `json:"status_at"`
	CreatedAt     int64              `json:"created_at"`
}

type continuityCheckOut struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
	// Breadcrumb is the checked scene's outline path, e.g. "1부 / 1장 / 씬
	// 3" — the screen groups and labels checks by it without a second
	// fetch per check.
	Breadcrumb     string                     `json:"breadcrumb"`
	ContentVersion int                        `json:"content_version"`
	Source         string                     `json:"source"`
	Note           string                     `json:"note"`
	CheckedAt      int64                      `json:"checked_at"`
	Stale          bool                       `json:"stale"`
	Observations   []continuityObservationOut `json:"observations"`
}

// ListContinuity returns a handler for continuity.list.
func ListContinuity(repo *continuity.Repo, nodes *node.Repo, entities *entity.Repo, facts *fact.Repo, beats *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string   `json:"project_id"`
			NodeID    string   `json:"node_id,omitempty"`
			Statuses  []string `json:"statuses,omitempty"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		checks, err := repo.List(ctx, continuity.ListFilter{
			ProjectID: in.ProjectID, NodeID: in.NodeID, Statuses: in.Statuses,
		})
		if err != nil {
			return nil, continuityError(err)
		}

		byID := map[string]node.Node{}
		if nodes != nil {
			all, err := nodes.ListByProject(ctx, in.ProjectID)
			if err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
			}
			for _, n := range all {
				byID[n.ID] = n
			}
		}

		out := make([]continuityCheckOut, 0, len(checks))
		for _, c := range checks {
			breadcrumb := ""
			if n, ok := byID[c.NodeID]; ok {
				breadcrumb = node.BreadcrumbLabel(byID, n)
			}
			observations := make([]continuityObservationOut, 0, len(c.Observations))
			for _, o := range c.Observations {
				refs := make([]continuityRefOut, 0, len(o.Refs))
				for _, ref := range o.Refs {
					refs = append(refs, continuityRefOut{
						Ref:   ref,
						Label: continuityRefLabel(ctx, ref, byID, entities, facts, beats),
					})
				}
				observations = append(observations, continuityObservationOut{
					ID: o.ID, CheckID: o.CheckID, ProjectID: o.ProjectID, NodeID: o.NodeID,
					Category: o.Category, Kind: o.Kind, Severity: o.Severity,
					Summary: o.Summary, Suggestion: o.Suggestion,
					ParagraphFrom: o.ParagraphFrom, ParagraphTo: o.ParagraphTo, Quote: o.Quote,
					Refs: refs, Status: o.Status, StatusAt: o.StatusAt, CreatedAt: o.CreatedAt,
				})
			}
			out = append(out, continuityCheckOut{
				ID: c.ID, ProjectID: c.ProjectID, NodeID: c.NodeID, Breadcrumb: breadcrumb,
				ContentVersion: c.ContentVersion, Source: c.Source, Note: c.Note,
				CheckedAt: c.CheckedAt, Stale: c.Stale, Observations: observations,
			})
		}
		return json.Marshal(struct {
			Checks []continuityCheckOut `json:"checks"`
		}{out})
	}
}

// continuityRefLabel resolves one ref's display name. A ref that no longer
// resolves (its entity, fact, beat or scene was deleted after this
// observation was recorded) returns "" rather than erroring the whole
// list — Validate guaranteed it existed when the check was made, not that
// it still does.
func continuityRefLabel(ctx context.Context, ref continuity.Ref, byID map[string]node.Node, entities *entity.Repo, facts *fact.Repo, beats *beat.Repo) string {
	switch ref.Type {
	case continuity.RefTypeEntity:
		if entities == nil {
			return ""
		}
		e, err := entities.Get(ctx, ref.ID)
		if err != nil {
			return ""
		}
		return e.Name
	case continuity.RefTypeFact:
		if facts == nil {
			return ""
		}
		c, err := facts.Get(ctx, ref.ID)
		if err != nil {
			return ""
		}
		return c.Claim
	case continuity.RefTypeBeat:
		if beats == nil {
			return ""
		}
		b, err := beats.Get(ctx, ref.ID)
		if err != nil {
			return ""
		}
		return b.Label
	case continuity.RefTypeScene:
		if n, ok := byID[ref.ID]; ok {
			return node.BreadcrumbLabel(byID, n)
		}
		return ""
	default:
		return ""
	}
}

// continuityChangedPayload mirrors mcphost's ChangedPayload and matches the
// shape memory.changed/skills.changed already use: a listener must not have
// to handle two shapes for the same notification method.
type continuityChangedPayload struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
}

// SetContinuityStatus returns a handler for continuity.set_status. notify
// fires "continuity.changed" after a successful change, the same way
// skills.write fires "skills.changed" — a different window's ContextPanel
// does not otherwise hear about a status a writer changed from Story
// World, and its open-observation badge would read stale until something
// else happened to remount it.
func SetContinuityStatus(repo *continuity.Repo, now Clock, notify func(method string, params any)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ObservationID string `json:"observation_id"`
			Status        string `json:"status"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ObservationID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		obs, err := repo.SetStatus(ctx, now(), in.ObservationID, in.Status)
		if err != nil {
			return nil, continuityError(err)
		}
		if notify != nil {
			notify("continuity.changed", continuityChangedPayload{ProjectID: obs.ProjectID, NodeID: obs.NodeID})
		}
		return json.Marshal(obs)
	}
}

// ContinuityOpenCounts returns a handler for continuity.open_counts: how
// many open observations each scene in the work carries, for
// ContextPanel's badge.
func ContinuityOpenCounts(repo *continuity.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		counts, err := repo.OpenCounts(ctx, in.ProjectID)
		if err != nil {
			return nil, continuityError(err)
		}
		return json.Marshal(struct {
			Counts map[string]int `json:"counts"`
		}{counts})
	}
}
