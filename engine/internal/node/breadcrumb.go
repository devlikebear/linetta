package node

import "strings"

// BreadcrumbLabel renders the slash-joined ancestor path ending in n.Label,
// e.g. "1부 / 1장 / 씬 3". byID must map every ancestor's id to its Node.
func BreadcrumbLabel(byID map[string]Node, n Node) string {
	parts := []string{n.Label}
	cur := n
	for cur.ParentID != nil {
		p, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		parts = append([]string{p.Label}, parts...)
		cur = p
	}
	return strings.Join(parts, " / ")
}
