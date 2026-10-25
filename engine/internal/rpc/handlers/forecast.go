package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/forecast"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/proposals"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// forecast.* serve story-assist Phase 4's "Where the story could go" sheet:
// an agent's linetta_save_forecast candidates, and the writer's own direct
// select/clear/delete — no MCP tool, no LLM turn for any of those three.
// Staleness is recomputed every time forecast.list is called (never a
// cached flag); a stale forecast may still be selected — the engine does
// not refuse it, only reports the warning, so the policy lives in one
// place rather than splitting between the engine and the screen.

// forecastDecisionOut adds the decided-on character's display name, same
// shape continuityRefOut/proposalEvidenceOut's "resolve here, once" rule.
type forecastDecisionOut struct {
	forecast.CharDecision
	EntityName string `json:"entity_name,omitempty"`
}

// forecastRiskOut adds the named entity/fact/scene's display text, if any.
type forecastRiskOut struct {
	forecast.Risk
	EntityName     string `json:"entity_name,omitempty"`
	FactClaim      string `json:"fact_claim,omitempty"`
	NodeBreadcrumb string `json:"node_breadcrumb,omitempty"`
}

// forecastBranchOut shadows Branch's own Decisions/Risks with the
// annotated versions above — Go's JSON encoder prefers the outer,
// same-named field over the embedded one, so every other Branch field
// (title, premise, beats, …) still comes through unchanged.
type forecastBranchOut struct {
	forecast.Branch
	Decisions []forecastDecisionOut `json:"decisions,omitempty"`
	Risks     []forecastRiskOut     `json:"risks,omitempty"`
}

// forecastOut shadows Forecast's own Branches the same way, and adds the
// per-list staleness forecast.Service.List recomputes plus the base
// episode's display label.
type forecastOut struct {
	forecast.Forecast
	Branches         []forecastBranchOut    `json:"branches"`
	Stale            bool                   `json:"stale"`
	Reasons          []forecast.StaleReason `json:"reasons,omitempty"`
	BaseEpisodeLabel string                 `json:"base_episode_label,omitempty"`
}

// forecastRefResolver is what ListForecasts needs to annotate a branch's
// refs with display names — a narrow seam so tests can wire only what
// they need, the same shape ListProposals' entities/threads params take.
type forecastRefResolver struct {
	projects *project.Repo
	nodes    *node.Repo
	entities *entity.Repo
	facts    *fact.Repo
}

func buildForecastOut(ctx context.Context, f forecast.ListedForecast, byNodeID map[string]node.Node, baseLabel string, r forecastRefResolver) forecastOut {
	branches := make([]forecastBranchOut, 0, len(f.Branches))
	for _, b := range f.Branches {
		decisions := make([]forecastDecisionOut, 0, len(b.Decisions))
		for _, d := range b.Decisions {
			name := ""
			if r.entities != nil {
				if e, err := r.entities.Get(ctx, d.EntityID); err == nil {
					name = e.Name
				}
			}
			decisions = append(decisions, forecastDecisionOut{CharDecision: d, EntityName: name})
		}
		risks := make([]forecastRiskOut, 0, len(b.Risks))
		for _, risk := range b.Risks {
			ro := forecastRiskOut{Risk: risk}
			if risk.EntityID != "" && r.entities != nil {
				if e, err := r.entities.Get(ctx, risk.EntityID); err == nil {
					ro.EntityName = e.Name
				}
			}
			if risk.FactID != "" && r.facts != nil {
				if card, err := r.facts.Get(ctx, risk.FactID); err == nil {
					ro.FactClaim = card.Claim
				}
			}
			if risk.NodeID != "" {
				if n, ok := byNodeID[risk.NodeID]; ok {
					ro.NodeBreadcrumb = node.BreadcrumbLabel(byNodeID, n)
				}
			}
			risks = append(risks, ro)
		}
		branches = append(branches, forecastBranchOut{Branch: b, Decisions: decisions, Risks: risks})
	}
	return forecastOut{
		Forecast: f.Forecast, Branches: branches, Stale: f.Stale, Reasons: f.Reasons, BaseEpisodeLabel: baseLabel,
	}
}

