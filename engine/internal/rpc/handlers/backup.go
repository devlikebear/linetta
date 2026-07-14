package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// CreateRecoveryBackup creates a versioned full-library snapshot that the
// native startup recovery screen can restore.
func CreateRecoveryBackup(st *store.Store, home string, now Clock) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		result, err := backup.RunManualRecovery(ctx, st.DB(), home, time.UnixMilli(now()))
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(result)
	}
}
