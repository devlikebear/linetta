//go:build mobile

package clidetect

import (
	"context"
	"testing"
)

func TestDetectUnsupportedOnMobile(t *testing.T) {
	if got := Detect(context.Background()); got != "" {
		t.Fatalf("expected empty path on mobile, got %q", got)
	}
}
