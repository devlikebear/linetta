package ledger

import (
	"slices"

	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/node"
)

// The foreshadowing map (#386 A): what the work owes, laid on the episode
// axis. Every point is an episode's ordinal, computed here rather than in the
// app so the desktop and iOS draw the same map.

// TimelineEpisode is one tick of the axis.
type TimelineEpisode struct {
	Ordinal int    `json:"ordinal"`
	NodeID  string `json:"node_id"`
	Label   string `json:"label"`
	Title   string `json:"title,omitempty"`
}

// TimelinePart is a top-level container and the episodes it spans.
type TimelinePart struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Title  string `json:"title,omitempty"`
	From   int    `json:"from"`
	To     int    `json:"to"`
}

// TimelinePoint is where on the axis one of an entry's scenes falls.
type TimelinePoint struct {
	Episode int    `json:"episode"`
	NodeID  string `json:"node_id"`
	Quote   string `json:"quote,omitempty"`
}

// TimelineEntry is one owed item on the map.
type TimelineEntry struct {
	ID      string         `json:"id"`
	Kind    string         `json:"kind"`
	Subject string         `json:"subject,omitempty"`
	Body    string         `json:"body"`
	Status  string         `json:"status"`
	Planted *TimelinePoint `json:"planted,omitempty"`
	Due     *TimelinePoint `json:"due,omitempty"`
	Paid    *TimelinePoint `json:"paid,omitempty"`
	// Overdue is an open entry whose due episode is already behind the
	// latest written one.
	Overdue bool `json:"overdue,omitempty"`
	// OpenFor is how many episodes an open entry has waited since it was
	// planted, counted to the latest written episode (the brief's "N화째
	// 열려 있음").
	OpenFor int `json:"open_for,omitempty"`
}

// Timeline is the whole map.
type Timeline struct {
	Episodes []TimelineEpisode `json:"episodes"`
	Parts    []TimelinePart    `json:"parts"`
	// Latest is the last episode with any prose in it; 0 for a work with none.
	Latest int `json:"latest"`
	// Current is the episode the asked-about scene is in; 0 when none was
	// asked for or it is not on the axis.
	Current int             `json:"current,omitempty"`
	Entries []TimelineEntry `json:"entries"`
}

// BuildTimeline lays entries of the given kinds (the tracked ones when kinds
// is empty) on nodes' episode axis. Superseded entries are left out: what
// replaced them is on the map instead. currentNodeID may be "".
func BuildTimeline(entries []Entry, nodes []node.Node, preset string, kinds []string, currentNodeID string) Timeline {
	if len(kinds) == 0 {
		kinds = []string{KindPromise, KindGag, KindQuestion}
	}
	// Siblings in outline order, whatever order nodes came in: episode
	// numbering follows the order it is handed.
	sorted := slices.Clone(nodes)
	slices.SortStableFunc(sorted, func(a, b node.Node) int { return a.Ordinal - b.Ordinal })
	eps := episode.FromNodes(sorted, preset)
	out := Timeline{Episodes: []TimelineEpisode{}, Parts: []TimelinePart{}, Entries: []TimelineEntry{}}
	words := episode.WordCounts(nodes)

	// Each scene's episode, then each container's last episode: a due
	// pointing at a part or an episode is due by the end of it.
	of := map[string]int{}
	for _, ep := range eps {
		out.Episodes = append(out.Episodes, TimelineEpisode{Ordinal: ep.Ordinal, NodeID: ep.NodeID, Label: ep.Label, Title: ep.Title})
		of[ep.NodeID] = ep.Ordinal
		for _, leaf := range ep.LeafIDs {
			of[leaf] = ep.Ordinal
			if words[leaf] > 0 {
				out.Latest = max(out.Latest, ep.Ordinal)
			}
		}
	}
	parent := map[string]string{}
	for _, n := range nodes {
		if n.ParentID != nil {
			parent[n.ID] = *n.ParentID
		}
	}
	last := map[string]int{}
	first := map[string]int{}
	for id, ord := range of {
		for cur, hops := id, 0; cur != "" && hops < 64; cur, hops = parent[cur], hops+1 {
			last[cur] = max(last[cur], ord)
			if f, ok := first[cur]; !ok || ord < f {
				first[cur] = ord
			}
		}
	}
	at := func(id string) int {
		if ord, ok := of[id]; ok {
			return ord
		}
		return last[id]
	}

	for _, n := range sorted {
		if n.ParentID != nil || n.Kind != node.KindContainer {
			continue
		}
		// A top-level container that is itself an episode is not a part.
		if _, isEpisode := of[n.ID]; isEpisode {
			continue
		}
		if to := last[n.ID]; to > 0 {
			out.Parts = append(out.Parts, TimelinePart{NodeID: n.ID, Label: n.Label, Title: n.Title, From: first[n.ID], To: to})
		}
	}
	if currentNodeID != "" {
		out.Current = at(currentNodeID)
	}

	point := func(nodeID, quote string) *TimelinePoint {
		if nodeID == "" {
			return nil
		}
		ord := at(nodeID)
		if ord == 0 {
			return nil
		}
		return &TimelinePoint{Episode: ord, NodeID: nodeID, Quote: quote}
	}
	for _, e := range entries {
		if e.SupersededBy != "" || !slices.Contains(kinds, e.Kind) {
			continue
		}
		te := TimelineEntry{
			ID: e.ID, Kind: e.Kind, Subject: e.Subject, Body: e.Body, Status: e.Status,
			Planted: point(e.PlantedNodeID, e.PlantedQuote),
			Due:     point(e.DueNodeID, ""),
			Paid:    point(e.PaidNodeID, e.PaidQuote),
		}
		if e.Status == StatusOpen {
			if te.Due != nil && out.Latest > 0 && te.Due.Episode < out.Latest {
				te.Overdue = true
			}
			if te.Planted != nil && out.Latest > te.Planted.Episode {
				te.OpenFor = out.Latest - te.Planted.Episode
			}
		}
		out.Entries = append(out.Entries, te)
	}
	return out
}
