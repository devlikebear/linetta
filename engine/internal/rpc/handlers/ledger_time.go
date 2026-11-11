package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// StoryTime returns a handler for ledger.story_time: the work's scenes in story
// order, the story date each is known at, and every elapsed time and age in
// years with what the comparison says of it. It is the table the ledger panel
// draws, one row an event.
func StoryTime(repo *ledger.Repo, nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		all, err := nodes.ListByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, err
		}
		st, err := repo.StoryTime(ctx, in.ProjectID)
		if err != nil {
			return nil, ledgerError(err)
		}
		return json.Marshal(struct {
			Scenes []KnowsScene `json:"scenes"`
			ledger.StoryTime
		}{storyScenes(all), st})
	}
}
