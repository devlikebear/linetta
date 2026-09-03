package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// CodexService is the slice of internal/codexauth the RPC layer needs. It
// speaks JSON and errors only, so the handler package stays free of the login
// flow's HTTP machinery.
type CodexService interface {
	LoginStart(ctx context.Context) (json.RawMessage, error)
	LoginStatus(ctx context.Context) (json.RawMessage, error)
	Logout(ctx context.Context) error
}

// CodexLoginStart returns a handler for codex.login_start. It answers with the
// authorize URL the shell opens in the OS browser; the login itself completes
// on the loopback callback, and the pane learns the outcome from
// codex.login_status.
func CodexLoginStart(svc CodexService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := svc.LoginStart(ctx)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return out, nil
	}
}

// CodexLoginStatus returns a handler for codex.login_status.
func CodexLoginStatus(svc CodexService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := svc.LoginStatus(ctx)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return out, nil
	}
}

// CodexLogout returns a handler for codex.logout.
func CodexLogout(svc CodexService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		if err := svc.Logout(ctx); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
