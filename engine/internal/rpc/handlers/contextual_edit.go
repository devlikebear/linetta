package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/contextualedit"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type contextualApplyParams struct {
	Plan      contextualedit.ChangePlan     `json:"plan"`
	Selection contextualedit.ApplySelection `json:"selection"`
}

func ContextualResolveTarget(service *contextualedit.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in contextualedit.ResolveTargetInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		target, err := service.ResolveTarget(ctx, in)
		if err != nil {
			return nil, contextualError(err)
		}
		return json.Marshal(target)
	}
}

func ContextualPlanChange(service *contextualedit.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in contextualedit.ChangeInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		plan, err := service.PlanContextChange(ctx, in)
		if err != nil {
			return nil, contextualError(err)
		}
		return json.Marshal(plan)
	}
}

func ContextualApplyChange(service *contextualedit.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in contextualApplyParams
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if strings.TrimSpace(in.Plan.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "plan required"}
		}
		result, err := service.ApplyContextChange(ctx, in.Plan, in.Selection, now())
		if err != nil {
			return nil, contextualError(err)
		}
		return json.Marshal(result)
	}
}

func ContextualCheckConsistency(service *contextualedit.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in contextualedit.ConsistencyInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if strings.TrimSpace(in.ProjectID) == "" || len(in.OldTerms) == 0 {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and old_terms required"}
		}
		report, err := service.CheckAfterChange(ctx, in)
		if err != nil {
			return nil, contextualError(err)
		}
		return json.Marshal(report)
	}
}

func contextualError(err error) error {
	if errors.Is(err, contextualedit.ErrInvalidInput) || errors.Is(err, contextualedit.ErrInvalidPlan) {
		return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
	}
	return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
}
