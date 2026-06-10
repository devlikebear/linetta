package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/stats"
)

type todayStatsParams struct {
	ProjectID string `json:"project_id"`
}

type rangeStatsParams struct {
	ProjectID string `json:"project_id"`
	FromDay   string `json:"from_day"`
	ToDay     string `json:"to_day"`
}

// TodayStats returns today's positive writing progress for a project.
func TodayStats(repo *stats.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p todayStatsParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		out, err := repo.Today(ctx, p.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(out)
	}
}

// RangeStats returns daily writing progress for an inclusive date range.
func RangeStats(repo *stats.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p rangeStatsParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.FromDay == "" || p.ToDay == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id, from_day and to_day required"}
		}
		out, err := repo.Range(ctx, p.ProjectID, p.FromDay, p.ToDay)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(out)
	}
}

// SummaryStats returns compact writing progress metrics for the sidebar.
func SummaryStats(repo *stats.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p todayStatsParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		out, err := repo.Summary(ctx, p.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(out)
	}
}
