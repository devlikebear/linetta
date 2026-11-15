package node

// ReadingOrder returns the scenes of a work in the order a reader meets them:
// the outline walked depth first, children in the order all lists them, which
// is the order ListByProject returns.
func ReadingOrder(all []Node) []Node {
	children := map[string][]Node{}
	for _, n := range all {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	var leaves []Node
	var walk func(string)
	walk = func(parent string) {
		for _, c := range children[parent] {
			if c.Kind == KindLeaf {
				leaves = append(leaves, c)
			}
			walk(c.ID)
		}
	}
	walk("")
	return leaves
}
