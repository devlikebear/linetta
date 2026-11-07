package ledger

import (
	"cmp"
	"context"
)

// Knowledge is who knows what at a scene. The reader's share is worked out from
// the questions the story has paid, never recorded apart: a second record of
// one reveal could disagree with the first.
type Knowledge struct {
	// Reader holds the questions paid by this scene. The scene that pays one is
	// the one that tells the reader, so it counts.
	Reader []Entry `json:"reader"`
	// Secrets holds the questions asked by this scene and not yet paid whose
	// answer the writer has written down.
	Secrets []Entry `json:"secrets"`
	// People holds, for each subject, the last knows fact about each question
	// up to this scene, keyed by the question's id.
	People map[string]map[string]Entry `json:"people"`
}

// KnowsAt reads the ledger at a scene. With no subjects it covers everyone the
// ledger has a knows fact for. Knows facts without a question are free notes
// and are left out.
func (r *Repo) KnowsAt(ctx context.Context, projectID, nodeID string, subjects []string) (Knowledge, error) {
	all, err := r.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return Knowledge{}, err
	}
	t := newTree(all)
	cur, onStory := t.index[nodeID]
	if !onStory {
		cur = len(t.leaves)
	}
	entries, err := r.List(ctx, Filter{ProjectID: projectID, Kinds: []string{KindQuestion, KindKnowledge}})
	if err != nil {
		return Knowledge{}, err
	}
	wanted := map[string]bool{}
	for _, s := range subjects {
		wanted[r.canonical(ctx, Entry{ProjectID: projectID, Kind: KindKnowledge, Subject: s})] = true
	}

	out := Knowledge{Reader: []Entry{}, Secrets: []Entry{}, People: map[string]map[string]Entry{}}
	for _, e := range entries {
		switch {
		case e.Kind == KindQuestion:
			switch {
			case e.Status == StatusDropped || t.after(e.PlantedNodeID, cur):
			case e.Status == StatusPaid && !t.after(e.PaidNodeID, cur):
				out.Reader = append(out.Reader, e)
			case e.Answer != "":
				out.Secrets = append(out.Secrets, e)
			}
		case e.Attribute == AttributeKnows && e.About != "" && e.Status == StatusOpen:
			if t.after(e.PlantedNodeID, cur) || (len(wanted) > 0 && !wanted[e.Subject]) {
				continue
			}
			byQuestion := out.People[e.Subject]
			if byQuestion == nil {
				byQuestion = map[string]Entry{}
				out.People[e.Subject] = byQuestion
			}
			if prior, ok := byQuestion[e.About]; !ok || t.storyOrder(e, prior) >= 0 {
				byQuestion[e.About] = e
			}
		}
	}
	return out, nil
}

// storyOrder compares where two entries were planted, and when they were
// recorded when that is the same scene.
func (t tree) storyOrder(a, b Entry) int {
	return cmp.Or(cmp.Compare(t.plantedAt(a), t.plantedAt(b)), cmp.Compare(a.CreatedAt, b.CreatedAt))
}
