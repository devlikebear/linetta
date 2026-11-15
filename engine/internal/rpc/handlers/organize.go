package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/organize"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// OrganizeStart is what the agent needs to run an organize pass.
type OrganizeStart struct {
	ProjectID    string
	EpisodeLabel string
	Language     string
	Scenes       []organize.Scene
}

// organizeResult tells the app what was asked of the pass: nothing when every
// scene of the episode was already read as it is.
type organizeResult struct {
	Started      bool   `json:"started"`
	EpisodeLabel string `json:"episode_label"`
	Scenes       int    `json:"scenes"`
	Unchanged    int    `json:"unchanged"`
	Short        int    `json:"short"`
}

// AgentOrganize handles agent.organize {project_id, node_id, force?} (#387):
// the writer asks for the episode node_id is in to be read for what the
// work's record lacks. Proposals land in the inbox; nothing is changed.
func AgentOrganize(ctrl AgentController, repo *organize.Repo, language func(context.Context) string, now func() int64) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
			NodeID    string `json:"node_id"`
			Force     bool   `json:"force,omitempty"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.NodeID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id, node_id required"}
		}
		plan, err := repo.Plan(ctx, in.ProjectID, in.NodeID, in.Force)
		if errors.Is(err, organize.ErrNotInEpisode) {
			return nil, rpc.NotFound(rpc.ReasonOrganizeNotInEpisode, err.Error())
		}
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		lang := ""
		if language != nil {
			lang = language(ctx)
		}
		label := node.DisplayLabel(plan.EpisodeLabel, lang)
		if label == "" {
			label = plan.EpisodeLabel
		}
		out := organizeResult{EpisodeLabel: label, Scenes: len(plan.Scenes), Unchanged: plan.Unchanged, Short: plan.Short}
		if len(plan.Scenes) == 0 {
			return json.Marshal(out)
		}
		scenes := plan.Scenes
		err = ctrl.StartOrganize(ctx, OrganizeStart{ProjectID: plan.ProjectID, EpisodeLabel: label, Language: lang, Scenes: scenes},
			func(ctx context.Context, outcome string) {
				for _, sc := range scenes {
					// The record is best effort: a scene left unrecorded is
					// only read again next time.
					_ = repo.Record(ctx, plan.ProjectID, sc, outcome, now())
				}
			})
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		out.Started = true
		return json.Marshal(out)
	}
}
