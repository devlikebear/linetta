package importmd

import "strings"

// PreviewNode is a serializable mirror of the tree that BuildProject would
// produce. Kind is "container" or "leaf". Used by the frontend preview modal.
type PreviewNode struct {
	Label    string         `json:"label"`
	Kind     string         `json:"kind"`
	Children []*PreviewNode `json:"children,omitempty"`
}

// PreviewResult is the read-only summary of what an import would create.
// No database mutation. Mirrors BuildResult's counts so the UI can show
// the same totals before and after.
type PreviewResult struct {
	Title          string         `json:"title"`
	ContainerCount int            `json:"container_count"`
	LeafCount      int            `json:"leaf_count"`
	Warnings       []string       `json:"warnings"`
	Roots          []*PreviewNode `json:"roots"`
}

// Preview converts a parsed Outline into a PreviewResult. fallbackFileName is
// used to derive a title when outline.Title is empty (mirrors BuildProject).
// Pure function, no I/O.
func Preview(outline Outline, fallbackFileName string) PreviewResult {
	title := outline.Title
	if title == "" {
		title = stripMarkdownExt(fallbackFileName)
	}
	if title == "" {
		title = "가져온 작품"
	}

	res := PreviewResult{Title: title}
	if len(outline.Roots) == 0 {
		res.Warnings = append(res.Warnings, WarnNoHeadings)
		return res
	}
	for _, r := range outline.Roots {
		res.Roots = append(res.Roots, walkPreview(r, &res))
	}
	return res
}

func walkPreview(n *OutlineNode, res *PreviewResult) *PreviewNode {
	hasChildren := len(n.Children) > 0
	hasBody := len(n.Body) > 0
	kind := "leaf"
	if hasChildren {
		kind = "container"
	}
	pn := &PreviewNode{Label: n.Label, Kind: kind}
	if kind == "container" {
		res.ContainerCount++
	} else {
		res.LeafCount++
	}
	if hasChildren && hasBody {
		// Synthetic 씬 1 leaf — same as BuildProject does.
		pn.Children = append(pn.Children, &PreviewNode{Label: "씬 1", Kind: "leaf"})
		res.LeafCount++
	}
	for _, c := range n.Children {
		pn.Children = append(pn.Children, walkPreview(c, res))
	}
	return pn
}

func stripMarkdownExt(name string) string {
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	for _, suf := range []string{".markdown", ".md"} {
		if strings.HasSuffix(lower, suf) {
			return name[:len(name)-len(suf)]
		}
	}
	return name
}
