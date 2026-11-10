package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// KnowsScene is one scene the writer can look at the work from, in story order.
type KnowsScene struct {
	NodeID      string `json:"node_id"`
	Label       string `json:"label"`
	ParentLabel string `json:"parent_label,omitempty"`
	// Flashback is set on a scene the writer marked as one.
	Flashback bool `json:"flashback,omitempty"`
}

// KnowsSecret is a question still open at the scene whose answer the writer
// has written down. The answer itself is left out: the table is about who
// knows it, and the writer reads the answer on the question.
type KnowsSecret struct {
	ID      string `json:"id"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body"`
	// PaidNodeID is where the reader learns it, when that is later in the story.
	PaidNodeID string `json:"paid_node_id,omitempty"`
}

// KnowsCell is the last knows fact a character has about a secret by the scene.
type KnowsCell struct {
	// Knows is true for 앎 (value 1) and false for 모름 (value 0).
	Knows  bool   `json:"knows"`
	NodeID string `json:"node_id,omitempty"`
}

// KnowsRow is one character in the scene.
type KnowsRow struct {
	Subject  string               `json:"subject"`
	EntityID string               `json:"entity_id"`
	Cells    map[string]KnowsCell `json:"cells"`
}

// KnowsAt returns a handler for ledger.knows_at: the work's scenes, and at the
// chosen one the open secrets and what each character in it knows of them.
// The characters are the ones the scene mentions — the same appearance index
// the brief reads for the scene's elements.
func KnowsAt(repo *ledger.Repo, nodes *node.Repo, mentions *mention.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
			NodeID    string `json:"node_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		all, err := nodes.ListByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, err
		}
		scenes := storyScenes(all)
		out := struct {
			Scenes  []KnowsScene  `json:"scenes"`
			NodeID  string        `json:"node_id"`
			Secrets []KnowsSecret `json:"secrets"`
			Rows    []KnowsRow    `json:"rows"`
		}{Scenes: scenes, Secrets: []KnowsSecret{}, Rows: []KnowsRow{}}

		onStory := false
		for _, s := range scenes {
			if s.NodeID == in.NodeID {
				onStory = true
				break
			}
		}
		// A container or an unknown id would read as the end of the story,
		// which is not the scene the writer picked.
		if !onStory {
			return json.Marshal(out)
		}
		out.NodeID = in.NodeID

		ents, err := mentions.ListEntitiesForNode(ctx, in.NodeID)
		if err != nil {
			return nil, err
		}
		var cast []entity.Entity
		names := []string{}
		for _, e := range ents {
			if e.Kind == entity.KindCharacter {
				cast = append(cast, e)
				names = append(names, e.Name)
			}
		}
		if len(names) == 0 {
			// KnowsAt reads an empty subject list as everyone; a scene with no
			// one in it should not borrow the whole work's cast.
			names = []string{"\x00"}
		}
		k, err := repo.KnowsAt(ctx, in.ProjectID, in.NodeID, names)
		if err != nil {
			return nil, ledgerError(err)
		}
		for _, q := range k.Secrets {
			s := KnowsSecret{ID: q.ID, Subject: q.Subject, Body: q.Body}
			if q.Status == ledger.StatusPaid {
				s.PaidNodeID = q.PaidNodeID
			}
			out.Secrets = append(out.Secrets, s)
		}
		for _, e := range cast {
			row := KnowsRow{Subject: e.Name, EntityID: e.ID, Cells: map[string]KnowsCell{}}
			for _, q := range k.Secrets {
				fact, ok := k.People[e.Name][q.ID]
				if !ok || fact.ValueNum == nil {
					continue
				}
				row.Cells[q.ID] = KnowsCell{Knows: *fact.ValueNum == ledger.KnowsYes, NodeID: fact.PlantedNodeID}
			}
			out.Rows = append(out.Rows, row)
		}
		return json.Marshal(out)
	}
}

// storyScenes lists the work's scenes in story order: a parent's children by
// ordinal, depth first, the order the ledger reads the story in.
func storyScenes(all []node.Node) []KnowsScene {
	byID := make(map[string]node.Node, len(all))
	children := map[string][]node.Node{}
	for _, n := range all {
		byID[n.ID] = n
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	out := []KnowsScene{}
	var walk func(string, int)
	walk = func(parent string, depth int) {
		if depth > 64 {
			return
		}
		for _, c := range children[parent] {
			if c.Kind == node.KindLeaf {
				s := KnowsScene{NodeID: c.ID, Label: c.Label, Flashback: c.Flashback}
				if c.ParentID != nil {
					s.ParentLabel = byID[*c.ParentID].Label
				}
				out = append(out, s)
			}
			walk(c.ID, depth+1)
		}
	}
	walk("", 0)
	return out
}
