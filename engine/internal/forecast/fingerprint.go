// Package forecast holds story-assist Phase 4: possible continuations
// ("branches") an agent drafts for the writer to compare, saved through
// linetta_save_forecast and never read by anything that builds the
// outline, the manuscript or Story World. See the README's "Where the
// story could go" section and docs/DEVELOPMENT.md's "Review group" for
// the user- and developer-facing shape of this.
package forecast

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// field separates the columns of one row before hashing; unit separates rows
// of a group; listSep separates the elements of a list-valued column
// (aliases, Keep/Avoid, an attribute map's entries) from each other. All
// three are control characters that cannot appear in the free-text fields
// being hashed (summaries, claims, aliases, …), so two different rows — or
// two different groupings of the same list elements — can never collide
// into the same bytes under concatenation. listSep is its own character,
// distinct from field/unit: reusing either of those for a nested list would
// let a list's own join reintroduce exactly the collision this separation
// scheme exists to rule out one level up (review finding f6, 2026-10-25).
const (
	field   = "\x1f"
	unit    = "\x1e"
	listSep = "\x1d"
)

// Deps is what Inputs needs to read a project's canon. Every repo is the
// same one engineapp already wires for the brief builder and MCP tools —
// forecast.Deps carries no new storage.
type Deps struct {
	Projects      *project.Repo
	Nodes         *node.Repo
	Entities      *entity.Repo
	Relationships *relationship.Repo
	Facts         *fact.Repo
	Threads       *thread.Repo
	Beats         *beat.Repo
	Direction     *direction.Repo
}

// InputHashes is one group-by-group fingerprint of a project's canon, at
// the moment Inputs was called. Nothing in here is a timestamp: the same
// canon gives the same hashes on any machine, and reverting canon to an
// earlier state reverts the hashes too.
//
// The forecasts table itself, story_proposals/story_proposal_items,
// continuity_checks/continuity_observations and the illustration tables are
// never read here — none of them is canon (phase-4-forecast.md's own
// list of what a forecast's input fingerprint excludes).
type InputHashes struct {
	Project string `json:"project"`
	Outline string `json:"outline"`
	// Episodes/Entities/Relationships/Facts are per-row, keyed by id — not a
	// single aggregate hash — so Diff can report how many rows actually
	// changed (review finding f5, 2026-10-25): an aggregate hash can only
	// say "this group differs", never "N of them".
	Episodes      map[string]string `json:"episodes"`
	Entities      map[string]string `json:"entities"`
	Relationships map[string]string `json:"relationships"`
	Facts         map[string]string `json:"facts"`
	Plot          string            `json:"plot"`
	Direction     string            `json:"direction"`
	// EpisodeLabels maps an episode's node id to its current label, for
	// Diff's stale-reason message only — it plays no part in Fingerprint,
	// which is why Episodes above holds bare hashes and not {label, hash}.
	EpisodeLabels map[string]string `json:"episode_labels,omitempty"`
}

// Fingerprint is the single sha256 a forecast row stores: a hash over every
// group hash, sorted group-name first so the result does not depend on
// struct field order. Episodes/Entities/Relationships/Facts' per-row hashes
// are folded in the same way, each keyed by its own row id.
func (h InputHashes) Fingerprint() string {
	lines := []string{
		"project" + field + h.Project,
		"outline" + field + h.Outline,
		"plot" + field + h.Plot,
		"direction" + field + h.Direction,
	}
	lines = append(lines, foldGroup("episode", h.Episodes)...)
	lines = append(lines, foldGroup("entity", h.Entities)...)
	lines = append(lines, foldGroup("relationship", h.Relationships)...)
	lines = append(lines, foldGroup("fact", h.Facts)...)
	sort.Strings(lines)
	return hashLines(lines)
}

// foldGroup turns a per-row hash map into one "prefix:id"+hash line per
// row, sorted by id so the result does not depend on map iteration order —
// Fingerprint's own convention, shared by every per-row group.
func foldGroup(prefix string, rows map[string]string) []string {
	ids := make([]string, 0, len(rows))
	for id := range rows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, prefix+":"+id+field+rows[id])
	}
	return lines
}

