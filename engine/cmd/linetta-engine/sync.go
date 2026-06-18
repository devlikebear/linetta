package main

import "context"

// syncResult is the tag-agnostic subset of gitsync.ResultSummary that main.go's
// backup retention loop needs. Keeping it free of any gitsync import lets the
// mas build omit the gitsync package entirely.
type syncResult struct {
	Error string
}

// dailySyncer is the daily git-sync hook invoked by the backup retention loop.
// The !mas build backs it with a real gitsync.Syncer; the mas build supplies a
// no-op.
type dailySyncer interface {
	RunOnce(ctx context.Context) (syncResult, error)
}
