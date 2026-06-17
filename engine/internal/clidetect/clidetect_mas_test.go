//go:build mas

package clidetect

import (
	"context"
	"testing"
)

func TestDetectMASReturnsEmpty(t *testing.T) {
	if got := Detect(context.Background()); got != "" {
		t.Fatalf("expected empty path in mas build, got %q", got)
	}
}
