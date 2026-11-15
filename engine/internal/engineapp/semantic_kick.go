package engineapp

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/semantic"
)

// semanticKick is the manuscript indexer that also tells meaning search
// (#384) a work's prose changed: the worker embeds the changed paragraphs a
// moment later, outside the save.
type semanticKick struct {
	*manuscript.Indexer
	sem *semantic.Service
}

func (k semanticKick) Upsert(ctx context.Context, projectID, nodeID, contentDoc string) error {
	if err := k.Indexer.Upsert(ctx, projectID, nodeID, contentDoc); err != nil {
		return err
	}
	k.sem.Kick(projectID)
	return nil
}
