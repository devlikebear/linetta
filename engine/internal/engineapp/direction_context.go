package engineapp

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

// directionBriefSource adapts *direction.Repo to storycontext.DirectionSource
// so storycontext does not import direction (matching skillBriefSource's
// shape for SkillSource).
type directionBriefSource struct {
	repo *direction.Repo
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
