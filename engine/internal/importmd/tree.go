package importmd

import (
	"regexp"
	"strings"
)

// Outline is the heading hierarchy extracted from a markdown document.
// Title corresponds to the document's H1 (if any).
// Roots are the H2-level nodes (with H3/H4 nested under them).
type Outline struct {
	Title string
	Roots []*OutlineNode
}

// OutlineNode is one heading + its body lines + its child headings.
// Body is the list of TiptapBlock parsed from the lines between this heading
// and the next heading (or end of input).
type OutlineNode struct {
	Level    int
	Label    string
	Body     []TiptapBlock
	Children []*OutlineNode
}

var headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)

// ParseOutline walks the markdown line by line, building the heading tree.
//
// Rules:
//   - H1 sets the Title once. Subsequent H1s demote to H2.
//   - H5+ clamps to H4.
//   - Body lines before any heading are dropped (no node to attach to).
//   - Body collected between headings attaches to the most recent heading.
func ParseOutline(text string) Outline {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var out Outline
	var stack []*OutlineNode // current ancestry; top is most recent open node
	var bodyBuf []string     // lines accumulating for current node
	titleSeen := false

	flushBody := func() {
		if len(bodyBuf) == 0 {
			return
		}
		text := strings.Join(bodyBuf, "\n")
		bodyBuf = nil
		if len(stack) == 0 {
			// No current node — drop.
			return
		}
		blocks := ParseBlocks(text)
		top := stack[len(stack)-1]
		top.Body = append(top.Body, blocks...)
	}

	pushHeading := func(level int, label string) {
		n := &OutlineNode{Level: level, Label: label}
		// Pop until top has Level < new.
		for len(stack) > 0 && stack[len(stack)-1].Level >= level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			out.Roots = append(out.Roots, n)
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, n)
		}
		stack = append(stack, n)
	}

	for _, ln := range strings.Split(text, "\n") {
		m := headingRe.FindStringSubmatch(ln)
		if m == nil {
			bodyBuf = append(bodyBuf, ln)
			continue
		}
		// Heading detected — first flush pending body to the current top node.
		flushBody()
		level := len(m[1])
		label := strings.TrimSpace(m[2])
		if level == 1 {
			if !titleSeen {
				titleSeen = true
				out.Title = label
				// H1 itself does not become a node; subsequent body goes nowhere
				// until an H2+ heading arrives.
				stack = stack[:0]
				continue
			}
			// Second H1+ demotes to H2.
			level = 2
		}
		if level > 4 {
			level = 4
		}
		pushHeading(level, label)
	}
	flushBody()
	return out
}
