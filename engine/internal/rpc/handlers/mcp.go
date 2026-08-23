package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// MCPController is the slice of the MCP host the RPC layer needs. Declared as
// an interface so this file compiles on every build tag — the host itself is
// //go:build !mobile.
type MCPController interface {
	Status() (json.RawMessage, error)
	Enable(ctx context.Context) error
	// EnableToken enables and returns the token with the status, so the
	// settings pane can render a client command exactly once.
	EnableToken(ctx context.Context) (json.RawMessage, error)
	Disable(ctx context.Context) error
	RegenerateToken(ctx context.Context) (json.RawMessage, error)
	Activity(ctx context.Context, limit int) (json.RawMessage, error)
}

// ErrMCPPortInUse lets the host report a taken port without the RPC layer
// importing mcphost. The renderer turns the reason code into a localized
// "port is in use, pick another" message.
var ErrMCPPortInUse = errors.New("mcp port in use")

// ErrMCPConsentRequired means MCP access has not been accepted yet.
var ErrMCPConsentRequired = errors.New("mcp consent required")

func mcpError(err error) error {
	switch {
	case errors.Is(err, ErrMCPPortInUse):
		return &rpc.MethodError{
			Code:    rpc.CodeInvalidParams,
			Message: err.Error(),
			Data:    rpc.ReasonData("mcp_port_in_use"),
		}
	case errors.Is(err, ErrMCPConsentRequired):
		return &rpc.MethodError{
			Code:    rpc.CodeInvalidParams,
			Message: err.Error(),
			Data:    rpc.ReasonData("mcp_consent_required"),
		}
	default:
		return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
	}
}

// MCPStatus returns a handler for mcp.status.
func MCPStatus(ctrl MCPController) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := ctrl.Status()
		if err != nil {
			return nil, mcpError(err)
		}
		return out, nil
	}
}

// MCPEnable returns a handler for mcp.enable.
func MCPEnable(ctrl MCPController) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := ctrl.EnableToken(ctx)
		if err != nil {
			return nil, mcpError(err)
		}
		return out, nil
	}
}

// MCPDisable returns a handler for mcp.disable. This is the kill switch: it
// drops the listener immediately.
func MCPDisable(ctrl MCPController) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		if err := ctrl.Disable(ctx); err != nil {
			return nil, mcpError(err)
		}
		out, err := ctrl.Status()
		if err != nil {
			return nil, mcpError(err)
		}
		return out, nil
	}
}

// MCPRegenerateToken returns a handler for mcp.regenerate_token. The new token
// is returned once so the settings pane can render a fresh client snippet;
// settings.get never exposes it again.
func MCPRegenerateToken(ctrl MCPController) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := ctrl.RegenerateToken(ctx)
		if err != nil {
			return nil, mcpError(err)
		}
		return out, nil
	}
}

type mcpActivityParams struct {
	Limit int `json:"limit,omitempty"`
}

// MCPActivity returns a handler for mcp.activity: what external agents did.
func MCPActivity(ctrl MCPController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p mcpActivityParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		out, err := ctrl.Activity(ctx, p.Limit)
		if err != nil {
			return nil, mcpError(err)
		}
		return out, nil
	}
}
