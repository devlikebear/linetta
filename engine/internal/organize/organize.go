// Package organize keeps the record behind the organize pass (#387): which
// scenes of an episode the writer's own prose has changed since a pass last
// read them, and how each pass went. The pass itself runs in the agent; it
// only proposes, into the suggestions inbox, and changes nothing.
package organize

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// MinSceneRunes is the shortest scene worth reading: the summariser's floor.
const MinSceneRunes = 60

// MinAutoChange is how much a scene's length must have moved since it was
// last read before a pass that runs on its own reads it again: the
// summariser's floor for a change worth a call. A request reads any change.
const MinAutoChange = 300

// MaxScenes caps one pass: an episode is a few scenes, and a pass over more
// would cost more than the writer expects from one request.
const MaxScenes = 8

// How a pass over a scene ended. Only done keeps it from being read again.
const (
	OutcomeDone    = "done"
	OutcomeTimeout = "timeout"
	OutcomeFailed  = "failed"
)

// ErrNotInEpisode is a node no episode of the work holds.
var ErrNotInEpisode = errors.New("organize: the scene is not in an episode of this work")

// Scene is one scene a pass is to read, as it stands now.
type Scene struct {
	NodeID  string `json:"node_id"`
	Version int    `json:"-"`
	Hash    string `json:"-"`
	Chars   int    `json:"-"`
	// LedgerSettled says the settlement pass already took this version into
	// the ledger: the organize pass leaves its foreshadowing alone.
	LedgerSettled bool `json:"ledger_settled,omitempty"`
}

// Plan is what a pass over one episode would read.
type Plan struct {
	ProjectID     string  `json:"project_id"`
	EpisodeNodeID string  `json:"episode_node_id"`
	EpisodeLabel  string  `json:"episode_label"`
	Scenes        []Scene `json:"scenes"`
	// Unchanged counts the scenes already read as they are; Short the ones
	// too short to read; Small, on a pass that runs on its own, the ones
	// changed by less than MinAutoChange.
	Unchanged int `json:"unchanged"`
	Short     int `json:"short"`
	Small     int `json:"small,omitempty"`
}

// Repo is the record.
type Repo struct {
	s        *store.Store
	nodes    *node.Repo
	projects *project.Repo
}

func NewRepo(s *store.Store, nodes *node.Repo, projects *project.Repo) *Repo {
	return &Repo{s: s, nodes: nodes, projects: projects}
}

// Plan finds the episode nodeID is in (a scene or the episode itself) and the
// scenes of it the writer has changed since a pass last finished them. force
// reads them all again. minChange, for a pass that runs on its own, also
// leaves out a scene whose length moved by less than that since it was last
// read (a scene never read counts its whole length).
func (r *Repo) Plan(ctx context.Context, projectID, nodeID string, force bool, minChange int) (Plan, error) {
	p, err := r.projects.Get(ctx, projectID)
	if err != nil {
		return Plan{}, err
	}
	all, err := r.nodes.ListByProject(ctx, p.ID)
	if err != nil {
		return Plan{}, err
	}
	sorted := slices.Clone(all)
	slices.SortStableFunc(sorted, func(a, b node.Node) int { return a.Ordinal - b.Ordinal })
	byID := map[string]node.Node{}
	for _, n := range all {
		byID[n.ID] = n
	}
	var ep *episode.Episode
	for _, e := range episode.FromNodes(sorted, p.OutlinePreset) {
		if e.NodeID == nodeID || slices.Contains(e.LeafIDs, nodeID) {
			ep = &e
			break
		}
	}
	if ep == nil {
		return Plan{}, ErrNotInEpisode
	}
	plan := Plan{ProjectID: p.ID, EpisodeNodeID: ep.NodeID, EpisodeLabel: ep.Label, Scenes: []Scene{}}
	if ep.Title != "" && ep.Label == "" {
		plan.EpisodeLabel = ep.Title
	}
	for _, id := range ep.LeafIDs {
		n := byID[id]
		text := ""
		if n.ContentDoc != nil {
			text = manuscript.DocToPlainText(*n.ContentDoc)
		}
		if utf8.RuneCountInString(text) < MinSceneRunes {
			plan.Short++
			continue
		}
		sc := Scene{NodeID: id, Version: n.ContentVersion, Hash: ledger.ProseHash(n.ContentDoc), Chars: utf8.RuneCountInString(text)}
		if !force {
			done, readChars, err := r.lastDone(ctx, id, sc.Hash)
			if err != nil {
				return Plan{}, err
			}
			if done {
				plan.Unchanged++
				continue
			}
			if minChange > 0 && abs(sc.Chars-readChars) < minChange {
				plan.Small++
				continue
			}
		}
		if sc.LedgerSettled, err = r.settled(ctx, id, n.ContentVersion); err != nil {
			return Plan{}, err
		}
		if len(plan.Scenes) < MaxScenes {
			plan.Scenes = append(plan.Scenes, sc)
		}
	}
	return plan, nil
}

// lastDone reports whether the scene's words, hash, were already read to the
// end, and how long the prose was when a pass last finished it (0: never).
func (r *Repo) lastDone(ctx context.Context, nodeID, hash string) (same bool, chars int, err error) {
	var readHash string
	err = r.s.DB().QueryRowContext(ctx,
		`SELECT text_hash, char_count FROM scene_organize WHERE node_id = ? AND outcome = 'done'`,
		nodeID).Scan(&readHash, &chars)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return readHash != "" && readHash == hash, chars, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (r *Repo) settled(ctx context.Context, nodeID string, version int) (bool, error) {
	var n int
	err := r.s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scene_settlements WHERE node_id = ? AND outcome = 'done' AND content_version = ?`,
		nodeID, version).Scan(&n)
	return n > 0, err
}

// Record keeps how a pass over sc ended. A done on record for the same words
// is not overwritten by a later pass that did not finish.
func (r *Repo) Record(ctx context.Context, projectID string, sc Scene, outcome string, now int64) error {
	if outcome != OutcomeDone && outcome != OutcomeTimeout && outcome != OutcomeFailed {
		return fmt.Errorf("organize: unknown outcome %q", outcome)
	}
	_, err := r.s.DB().ExecContext(ctx, `
INSERT INTO scene_organize (node_id, project_id, content_version, text_hash, outcome, organized_at, char_count)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(node_id) DO UPDATE SET
  project_id = excluded.project_id,
  content_version = excluded.content_version,
  text_hash = excluded.text_hash,
  outcome = excluded.outcome,
  organized_at = excluded.organized_at,
  char_count = excluded.char_count
WHERE NOT (scene_organize.outcome = 'done'
           AND scene_organize.text_hash = excluded.text_hash
           AND excluded.outcome <> 'done')`,
		sc.NodeID, projectID, sc.Version, sc.Hash, outcome, now, sc.Chars)
	return err
}
