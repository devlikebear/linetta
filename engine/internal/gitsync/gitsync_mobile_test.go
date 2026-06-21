//go:build mobile

package gitsync

import (
	"context"
	"strings"
	"testing"
)

func TestGitSyncUnsupportedOnMobile(t *testing.T) {
	s := New(nil, nil, nil, nil)
	res, err := s.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected not-supported error on mobile build")
	}
	if !strings.Contains(err.Error(), "mobile") {
		t.Fatalf("expected mobile not-supported error, got %v; summary=%+v", err, res)
	}
}
