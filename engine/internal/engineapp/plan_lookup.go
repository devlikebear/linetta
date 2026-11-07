package engineapp

import (
	"context"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/forecast"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

// planLookup answers whether an empty scene has anything that says what it is
// for. It reads only what the work already keeps — no new writing contract:
//
//   - the scene's plan, or its episode's (nodes.plan, linetta_write_summary
//     as_plan=true);
//   - the writer's goal for the scene (scene_intents.goal);
//   - a beat of the forecast branch the writer chose that falls on this
//     scene's episode.
//
// Only "is anything there" is asked. The built-in agent's plan gate refuses on
// the answer; the MCP write tools turn it into a warning for external clients.
type planLookup struct {
	nodes     *node.Repo
	projects  *project.Repo
	direction *direction.Repo
	forecasts *forecast.Service
}

func (p planLookup) PlanState(ctx context.Context, nodeID string) (empty, planned bool, err error) {
	if p.nodes == nil {
		return false, false, nil
	}
	n, err := p.nodes.Get(ctx, nodeID)
	if errors.Is(err, node.ErrNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if n.Kind != node.KindLeaf || n.WordCount > 0 {
		return false, false, nil
	}
	if strings.TrimSpace(n.Plan) != "" {
		return true, true, nil
	}
	if p.direction != nil {
		in, ok, err := p.direction.GetSceneIntent(ctx, nodeID)
		if err != nil {
			return true, false, err
		}
		if ok && strings.TrimSpace(in.Goal) != "" {
			return true, true, nil
		}
	}
	if p.projects == nil {
		return true, false, nil
	}
	proj, err := p.projects.Get(ctx, n.ProjectID)
	if err != nil {
		return true, false, err
	}
	ep, err := episode.Resolve(ctx, p.nodes, proj.OutlinePreset, nodeID)
	if errors.Is(err, episode.ErrNotFound) {
		return true, false, nil
	}
	if err != nil {
		return true, false, err
	}
	if ep.NodeID != nodeID {
		if parent, err := p.nodes.Get(ctx, ep.NodeID); err == nil && strings.TrimSpace(parent.Plan) != "" {
			return true, true, nil
		}
	}
	beat, err := p.branchBeat(ctx, n.ProjectID, proj.OutlinePreset, ep.NodeID)
	return true, beat, err
}

// branchBeat reports whether the chosen branch has a beat for episodeID. A
// beat's episode_offset counts from the episode the forecast was drawn after;
// a forecast drawn before anything was written counts from the start.
func (p planLookup) branchBeat(ctx context.Context, projectID, preset, episodeID string) (bool, error) {
	if p.forecasts == nil {
		return false, nil
	}
	f, ok, err := p.forecasts.Selected(ctx, projectID)
	if err != nil || !ok {
		return false, err
	}
	branch, ok := f.Selection()
	if !ok || len(branch.Beats) == 0 {
		return false, nil
	}
	list, err := episode.List(ctx, p.nodes, projectID, preset)
	if err != nil {
		return false, err
	}
	base, at := -1, -1
	for i, ep := range list {
		if ep.NodeID == f.BaseNodeID {
			base = i
		}
		if ep.NodeID == episodeID {
			at = i
		}
	}
	if at < 0 || (f.BaseNodeID != "" && base < 0) {
		return false, nil
	}
	for _, b := range branch.Beats {
		if b.EpisodeOffset == at-base && strings.TrimSpace(b.Summary) != "" {
			return true, nil
		}
	}
	return false, nil
}
