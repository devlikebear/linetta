package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStdioPing(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := serveStdio(ctx, in, &out, t.TempDir()); err != nil {
		t.Fatalf("serveStdio: %v", err)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	line := strings.TrimSpace(out.String())
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		t.Fatalf("unmarshal %q: %v", line, err)
	}
	if string(env.Result) != `"pong"` {
		t.Fatalf("result = %s, want \"pong\"", env.Result)
	}
}
