package main

import (
	"encoding/json"
	"testing"
)

func TestHandleRequestPing(t *testing.T) {
	if err := startEngine(t.TempDir()); err != nil {
		t.Fatalf("startEngine: %v", err)
	}
	defer stopEngine()

	resp := handleRequest([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("unmarshal %q: %v", resp, err)
	}
	if string(env.Result) != `"pong"` {
		t.Fatalf("result = %s, want \"pong\"", env.Result)
	}
}

func TestHandleRequestBeforeStart(t *testing.T) {
	_ = stopEngine()
	resp := handleRequest([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	var env struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("unmarshal %q: %v", resp, err)
	}
	if env.Error == nil {
		t.Fatalf("expected error envelope when engine not started, got %s", resp)
	}
}

func TestNotifierFanout(t *testing.T) {
	var gotMethod, gotParams string
	setGoNotifier(func(method string, params json.RawMessage) {
		gotMethod = method
		gotParams = string(params)
	})
	defer setGoNotifier(nil)

	emitNotify("ai.delta", json.RawMessage(`{"text":"hi"}`))
	if gotMethod != "ai.delta" {
		t.Fatalf("method = %q, want ai.delta", gotMethod)
	}
	if gotParams != `{"text":"hi"}` {
		t.Fatalf("params = %q", gotParams)
	}
}
