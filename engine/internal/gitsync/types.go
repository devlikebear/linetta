package gitsync

import (
	"context"
	"time"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// ResultSummary is the wire shape returned to the UI. Soft errors (no remote,
// auth, network) land in Error; only configuration/IO problems before git runs
// are reported as a Go error from RunOnce.
type ResultSummary struct {
	Skipped      bool   `json:"skipped"`
	FilesWritten int    `json:"files_written"`
	Committed    bool   `json:"committed"`
	Pushed       bool   `json:"pushed"`
	Message      string `json:"message"`
	Error        string `json:"error"`
}

// CmdRunner is the test seam for recording or stubbing git invocations.
type CmdRunner func(ctx context.Context, dir string, args ...string) (string, error)

// Syncer composes the dependencies needed to read settings, list projects,
// build markdown payloads, and run git when the platform supports it.
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

// New constructs a Syncer. Mobile builds install a nil runner and return
// unsupported errors before any git command can run.
func New(s *settings.Store, p *project.Repo, n *node.Repo, e *entity.Repo, rels ...*relationship.Repo) *Syncer {
	var rr *relationship.Repo
	if len(rels) > 0 {
		rr = rels[0]
	}
	return &Syncer{
		Settings:      s,
		Projects:      p,
		Nodes:         n,
		Entities:      e,
		Relationships: rr,
		Run:           defaultRunner(),
		Now:           time.Now,
	}
}

// InitResult is the structured outcome of one Init call.
type InitResult struct {
	Skipped     bool   `json:"skipped"`
	AlreadyRepo bool   `json:"already_repo"`
	Created     bool   `json:"created"`
	Dir         string `json:"dir"`
	Error       string `json:"error"`
}
