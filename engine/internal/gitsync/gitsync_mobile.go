//go:build mobile

package gitsync

import (
	"context"
	"errors"
	"time"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

var errUnsupported = errors.New("git sync is not supported on mobile builds")

type ResultSummary struct {
	Skipped      bool   `json:"skipped"`
	FilesWritten int    `json:"files_written"`
	Committed    bool   `json:"committed"`
	Pushed       bool   `json:"pushed"`
	Message      string `json:"message"`
	Error        string `json:"error"`
}

type CmdRunner func(ctx context.Context, dir string, args ...string) (string, error)

type Syncer struct {
	Settings      *settings.Store
	Projects      *project.Repo
	Nodes         *node.Repo
	Entities      *entity.Repo
	Relationships *relationship.Repo
	Run           CmdRunner
	Now           func() time.Time
	Ops           *opsstatus.Repo
}

func New(s *settings.Store, p *project.Repo, n *node.Repo, e *entity.Repo, rels ...*relationship.Repo) *Syncer {
	var rr *relationship.Repo
	if len(rels) > 0 {
		rr = rels[0]
	}
	return &Syncer{Settings: s, Projects: p, Nodes: n, Entities: e, Relationships: rr}
}

type InitResult struct {
	Skipped     bool   `json:"skipped"`
	AlreadyRepo bool   `json:"already_repo"`
	Created     bool   `json:"created"`
	Dir         string `json:"dir"`
	Error       string `json:"error"`
}

func (s *Syncer) Init(context.Context) (InitResult, error) {
	return InitResult{Error: errUnsupported.Error()}, errUnsupported
}

func (s *Syncer) RunOnce(context.Context) (ResultSummary, error) {
	return ResultSummary{Error: errUnsupported.Error()}, errUnsupported
}
