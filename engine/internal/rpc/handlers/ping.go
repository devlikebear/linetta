// Package handlers contains the RPC method implementations exposed by the
// Linetta engine.
package handlers

import (
	"context"
	"encoding/json"
)

// Ping is the proof-of-life handler. It ignores its params and returns "pong".
func Ping(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`"pong"`), nil
}
