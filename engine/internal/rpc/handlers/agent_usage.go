package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/agentusage"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// agentUsageDefaultDays is the window Settings shows when it names none.
const agentUsageDefaultDays = 30

// AgentUsage returns what the built-in agent's requests cost over the last
// `days` days (30 when unset), summed by kind and model. Days beyond what the
// table keeps (agentusage.Retention) read as all of it.
func AgentUsage(repo *agentusage.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			Days int `json:"days"`
		}
		if len(params) > 0 && string(params) != "null" {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "days must be a number"}
			}
		}
		if p.Days <= 0 {
			p.Days = agentUsageDefaultDays
		}
		const day int64 = 24 * 60 * 60 * 1000
		window := min(int64(p.Days)*day, agentusage.Retention)
		summary, err := repo.Summary(ctx, now()-window)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(summary)
	}
}
