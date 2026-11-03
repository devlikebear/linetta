package continuity

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"slices"
	"strings"
)

// recentPairs is how many scenes back every sampled scene is checked against.
const recentPairs = 2

// SampleScene is one written scene as a sample sees it, in story order.
type SampleScene struct {
	NodeID  string
	Label   string
	Version int
	// Anchor marks a scene that opens the work, a volume or an arc, where the
	// premises later scenes lean on are set down.
	Anchor bool
}

// SamplePair is two scenes to compare, the later one second.
type SamplePair struct {
	Earlier SampleScene
	Later   SampleScene
}

// SamplePairs picks what to compare a scene with: the anchors before it, the
// scenes just before it, and one more, the candidate if the caller has one and
// otherwise drawn at random. Pure chance alone cannot keep up: two particular
// scenes in a thousand take hundreds of thousands of checks to meet. This asks a
// fixed few of each scene, so the work grows with the book rather than its
// square. The draw is seeded from the scene, so the same input gives the same
// pairs, and seen drops what has already been compared at these versions.
func SamplePairs(scenes []SampleScene, target string, candidate string, seen func(SamplePair) bool) []SamplePair {
	at := slices.IndexFunc(scenes, func(s SampleScene) bool { return s.NodeID == target })
	if at <= 0 {
		return nil
	}
	picked := map[int]bool{}
	for i := max(0, at-recentPairs); i < at; i++ {
		picked[i] = true
	}
	for i := range at {
		if scenes[i].Anchor {
			picked[i] = true
		}
	}
	extra := -1
	if candidate != "" {
		extra = slices.IndexFunc(scenes[:at], func(s SampleScene) bool { return s.NodeID == candidate })
	}
	if extra < 0 {
		var rest []int
		for i := range at {
			if !picked[i] {
				rest = append(rest, i)
			}
		}
		if len(rest) > 0 {
			extra = rest[sampleRand(target).IntN(len(rest))]
		}
	}
	if extra >= 0 {
		picked[extra] = true
	}

	var out []SamplePair
	for i := range at {
		if !picked[i] {
			continue
		}
		pair := SamplePair{Earlier: scenes[i], Later: scenes[at]}
		if seen == nil || !seen(pair) {
			out = append(out, pair)
		}
	}
	return out
}

// SampleRequest is what a sample pass is told: the writer's fixed rules and the
// scenes to set side by side, nothing of how the book came to be written.
func SampleRequest(projectID string, rules []string, pairs []SamplePair) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Work: %s\n", projectID)
	if len(rules) > 0 {
		b.WriteString("\nThe work's fixed rules:\n")
		for _, rule := range rules {
			b.WriteString("- " + rule + "\n")
		}
	}
	b.WriteString("\nCompare these pairs of scenes, earlier first:\n")
	for _, p := range pairs {
		fmt.Fprintf(&b, "- %s (%s) and %s (%s)\n", p.Earlier.Label, p.Earlier.NodeID, p.Later.Label, p.Later.NodeID)
	}
	return b.String()
}

// SampleCalls is the tool budget for a set of pairs: each distinct scene read
// once, and a report for each.
func SampleCalls(pairs []SamplePair) int {
	scenes := map[string]bool{}
	for _, p := range pairs {
		scenes[p.Earlier.NodeID], scenes[p.Later.NodeID] = true, true
	}
	return 2*len(scenes) + 2
}

func sampleRand(target string) *rand.Rand {
	h := fnv.New64a()
	_, _ = h.Write([]byte(target))
	return rand.New(rand.NewPCG(h.Sum64(), 0x6c696e65))
}

// PairSeen reports whether a pair was compared at these versions.
func (r *Repo) PairSeen(ctx context.Context, p SamplePair) (bool, error) {
	var n int
	err := r.s.DB().QueryRowContext(ctx, `
SELECT COUNT(*) FROM continuity_sample_pairs
 WHERE node_a = ? AND version_a = ? AND node_b = ? AND version_b = ?`,
		p.Earlier.NodeID, p.Earlier.Version, p.Later.NodeID, p.Later.Version).Scan(&n)
	return n > 0, err
}

// RecordPairs notes pairs as compared.
func (r *Repo) RecordPairs(ctx context.Context, now int64, projectID string, pairs []SamplePair) error {
	for _, p := range pairs {
		if _, err := r.s.DB().ExecContext(ctx, `
INSERT OR REPLACE INTO continuity_sample_pairs (project_id, node_a, version_a, node_b, version_b, checked_at)
VALUES (?, ?, ?, ?, ?, ?)`, projectID, p.Earlier.NodeID, p.Earlier.Version, p.Later.NodeID, p.Later.Version, now); err != nil {
			return err
		}
	}
	return nil
}
