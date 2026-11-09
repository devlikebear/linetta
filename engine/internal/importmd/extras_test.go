package importmd

import (
	"context"
	"slices"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/mdmeta"
	"github.com/devlikebear/linetta/engine/internal/project"
)

func TestRestoreProjectDetails_episodeRange(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name             string
		meta             mdmeta.ProjectMeta
		wantMin, wantTgt int
		wantMax          int
		wantRangeSkipped bool
	}{
		{
			name:    "bounds come back",
			meta:    mdmeta.ProjectMeta{EpisodeCharTarget: 5300, EpisodeCharMin: 5000, EpisodeCharMax: 6000},
			wantMin: 5000, wantTgt: 5300, wantMax: 6000,
		},
		{
			// A file exported before the bounds existed: the work keeps none,
			// even a 4,000 without-spaces target under the web novel minimum.
			name:    "file from before the bounds",
			meta:    mdmeta.ProjectMeta{EpisodeCharTarget: 4000, CharCountBasis: "without_spaces"},
			wantMin: 0, wantTgt: 4000, wantMax: 0,
		},
		{
			name:    "bounds that do not fit are skipped",
			meta:    mdmeta.ProjectMeta{EpisodeCharTarget: 5000, EpisodeCharMin: 6000},
			wantMin: 0, wantTgt: 5000, wantMax: 0,
			wantRangeSkipped: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr, _ := setupRepos(t)
			p, err := pr.Create(ctx, 1, project.NewInput{Title: "가져온 작품", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first"})
			if err != nil {
				t.Fatal(err)
			}
			res := BuildResult{Project: p}
			meta := tc.meta
			if err := RestoreProjectDetails(ctx, pr, 2, mdmeta.Metadata{Project: &meta}, &res); err != nil {
				t.Fatalf("RestoreProjectDetails: %v", err)
			}
			got := res.Project
			if got.EpisodeCharMin != tc.wantMin || got.EpisodeCharTarget != tc.wantTgt || got.EpisodeCharMax != tc.wantMax {
				t.Fatalf("rule = %d/%d/%d, want %d/%d/%d", got.EpisodeCharMin, got.EpisodeCharTarget, got.EpisodeCharMax, tc.wantMin, tc.wantTgt, tc.wantMax)
			}
			skipped := slices.Contains(res.Warnings, WarnProjectFieldSkipped+":episode_char_range")
			if skipped != tc.wantRangeSkipped {
				t.Fatalf("range skipped warning = %v, want %v (warnings %v)", skipped, tc.wantRangeSkipped, res.Warnings)
			}
		})
	}
}
