//go:build mas || mobile

package main

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func TestSetupGitSyncDisabledIsNoop(t *testing.T) {
	s := rpc.NewServer()
	syncer := setupGitSync(s, nil, nil, nil, nil, nil, nil)
	res, err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("RunOnce returned non-empty error: %q", res.Error)
	}
}
