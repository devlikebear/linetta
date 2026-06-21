package engineapp

import "context"

type syncResult struct {
	Error string
}

type dailySyncer interface {
	RunOnce(ctx context.Context) (syncResult, error)
}
