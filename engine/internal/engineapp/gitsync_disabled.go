//go:build mas || mobile

package engineapp

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

const gitSyncAvailable = false

func setupGitSync(deps syncDeps) dailySyncer {
	unavailable := func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, &rpc.MethodError{
			Code:    rpc.CodeMethodNotFound,
			Message: "git sync is not available in this build",
		}
	}
	deps.server.Handle("git_sync.run", unavailable)
	deps.server.Handle("git_sync.init", unavailable)
	return noopSyncer{}
}
