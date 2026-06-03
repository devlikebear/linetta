package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// The param guards in the companion handlers run before svc is touched, so we
// can validate them with a nil service.

func TestCompanionSendInvalidParams(t *testing.T) {
	clock := func() int64 { return 0 }
	h := CompanionSend(nil, clock)

	// Empty params object -> missing project_id and text.
	_, err := h(context.Background(), json.RawMessage(`{}`))
	var mErr *rpc.MethodError
	if !errors.As(err, &mErr) || mErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("empty params: expected rpc.MethodError CodeInvalidParams, got %T %v", err, err)
	}

	// project_id present but text missing.
	_, err = h(context.Background(), json.RawMessage(`{"project_id":"p1"}`))
	if !errors.As(err, &mErr) || mErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("missing text: expected rpc.MethodError CodeInvalidParams, got %T %v", err, err)
	}
}

func TestCompanionHistoryInvalidParams(t *testing.T) {
	h := CompanionHistory(nil)
	_, err := h(context.Background(), json.RawMessage(`{}`))
	var mErr *rpc.MethodError
	if !errors.As(err, &mErr) || mErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("expected rpc.MethodError CodeInvalidParams, got %T %v", err, err)
	}
}

func TestCompanionCompactInvalidParams(t *testing.T) {
	clock := func() int64 { return 0 }
	h := CompanionCompact(nil, clock)
	_, err := h(context.Background(), json.RawMessage(`{}`))
	var mErr *rpc.MethodError
	if !errors.As(err, &mErr) || mErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("expected rpc.MethodError CodeInvalidParams, got %T %v", err, err)
	}
}

func TestCompanionClearInvalidParams(t *testing.T) {
	h := CompanionClear(nil)
	_, err := h(context.Background(), json.RawMessage(`{}`))
	var mErr *rpc.MethodError
	if !errors.As(err, &mErr) || mErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("expected rpc.MethodError CodeInvalidParams, got %T %v", err, err)
	}
}

func TestCompanionCancelInvalidParams(t *testing.T) {
	h := CompanionCancel(nil)
	_, err := h(context.Background(), json.RawMessage(`{}`))
	var mErr *rpc.MethodError
	if !errors.As(err, &mErr) || mErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("expected rpc.MethodError CodeInvalidParams, got %T %v", err, err)
	}
}

func TestCompanionRememberInvalidParams(t *testing.T) {
	h := CompanionRemember(nil)
	_, err := h(context.Background(), json.RawMessage(`{"project_id":"p"}`)) // text missing
	var me *rpc.MethodError
	if !errors.As(err, &me) || me.Code != rpc.CodeInvalidParams {
		t.Fatalf("expected InvalidParams, got %v", err)
	}
}

func TestCompanionApplyOpsInvalidParams(t *testing.T) {
	clock := func() int64 { return 0 }
	h := CompanionApplyOps(nil, clock)

	for _, params := range []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"project_id":"p"}`),
		json.RawMessage(`{"ops":[{"op":"set_outline","outline":"x"}]}`),
	} {
		_, err := h(context.Background(), params)
		var me *rpc.MethodError
		if !errors.As(err, &me) || me.Code != rpc.CodeInvalidParams {
			t.Fatalf("params %s: expected InvalidParams, got %T %v", params, err, err)
		}
	}
}
