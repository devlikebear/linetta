// Package companion is what remains of Linetta's built-in AI companion after
// the MCP-first pivot (#47) removed it: the data it owned, not the agent that
// produced it.
//
// The transcript, the writer's remembered facts, and their attached references
// all survive because something still reads them. The story brief an MCP agent
// asks for is built from the fact, memory, and reference sources here, and the
// transcript archive export reads the history repo. Nothing in this package
// talks to a language model any more.
//
// The package name is now a misnomer — these are three unrelated stores that
// happened to share an owner. Renaming them is a follow-up, deliberately not
// bundled with a deletion this size.
package companion

import (
	"context"
	"os"

	"github.com/devlikebear/linetta/engine/internal/fact"
)

// factContextLimit caps how many Fact Book cards reach a story brief.
const factContextLimit = 12

// Service holds the stores the story brief and the archive export still need.
type Service struct {
	facts      *fact.Repo
	memBase    string
	history    *HistoryRepo
	references *ReferenceRepo
}

// NewService constructs the surviving store owner. memBase is the root the
// per-project memory workspaces live under (e.g. <home>).
func NewService(memBase string) *Service {
	return &Service{memBase: memBase}
}

func (s *Service) WithHistory(repo *HistoryRepo) *Service {
	s.history = repo
	return s
}

func (s *Service) WithReferences(repo *ReferenceRepo) *Service {
	s.references = repo
	return s
}

func (s *Service) WithFacts(repo *fact.Repo) *Service {
	s.facts = repo
	return s
}

// HistoryView returns the stored transcript for a project or scene.
func (s *Service) HistoryView(ctx context.Context, q HistoryQuery) ([]HistoryMessage, error) {
	if s.history == nil {
		return nil, nil
	}
	return s.history.List(ctx, q)
}

// DeleteProjectData removes the per-project memory workspace. Rows in the
// transcript and reference tables go with the project via ON DELETE CASCADE.
func (s *Service) DeleteProjectData(_ context.Context, projectID string) error {
	return os.RemoveAll(memRoot(s.memBase, projectID))
}

// ListReferences returns the writer's attached material for a project or scene.
func (s *Service) ListReferences(ctx context.Context, q ReferenceQuery) ([]Reference, error) {
	if s.references == nil {
		return nil, nil
	}
	return s.references.List(ctx, q)
}
