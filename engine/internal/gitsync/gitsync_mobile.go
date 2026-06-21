//go:build mobile

package gitsync

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("git sync is not supported on mobile builds")

func defaultRunner() CmdRunner {
	return nil
}

func (s *Syncer) Init(context.Context) (InitResult, error) {
	return InitResult{Error: errUnsupported.Error()}, errUnsupported
}

func (s *Syncer) RunOnce(context.Context) (ResultSummary, error) {
	return ResultSummary{Error: errUnsupported.Error()}, errUnsupported
}
