package direction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// Repo stores a work's direction in library.db, so it travels with the
// backup (restore.MergeProject carries project_focus and scene_intents).
type Repo struct {
	s        *store.Store
	nodes    *node.Repo
	projects *project.Repo
}

func NewRepo(s *store.Store, nodes *node.Repo, projects *project.Repo) *Repo {
	return &Repo{s: s, nodes: nodes, projects: projects}
}

// GetFocus returns the work's current focus and whether it looks stale.
func (r *Repo) GetFocus(ctx context.Context, projectID string) (FocusState, error) {
	p, err := r.projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return FocusState{}, ErrProjectNotFound
		}
		return FocusState{}, err
	}

	var (
		f      = Focus{ProjectID: projectID}
		anchor sql.NullString
	)
	err = r.s.DB().QueryRowContext(ctx,
		"SELECT body, anchor_node_id, set_at FROM project_focus WHERE project_id = ?", projectID).
		Scan(&f.Body, &anchor, &f.SetAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FocusState{}, nil
	}
	if err != nil {
		return FocusState{}, err
	}
	if anchor.Valid {
		f.AnchorNodeID = anchor.String
	}
	state := FocusState{Focus: f, Present: true}

	last, found, err := episode.LastWritten(ctx, r.nodes, projectID, p.OutlinePreset)
	if err != nil {
		return FocusState{}, err
	}

	if f.AnchorNodeID == "" {
		// No anchor was recorded because nothing had been written yet when
		// the focus was set (SetFocus leaves anchor_node_id NULL in that
		// case — never because an anchor was deleted; deletion leaves a
		// dangling id, handled below). Treat the anchor as episode 0.
		if found {
			state.EpisodesSince = last.Ordinal
		}
		state.Stale = state.EpisodesSince >= FocusStaleAfterEpisodes
		return state, nil
	}

	anchorEp, err := episode.Resolve(ctx, r.nodes, p.OutlinePreset, f.AnchorNodeID)
	if errors.Is(err, episode.ErrNotFound) {
		state.EpisodesSince = -1
		state.Stale = true
		return state, nil
	}
	if err != nil {
		return FocusState{}, err
	}
	if !found {
		// Nothing written yet anywhere: the anchor episode is still the
		// newest word, so the focus cannot have gone stale.
		return state, nil
	}
	state.EpisodesSince = last.Ordinal - anchorEp.Ordinal
	state.Stale = state.EpisodesSince >= FocusStaleAfterEpisodes
	return state, nil
}

// SetFocus replaces the work's focus. An empty body clears it — no focus is
// a valid state, not an error. The anchor episode is the engine's choice
// (the last episode with words in it right now), never the caller's:
// clients do not send anchor_node_id. Calling SetFocus again with the same
// body is how the writer says "still valid" — it re-anchors to the episode
// that is newest right now.
func (r *Repo) SetFocus(ctx context.Context, now int64, projectID, body string) (FocusState, error) {
	body = strings.TrimSpace(body)
	p, err := r.projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return FocusState{}, ErrProjectNotFound
		}
		return FocusState{}, err
	}

	if body == "" {
		if _, err := r.s.DB().ExecContext(ctx, "DELETE FROM project_focus WHERE project_id = ?", projectID); err != nil {
			return FocusState{}, err
		}
		return FocusState{}, nil
	}
	if utf8.RuneCountInString(body) > MaxFocusRunes {
		return FocusState{}, ErrFocusTooLong
	}

	last, found, err := episode.LastWritten(ctx, r.nodes, projectID, p.OutlinePreset)
	if err != nil {
		return FocusState{}, err
	}
	var anchor any
	if found {
		anchor = last.NodeID
	}
	if _, err := r.s.DB().ExecContext(ctx, `INSERT INTO project_focus (project_id, body, anchor_node_id, set_at) VALUES (?,?,?,?)
ON CONFLICT(project_id) DO UPDATE SET body=excluded.body, anchor_node_id=excluded.anchor_node_id, set_at=excluded.set_at`,
		projectID, body, anchor, now); err != nil {
		return FocusState{}, err
	}
	return r.GetFocus(ctx, projectID)
}

// GetSceneIntent returns a scene's intent and whether it has one.
func (r *Repo) GetSceneIntent(ctx context.Context, nodeID string) (SceneIntent, bool, error) {
	si := SceneIntent{NodeID: nodeID}
	var keepJSON, avoidJSON string
	err := r.s.DB().QueryRowContext(ctx,
		"SELECT project_id, goal, keep_json, avoid_json, updated_at FROM scene_intents WHERE node_id = ?", nodeID).
		Scan(&si.ProjectID, &si.Goal, &keepJSON, &avoidJSON, &si.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SceneIntent{}, false, nil
	}
	if err != nil {
		return SceneIntent{}, false, err
	}
	si.Keep = decodeList(keepJSON)
	si.Avoid = decodeList(avoidJSON)
	return si, true, nil
}

func decodeList(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

// SetSceneIntent replaces one scene's intent. A node that is not a leaf
// (not a scene) is rejected: this is a scene-unit concept in the MVP, not an
// episode-container one. goal, keep and avoid all empty clears the row — no
// intent is a valid state.
func (r *Repo) SetSceneIntent(ctx context.Context, now int64, in SceneIntent) (SceneIntent, error) {
	n, err := r.nodes.Get(ctx, in.NodeID)
	if err != nil {
		if errors.Is(err, node.ErrNotFound) {
			return SceneIntent{}, ErrNodeNotFound
		}
		return SceneIntent{}, err
	}
	if n.Kind != node.KindLeaf {
		return SceneIntent{}, ErrNotScene
	}

	goal := strings.TrimSpace(in.Goal)
	if utf8.RuneCountInString(goal) > MaxGoalRunes {
		return SceneIntent{}, ErrGoalTooLong
	}
	keep, err := normalizeList(in.Keep)
	if err != nil {
		return SceneIntent{}, err
	}
	avoid, err := normalizeList(in.Avoid)
	if err != nil {
		return SceneIntent{}, err
	}

	if goal == "" && len(keep) == 0 && len(avoid) == 0 {
		if _, err := r.s.DB().ExecContext(ctx, "DELETE FROM scene_intents WHERE node_id = ?", in.NodeID); err != nil {
			return SceneIntent{}, err
		}
		return SceneIntent{NodeID: in.NodeID, ProjectID: n.ProjectID, Keep: []string{}, Avoid: []string{}}, nil
	}

	keepJSON, err := json.Marshal(keep)
	if err != nil {
		return SceneIntent{}, err
	}
	avoidJSON, err := json.Marshal(avoid)
	if err != nil {
		return SceneIntent{}, err
	}
	out := SceneIntent{NodeID: in.NodeID, ProjectID: n.ProjectID, Goal: goal, Keep: keep, Avoid: avoid, UpdatedAt: now}
	_, err = r.s.DB().ExecContext(ctx, `INSERT INTO scene_intents (node_id, project_id, goal, keep_json, avoid_json, updated_at) VALUES (?,?,?,?,?,?)
ON CONFLICT(node_id) DO UPDATE SET goal=excluded.goal, keep_json=excluded.keep_json, avoid_json=excluded.avoid_json, updated_at=excluded.updated_at`,
		out.NodeID, out.ProjectID, out.Goal, string(keepJSON), string(avoidJSON), out.UpdatedAt)
	if err != nil {
		return SceneIntent{}, err
	}
	return out, nil
}
