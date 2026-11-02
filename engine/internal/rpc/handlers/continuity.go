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
	"github.com/devlikebear/linetta/engine/internal/ledger"
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

// DeleteContinuity returns a handler for continuity.delete, which fires
// continuity.changed for the same reason SetContinuityStatus does.
func DeleteContinuity(repo *continuity.Repo, notify func(method string, params any)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ObservationID string `json:"observation_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ObservationID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		obs, err := repo.Delete(ctx, in.ObservationID)
		if err != nil {
			return nil, continuityError(err)
		}
		if notify != nil {
			notify("continuity.changed", continuityChangedPayload{ProjectID: obs.ProjectID, NodeID: obs.NodeID})
		}
		return json.RawMessage(`{"ok":true}`), nil
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

// sampleTargets is how many of the latest scenes a check covers when it names
// none: enough to keep up, not the book.
const sampleTargets = 3

// SampleCheck returns a handler for continuity.sample_check. It picks the scenes
// to compare here, so the choice is the same however the check is started, and
// hands them to the agent to read with no conversation behind it.
func SampleCheck(repo *continuity.Repo, entries *ledger.Repo, ctrl AgentController, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
			NodeID    string `json:"node_id,omitempty"`
			// Force compares pairs again that were compared at these versions.
			Force bool `json:"force,omitempty"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		written, err := entries.SampleScenes(ctx, in.ProjectID)
		if err != nil {
			return nil, ledgerError(err)
		}
		scenes := make([]continuity.SampleScene, 0, len(written))
		for _, w := range written {
			scenes = append(scenes, continuity.SampleScene{NodeID: w.NodeID, Label: w.Label, Version: w.Version, Anchor: w.Anchor})
		}
		targets := []string{in.NodeID}
		if in.NodeID == "" {
			targets = nil
			for _, s := range scenes[max(0, len(scenes)-sampleTargets):] {
				targets = append(targets, s.NodeID)
			}
		}
		seen := func(p continuity.SamplePair) bool {
			if in.Force {
				return false
			}
			done, err := repo.PairSeen(ctx, p)
			return err == nil && done
		}
		var pairs []continuity.SamplePair
		for _, target := range targets {
			exclude := map[string]bool{}
			for _, p := range continuity.SamplePairs(scenes, target, "", nil) {
				exclude[p.Earlier.NodeID] = true
			}
			candidate, err := entries.Neighbour(ctx, in.ProjectID, target, exclude)
			if err != nil {
				return nil, ledgerError(err)
			}
			pairs = append(pairs, continuity.SamplePairs(scenes, target, candidate, seen)...)
		}
		out := struct {
			Pairs int `json:"pairs"`
		}{len(pairs)}
		if len(pairs) == 0 {
			return json.Marshal(out)
		}
		rules, err := entries.PinnedBodies(ctx, in.ProjectID)
		if err != nil {
			return nil, ledgerError(err)
		}
		if err := ctrl.StartSampleCheck(ctx, in.ProjectID, continuity.SampleRequest(in.ProjectID, rules, pairs), continuity.SampleCalls(pairs)); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		if err := repo.RecordPairs(ctx, now(), in.ProjectID, pairs); err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}
}