// ListForecasts returns a handler for forecast.list.
func ListForecasts(svc *forecast.Service, projects *project.Repo, nodes *node.Repo, entities *entity.Repo, facts *fact.Repo) rpc.Handler {
	r := forecastRefResolver{projects: projects, nodes: nodes, entities: entities, facts: facts}
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return json.Marshal(struct {
				Forecasts []forecastOut `json:"forecasts"`
			}{[]forecastOut{}})
		}
		list, err := svc.List(ctx, in.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}

		byNodeID := map[string]node.Node{}
		if nodes != nil {
			if all, err := nodes.ListByProject(ctx, in.ProjectID); err == nil {
				for _, n := range all {
					byNodeID[n.ID] = n
				}
			}
		}
		preset := ""
		if projects != nil {
			if proj, err := projects.Get(ctx, in.ProjectID); err == nil {
				preset = proj.OutlinePreset
			}
		}

		out := make([]forecastOut, 0, len(list))
		for _, f := range list {
			label := ""
			if f.BaseNodeID != "" && nodes != nil {
				if ep, err := episode.Resolve(ctx, nodes, preset, f.BaseNodeID); err == nil {
					label = ep.Label
				}
			}
			out = append(out, buildForecastOut(ctx, f, byNodeID, label, r))
		}
		return json.Marshal(struct {
			Forecasts []forecastOut `json:"forecasts"`
		}{out})
	}
}

// SelectForecast returns a handler for forecast.select. A stale forecast
// may still be selected — see this file's doc comment — so the response
// carries the warning rather than a refusal.
func SelectForecast(svc *forecast.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ForecastID string `json:"forecast_id"`
			BranchID   string `json:"branch_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil ||
			strings.TrimSpace(in.ForecastID) == "" || strings.TrimSpace(in.BranchID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "forecasts are not available"}
		}
		f, err := svc.Select(ctx, now(), in.ForecastID, in.BranchID)
		if err != nil {
			if errors.Is(err, forecast.ErrNotFound) {
				return nil, rpc.NotFound(rpc.ReasonForecastNotFound, err.Error())
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		reasons, err := svc.Stale(ctx, f)
		if err != nil {
			reasons = nil
		}
		return json.Marshal(struct {
			OK      bool                   `json:"ok"`
			Stale   bool                   `json:"stale"`
			Reasons []forecast.StaleReason `json:"reasons,omitempty"`
		}{true, len(reasons) > 0, reasons})
	}
}

// ClearForecastSelection returns a handler for forecast.clear_selection.
func ClearForecastSelection(svc *forecast.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "forecasts are not available"}
		}
		if err := svc.ClearSelection(ctx, in.ProjectID); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(struct {
			OK bool `json:"ok"`
		}{true})
	}
}

// DeleteForecast returns a handler for forecast.delete.
func DeleteForecast(svc *forecast.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ForecastID string `json:"forecast_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ForecastID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "forecasts are not available"}
		}
		if err := svc.Delete(ctx, in.ForecastID); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(struct {
			OK bool `json:"ok"`
		}{true})
	}
}

// UndoProposals returns a handler for proposals.undo: the forecast sheet's
// outline-reflection revert button, scoped to one storyops undo batch
// (story-assist Phase 4's 4.A.5) — see proposals.Service.Undo's doc
// comment for exactly what it reverts.
func UndoProposals(svc *proposals.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
			BatchID   string `json:"batch_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil ||
			strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.BatchID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "proposals are not available"}
		}
		if err := svc.Undo(ctx, now(), in.ProjectID, in.BatchID); err != nil {
			if errors.Is(err, proposals.ErrUndoBatchNotFound) {
				return nil, rpc.NotFound(rpc.ReasonProposalsUndoUnavailable, err.Error())
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(struct {
			OK bool `json:"ok"`
		}{true})
	}
}
