package handlers

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPing(t *testing.T) {
	got, err := Ping(context.Background(), nil)
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	var s string
	if err := json.Unmarshal(got, &s); err != nil {
		t.Fatalf("Ping result not JSON string: %v (raw=%s)", err, string(got))
	}
	if s != "pong" {
		t.Errorf("ping = %q, want %q", s, "pong")
	}
}
