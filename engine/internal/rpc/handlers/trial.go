package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// TrialService is where the Linetta trial's standing comes from.
type TrialService interface {
	TrialStatus(ctx context.Context) publish.TrialStatus
}

// TrialStatus returns a handler for trial.status: how much of the Linetta
// trial this device's Works account has left. It never fails; a device with
// no connection or no answer from the site says so in state.
func TrialStatus(svc TrialService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(svc.TrialStatus(ctx))
	}
}