// changedRowIDs reports, sorted, every id present (with a hash) in either
// old or now whose hash differs between them — added, removed and changed
// rows all count, since each one row is something the writer's forecast was
// not actually drawn from. Shared by every per-row group's Diff entry.
func changedRowIDs(old, now map[string]string) []string {
	changed := map[string]bool{}
	for id, h := range old {
		if now[id] != h {
			changed[id] = true
		}
	}
	for id, h := range now {
		if old[id] != h {
			changed[id] = true
		}
	}
	ids := make([]string, 0, len(changed))
	for id := range changed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// StaleReason names one group of canon that changed between two
// fingerprints, phrased for the writer: "characters" with Count 2, or
// "episodes" with the changed episodes' own labels.
type StaleReason struct {
	Group         string   `json:"group"`
	Count         int      `json:"count,omitempty"`
	EpisodeLabels []string `json:"episode_labels,omitempty"`
}

// Diff reports what changed between a forecast's saved input hashes and the
// project's current ones. List is recomputed every time it is called — a
// forecast that looks stale now and is re-checked after canon reverts to
// what it was drawn from reports no reasons at all.
func Diff(old, now InputHashes) []StaleReason {
	var reasons []StaleReason
	group := func(name, a, b string) {
		if a != b {
			reasons = append(reasons, StaleReason{Group: name, Count: 1})
		}
	}
	group("project", old.Project, now.Project)
	group("outline", old.Outline, now.Outline)
	group("plot", old.Plot, now.Plot)
	group("direction", old.Direction, now.Direction)

	rowGroup := func(name string, oldRows, nowRows map[string]string) {
		if ids := changedRowIDs(oldRows, nowRows); len(ids) > 0 {
			reasons = append(reasons, StaleReason{Group: name, Count: len(ids)})
		}
	}
	rowGroup("entities", old.Entities, now.Entities)
	rowGroup("relationships", old.Relationships, now.Relationships)
	rowGroup("facts", old.Facts, now.Facts)

	changedEpisodes := map[string]bool{}
	for id, h := range old.Episodes {
		if now.Episodes[id] != h {
			changedEpisodes[id] = true
		}
	}
	for id, h := range now.Episodes {
		if old.Episodes[id] != h {
			changedEpisodes[id] = true
		}
	}
	if len(changedEpisodes) > 0 {
		ids := make([]string, 0, len(changedEpisodes))
		for id := range changedEpisodes {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		labels := make([]string, 0, len(ids))
		for _, id := range ids {
			label := now.EpisodeLabels[id]
			if label == "" {
				label = old.EpisodeLabels[id]
			}
			if label == "" {
				label = id
			}
			labels = append(labels, label)
		}
		reasons = append(reasons, StaleReason{Group: "episodes", Count: len(ids), EpisodeLabels: labels})
	}
	return reasons
}

// Inputs hashes exactly what a forecast was drawn from, group by group, so
// a stale forecast can say what changed. No timestamps: the same canon
// gives the same hashes on any machine. The forecasts table itself is
// never an input.
func Inputs(ctx context.Context, deps Deps, projectID string) (InputHashes, error) {
	var out InputHashes

	proj, err := deps.Projects.Get(ctx, projectID)
	if err != nil {
		return InputHashes{}, err
	}
	genres := append([]string{}, proj.Genres...)
	sort.Strings(genres)
	out.Project = hashLines([]string{
		"synopsis" + field + proj.Synopsis,
		"outline" + field + proj.Outline,
		"style_notes" + field + proj.StyleNotes,
		// Genres/DefaultPOV/LengthTarget feed the brief exactly the same way
		// Synopsis/Outline/StyleNotes above do (storycontext/builder.go
		// renders all six into the Project section) — missing them here let
		// a writer change the project's genre or POV without the forecast
		// drawn from the old one ever going stale (review finding f4,
		// 2026-10-25).
		"genres" + field + strings.Join(genres, listSep),
		"default_pov" + field + proj.DefaultPOV,
		"length_target" + field + proj.LengthTarget,
	})

	nodes, err := deps.Nodes.ListByProject(ctx, projectID)
	if err != nil {
		return InputHashes{}, err
	}
	nodesByID := make(map[string]node.Node, len(nodes))
	outlineLines := make([]string, 0, len(nodes))
	for _, n := range nodes {
		nodesByID[n.ID] = n
		parent := ""
		if n.ParentID != nil {
			parent = *n.ParentID
		}
		outlineLines = append(outlineLines, strings.Join([]string{
			n.ID, parent, strconv.Itoa(n.Ordinal), n.Kind, n.Label, n.Title, n.Summary,
		}, field))
	}
	out.Outline = hashLines(outlineLines)

	eps, err := episode.List(ctx, deps.Nodes, projectID, proj.OutlinePreset)
	if err != nil {
		return InputHashes{}, err
	}
	out.Episodes = make(map[string]string, len(eps))
	out.EpisodeLabels = make(map[string]string, len(eps))
	for _, ep := range eps {
		leafLines := make([]string, 0, len(ep.LeafIDs))
		for _, leafID := range ep.LeafIDs {
			n := nodesByID[leafID]
			doc := ""
			if n.ContentDoc != nil {
				doc = *n.ContentDoc
			}
			leafLines = append(leafLines, leafID+field+doc)
		}
		out.Episodes[ep.NodeID] = hashLines(leafLines)
		out.EpisodeLabels[ep.NodeID] = ep.Label
	}

	entities, err := deps.Entities.ListByProject(ctx, projectID)
	if err != nil {
		return InputHashes{}, err
	}
	out.Entities = make(map[string]string, len(entities))
	for _, e := range entities {
		aliases := append([]string{}, e.Aliases...)
		sort.Strings(aliases)
		out.Entities[e.ID] = hashLine(strings.Join([]string{
			e.ID, e.Kind, e.Name, strings.Join(aliases, listSep), e.Role, e.Summary, hashAttrs(e.Attributes),
		}, field))
	}

	rels, err := deps.Relationships.ListByProject(ctx, projectID)
	if err != nil {
		return InputHashes{}, err
	}
	out.Relationships = make(map[string]string, len(rels))
	for _, rel := range rels {
		out.Relationships[rel.ID] = hashLine(strings.Join([]string{
			rel.ID, rel.FromID, rel.ToID, rel.Label, rel.Notes,
		}, field))
	}

	facts, err := deps.Facts.List(ctx, fact.ListFilter{ProjectID: projectID, Limit: allFacts})
	if err != nil {
		return InputHashes{}, err
	}
	out.Facts = make(map[string]string, len(facts))
	for _, f := range facts {
		out.Facts[f.ID] = hashLine(strings.Join([]string{f.ID, f.Claim, f.Result, f.Status}, field))
	}

	threads, err := deps.Threads.ListByProject(ctx, projectID, true)
	if err != nil {
		return InputHashes{}, err
	}
	plotLines := make([]string, 0, len(threads))
	for _, th := range threads {
		closed := ""
		if th.ClosedAt != nil {
			closed = strconv.FormatInt(*th.ClosedAt, 10)
		}
		plotLines = append(plotLines, strings.Join([]string{
			"thread", th.ID, th.Name, th.Color, th.Summary, closed,
		}, field))
		beats, err := deps.Beats.ListByThread(ctx, th.ID)
		if err != nil {
			return InputHashes{}, err
		}
		for _, b := range beats {
			bnode := ""
			if b.NodeID != nil {
				bnode = *b.NodeID
			}
			plotLines = append(plotLines, strings.Join([]string{
				"beat", b.ID, th.ID, bnode, strconv.Itoa(b.Ordinal), b.Label, b.Description, strconv.Itoa(b.Intensity),
			}, field))
		}
	}
	out.Plot = hashLines(plotLines)

	directionLines := []string{}
	focus, err := deps.Direction.GetFocus(ctx, projectID)
	if err != nil {
		return InputHashes{}, err
	}
	if focus.Present {
		directionLines = append(directionLines, strings.Join([]string{
			"focus", focus.Body, focus.AnchorNodeID,
		}, field))
	}
	intents, err := deps.Direction.ListSceneIntents(ctx, projectID)
	if err != nil {
		return InputHashes{}, err
	}
	for _, si := range intents {
		keep := append([]string{}, si.Keep...)
		sort.Strings(keep)
		avoid := append([]string{}, si.Avoid...)
		sort.Strings(avoid)
		directionLines = append(directionLines, strings.Join([]string{
			"intent", si.NodeID, si.Goal, strings.Join(keep, listSep), strings.Join(avoid, listSep),
		}, field))
	}
	out.Direction = hashLines(directionLines)

	return out, nil
}

// allFacts is fact.Repo.List's own ceiling: anything over 100 (or <= 0)
// resets its limit to 50, so 100 is the largest set List can ever return.
// Inputs hashes exactly that set, not a smaller slice of it.
const allFacts = 100

func hashAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+attrs[k])
	}
	return strings.Join(parts, listSep)
}

// hashLines sorts lines (so the result does not depend on the order rows
// came back from SQLite in) and sha256's the joined, length-delimited
// bytes — length-delimited so no row's own content, however it is
// punctuated, can be mistaken for the separator between rows.
func hashLines(lines []string) string {
	sorted := append([]string{}, lines...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, l := range sorted {
		h.Write([]byte(l))
		h.Write([]byte(unit))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// hashLine is hashLines for exactly one row — a per-row entry in a group
// keyed by id (Entities/Relationships/Facts), where "sort many lines, then
// sha256" would be wasted work and would needlessly store the row's own
// free text in the stored input_hashes_json blob instead of a fixed-size
// digest.
func hashLine(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
