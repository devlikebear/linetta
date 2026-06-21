package engineapp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAppHandlePing(t *testing.T) {
	ctx := context.Background()
	app, err := Open(ctx, Options{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer app.Close()

	resp, err := app.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error != nil {
		t.Fatalf("ping error: %s", env.Error.Message)
	}
	if string(env.Result) != `"pong"` {
		t.Fatalf("ping result = %s, want \"pong\"", env.Result)
	}
}
