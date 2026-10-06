package style

import (
	"strings"

	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
)

// ScenesUnder returns the scenes to check, in outline order: every scene of
// the work when rootID is empty, the scene itself when rootID names one, and
// every scene beneath it when rootID names a chapter. nodes is the work's
// flat node list (node.Repo.ListByProject).
func ScenesUnder(nodes []node.Node, rootID string) []Scene {
	children := map[string][]node.Node{}
	byID := map[string]node.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
		parent := ""
		if n.ParentID != nil {
			parent = *n.ParentID
		}
		children[parent] = append(children[parent], n)
	}
	var out []Scene
	var walk func(parent string)
	walk = func(parent string) {
		for _, n := range children[parent] {
			if n.Kind == node.KindLeaf {
				out = append(out, sceneOf(n))
				continue
			}
			walk(n.ID)
		}
	}
	if root, ok := byID[rootID]; ok && root.Kind == node.KindLeaf {
		return []Scene{sceneOf(root)}
	}
	walk(rootID)
	return out
}

func sceneOf(n node.Node) Scene {
	sc := Scene{NodeID: n.ID, Label: n.Label}
	if n.Title != "" {
		sc.Label += " — " + n.Title
	}
	if n.ContentDoc == nil {
		return sc
	}
	// The plain-text copy is the scene as a reader sees it: marks dropped,
	// mentions as their names, one entry per paragraph.
	text, err := export.DocToPlainText([]byte(*n.ContentDoc))
	if err != nil || text == "" {
		return sc
	}
	sc.Paragraphs = strings.Split(text, "\n\n")
	return sc
}
