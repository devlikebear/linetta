// Package submission checks a work against what a contest or serial site
// asks of the whole submission: how many episodes, how long each runs, how
// long the work runs, and whether a synopsis goes with it (episode-length
// Phase 3). It only reports. Nothing is refused for falling short: a work in
// progress is short by nature.
package submission

import (
	"strings"

	"github.com/devlikebear/linetta/engine/internal/charcount"
	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

// EpisodeLength is one episode outside the work's episode bounds.
type EpisodeLength struct {
	NodeID string `json:"node_id"`
	// SceneID is the episode's first scene, where the app opens it.
	SceneID string `json:"scene_id"`
	Label   string `json:"label"`
	Chars   int    `json:"chars"`
	// Delta is negative under the minimum, positive over the maximum.
	Delta int `json:"delta"`
}

// Report is the work measured against its submission rule.
type Report struct {
	// HasRule is whether the work sets any submission rule; a report on a
	// work with none has nothing to check.
	HasRule bool `json:"has_rule"`
	// Structured is whether the work has episodes. A short story is one
	// piece: its episode lines are left out.
	Structured bool `json:"structured"`

	// Episodes counts the episodes with text in them. A "3화" made ahead
	// with nothing written is not an episode yet.
	Episodes    int `json:"episodes"`
	MinEpisodes int `json:"min_episodes"`

	// EpisodeRule is whether the work bounds episode length at all; without
	// it Meeting, Short and Long say nothing.
	EpisodeRule bool            `json:"episode_rule"`
	Meeting     int             `json:"meeting"`
	Short       []EpisodeLength `json:"short"`
	Long        []EpisodeLength `json:"long"`

	TotalChars int    `json:"total_chars"`
	TotalMin   int    `json:"total_min"`
	TotalMax   int    `json:"total_max"`
	TotalState string `json:"total_state"`
	TotalDelta int    `json:"total_delta,omitempty"`

	NeedsSynopsis bool `json:"needs_synopsis"`
	HasSynopsis   bool `json:"has_synopsis"`

	// Ready is whether every rule the work sets is met.
	Ready bool `json:"ready"`
}

// Check measures the work whose nodes are given against p's rules.
func Check(nodes []node.Node, p project.Project) Report {
	r := Report{
		HasRule:       p.HasSubmissionRule(),
		MinEpisodes:   p.SubmissionMinEpisodes,
		TotalMin:      p.SubmissionTotalMin,
		TotalMax:      p.SubmissionTotalMax,
		NeedsSynopsis: p.SubmissionNeedsSynopsis,
		HasSynopsis:   strings.TrimSpace(p.Synopsis) != "",
		Short:         []EpisodeLength{},
		Long:          []EpisodeLength{},
	}
	for _, n := range nodes {
		if n.Kind == node.KindLeaf {
			r.TotalChars += n.WordCount
		}
	}
	total := charcount.JudgeEpisode(r.TotalChars, r.TotalMin, r.TotalMax)
	r.TotalState, r.TotalDelta = total.State, total.Delta

	eps := episode.FromNodes(nodes, p.OutlinePreset)
	r.Structured = !(len(eps) == 1 && eps[0].Whole) && len(eps) > 0
	if !r.Structured {
		if r.TotalChars > 0 {
			r.Episodes = 1
		}
	} else {
		r.EpisodeRule = p.EpisodeCharMin > 0 || p.EpisodeCharMax > 0
		counts := episode.WordCounts(nodes)
		for _, ep := range eps {
			chars := episode.Chars(ep, counts)
			if chars == 0 {
				continue
			}
			r.Episodes++
			if !r.EpisodeRule {
				continue
			}
			judged := charcount.JudgeEpisode(chars, p.EpisodeCharMin, p.EpisodeCharMax)
			row := EpisodeLength{NodeID: ep.NodeID, SceneID: ep.LeafIDs[0], Label: ep.Label, Chars: chars, Delta: judged.Delta}
			switch judged.State {
			case charcount.LengthShort:
				r.Short = append(r.Short, row)
			case charcount.LengthLong:
				r.Long = append(r.Long, row)
			default:
				r.Meeting++
			}
		}
	}

	r.Ready = r.Episodes >= r.MinEpisodes &&
		len(r.Short) == 0 && len(r.Long) == 0 &&
		r.TotalState != charcount.LengthShort && r.TotalState != charcount.LengthLong &&
		(!r.NeedsSynopsis || r.HasSynopsis)
	return r
}
