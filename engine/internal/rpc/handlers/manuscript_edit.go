package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type replaceApplyParams struct {
	Plan         manuscriptedit.ReplacePlan `json:"plan"`
	CandidateIDs []string                   `json:"candidate_ids"`
}

func ReplacePreview(service *manuscriptedit.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p manuscriptedit.ReplacePlanRequest
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if strings.TrimSpace(p.ProjectID) == "" || strings.TrimSpace(p.Query) == "" || strings.TrimSpace(p.Replacement) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id, query and replacement required"}
		}
		plan, err := service.PlanReplace(ctx, p)
		if err != nil {
			if errors.Is(err, manuscriptedit.ErrInvalidRequest) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(plan)
	}
}

func ReplaceApply(service *manuscriptedit.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p replaceApplyParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if strings.TrimSpace(p.Plan.ProjectID) == "" || strings.TrimSpace(p.Plan.Query) == "" ||
			strings.TrimSpace(p.Plan.Replacement) == "" || len(p.CandidateIDs) == 0 {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "plan and candidate_ids required"}
		}
		result, err := service.ApplyReplace(ctx, p.Plan, p.CandidateIDs, now())
		if err != nil {
			if errors.Is(err, manuscriptedit.ErrInvalidPlan) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(result)
	}
}
