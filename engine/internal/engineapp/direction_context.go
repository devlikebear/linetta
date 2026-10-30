package engineapp

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/forecast"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

// directionBriefSource adapts *direction.Repo and *forecast.Service to
// storycontext.DirectionSource so storycontext does not import either
// (matching skillBriefSource's shape for SkillSource).
type directionBriefSource struct {
	repo     *direction.Repo
	forecast *forecast.Service
}

// FocusFor returns the work's current focus for the brief. A lookup error is
// treated as absent: the companion/story-brief rule that an optional section
// goes empty rather than aborting the whole brief.
func (s directionBriefSource) FocusFor(ctx context.Context, projectID string) (storycontext.DirectionFocus, bool, error) {
	if s.repo == nil {
		return storycontext.DirectionFocus{}, false, nil
	}
	state, err := s.repo.GetFocus(ctx, projectID)
	if err != nil {
		return storycontext.DirectionFocus{}, false, err
	}
	if !state.Present {
		return storycontext.DirectionFocus{}, false, nil
	}
	return storycontext.DirectionFocus{
		Body:          state.Body,
		Stale:         state.Stale,
		EpisodesSince: state.EpisodesSince,
	}, true, nil
}

// SceneIntentFor returns the scene's intent for the brief.
func (s directionBriefSource) SceneIntentFor(ctx context.Context, nodeID string) (storycontext.DirectionSceneIntent, bool, error) {
	if s.repo == nil {
		return storycontext.DirectionSceneIntent{}, false, nil
	}
	intent, present, err := s.repo.GetSceneIntent(ctx, nodeID)
	if err != nil {
		return storycontext.DirectionSceneIntent{}, false, err
	}
	if !present {
		return storycontext.DirectionSceneIntent{}, false, nil
	}
	return storycontext.DirectionSceneIntent{
		Goal:  intent.Goal,
		Keep:  intent.Keep,
		Avoid: intent.Avoid,
	}, true, nil
}

// PlannedBranchFor returns the project's selected forecast branch, if any
// (story-assist Phase 4). A lookup error is treated as absent, the same
// convention FocusFor/SceneIntentFor use: an optional brief section goes
// empty rather than aborting the whole brief.
func (s directionBriefSource) PlannedBranchFor(ctx context.Context, projectID string) (storycontext.DirectionPlannedBranch, bool, error) {
	if s.forecast == nil {
		return storycontext.DirectionPlannedBranch{}, false, nil
	}
	f, found, err := s.forecast.Selected(ctx, projectID)
	if err != nil {
		return storycontext.DirectionPlannedBranch{}, false, err
	}
	if !found {
		return storycontext.DirectionPlannedBranch{}, false, nil
	}
	branch, ok := f.Selection()
	if !ok {
		// Selected forecast row names a branch id that is no longer among
		// its own stored branches — should not happen (Select validates
		// this), but a dangling pointer must read as "nothing selected",
		// not surface a broken brief.
		return storycontext.DirectionPlannedBranch{}, false, nil
	}

	// A failure here must not collapse to Stale:false — that would read as
	// "checked and fresh" when it was never actually checked, the exact
	// anti-pattern already fixed in forecast.go's SelectForecast (review
	// finding f7). Unlike that RPC handler (which has already committed a
	// successful Select and so reports the failure as its own error), this
	// adapter has no successful primary action to protect: propagating the
	// error up drops the whole planned-branch section, the same "error ==
	// absent" convention Selected's own error handling just above already
	// follows, and the one the builder's call site (storycontext/
	// builder.go) already expects from every DirectionSource method.
	reasons, err := s.forecast.Stale(ctx, f)
	if err != nil {
		return storycontext.DirectionPlannedBranch{}, false, err
	}
	staleReasons := make([]storycontext.DirectionStaleReason, 0, len(reasons))
	for _, r := range reasons {
		staleReasons = append(staleReasons, storycontext.DirectionStaleReason{
			Group: r.Group, Count: r.Count, EpisodeLabels: r.EpisodeLabels,
		})
	}

	beats := make([]storycontext.DirectionPlannedBeat, 0, len(branch.Beats))
	for _, bt := range branch.Beats {
		beats = append(beats, storycontext.DirectionPlannedBeat{EpisodeOffset: bt.EpisodeOffset, Summary: bt.Summary})
	}

	return storycontext.DirectionPlannedBranch{
		Title: branch.Title, Premise: branch.Premise, Beats: beats,
		Stale: len(staleReasons) > 0, StaleReasons: staleReasons,
	}, true, nil
}
