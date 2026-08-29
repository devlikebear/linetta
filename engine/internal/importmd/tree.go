package importmd

import (
	"regexp"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/mdmeta"
)

// Document is a full markdown import parse: the manuscript outline plus any
// Linetta metadata recovered from frontmatter or legacy appendices.
type Document struct {
	Outline  Outline
	Metadata mdmeta.Metadata
	Warnings []string
}

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

var headingRe = regexp.MustCompile(`^[ \t]*(#{1,6})\s+(.+?)\s*$`)
var legacyEntityLineRe = regexp.MustCompile(`^\s*-\s+\*\*(.+?)\*\*\s+\(([^)]+)\)(?:\s+·\s+([^—]+?))?(?:\s+—\s+(.+))?\s*$`)
var legacyRelationshipLineRe = regexp.MustCompile(`^\s*-\s+\*\*(.+?)\*\*\s+→\s+\*\*(.+?)\*\*\s*:\s*(.+?)(?:\s+—\s+(.+))?\s*$`)

// ParseOutline walks the markdown line by line, building the heading tree.
//
// Rules:
//   - H1 sets the Title once. Subsequent H1s demote to H2.
//   - H5+ clamps to H4.
//   - Body lines before any heading are dropped (no node to attach to).
//   - Body collected between headings attaches to the most recent heading.
func ParseOutline(text string) Outline {
	return ParseDocument(text).Outline
}

// ParseDocument strips Linetta metadata/frontmatter and visible metadata
// appendices before parsing the manuscript outline.
func ParseDocument(text string) Document {
	meta, body, err := mdmeta.ExtractFrontMatter(text)
	warnings := []string{}
	if err != nil {
		meta = mdmeta.Metadata{}
		body = text
		warnings = append(warnings, WarnFrontmatterUnreadable)
	}
	legacy, stripped := stripLinettaAppendices(body, meta.Empty())
	if meta.Empty() {
		meta.Entities = append(meta.Entities, legacy.Entities...)
		meta.Relationships = append(meta.Relationships, legacy.Relationships...)
	}
	return Document{
		Outline:  parseOutlineBody(stripped),
		Metadata: mdmeta.Normalize(meta),
		Warnings: warnings,
	}
}

func parseOutlineBody(text string) Outline {
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

func stripLinettaAppendices(text string, captureLegacy bool) (mdmeta.Metadata, string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	var meta mdmeta.Metadata
	out := make([]string, 0, len(lines))

	for i := 0; i < len(lines); {
		m := headingRe.FindStringSubmatch(lines[i])
		if m == nil || len(m[1]) != 2 {
			out = append(out, lines[i])
			i++
			continue
		}
		kind := appendixKind(m[2])
		if kind == "" {
			out = append(out, lines[i])
			i++
			continue
		}

		j := i + 1
		for j < len(lines) {
			next := headingRe.FindStringSubmatch(lines[j])
			if next != nil && len(next[1]) <= 2 {
				break
			}
			j++
		}
		section := lines[i+1 : j]
		parsed, ok := parseLegacyAppendix(kind, section)
		if ok {
			if captureLegacy {
				meta.Entities = append(meta.Entities, parsed.Entities...)
				meta.Relationships = append(meta.Relationships, parsed.Relationships...)
			}
			i = j
			continue
		}

		out = append(out, lines[i])
		i++
	}

	return mdmeta.Normalize(meta), strings.Join(out, "\n")
}

func appendixKind(label string) string {
	compact := strings.ReplaceAll(strings.TrimSpace(strings.ToLower(label)), " ", "")
	switch {
	case strings.Contains(compact, "등장인물") || strings.Contains(compact, "캐릭터") ||
		strings.Contains(compact, "인물") || strings.Contains(compact, "장소") ||
		strings.Contains(compact, "characters") || strings.Contains(compact, "places"):
		return "entities"
	case strings.Contains(compact, "관계") || strings.Contains(compact, "relationships"):
		return "relationships"
	default:
		return ""
	}
}

func parseLegacyAppendix(kind string, lines []string) (mdmeta.Metadata, bool) {
	var meta mdmeta.Metadata
	parsed := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch kind {
		case "entities":
			m := legacyEntityLineRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			meta.Entities = append(meta.Entities, mdmeta.Entity{
				Kind:    strings.TrimSpace(m[2]),
				Name:    strings.TrimSpace(m[1]),
				Role:    strings.TrimSpace(m[3]),
				Summary: strings.TrimSpace(m[4]),
			})
			parsed++
		case "relationships":
			m := legacyRelationshipLineRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			meta.Relationships = append(meta.Relationships, mdmeta.Relationship{
				FromName: strings.TrimSpace(m[1]),
				ToName:   strings.TrimSpace(m[2]),
				Label:    strings.TrimSpace(m[3]),
				Notes:    strings.TrimSpace(m[4]),
			})
			parsed++
		}
	}
	return mdmeta.Normalize(meta), parsed > 0
}
