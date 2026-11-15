// Package manuscriptir turns a work's outline and its scenes' Tiptap documents
// into one format-neutral list of blocks, which the DOCX and TXT writers (and
// HWPX later, #383) each write out. A writer decides how a block looks; this
// package decides what the blocks are, so every format agrees on what the
// manuscript says and on what never leaves it — mention ids, note markers and
// the notes behind them.
package manuscriptir

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/illustref"
	"github.com/devlikebear/linetta/engine/internal/node"
)

// Doc is one export: the work's title and its sections in reading order.
type Doc struct {
	Title    string
	Sections []Section
}

// Section is one outline container (a part, an episode, a chapter) with the
// prose of the scenes directly under it, or, for scenes that sit at the top of
// the outline or a single scene exported alone, those scenes with no heading.
type Section struct {
	// Heading is the container's label and title as the reader should see
	// them; "" for a section with none.
	Heading string
	// Level is the heading's depth, 1 for a top-level container.
	Level int
	// HoldsScenes is true when the container has scenes of its own: the unit
	// a writer means by "a new page for each episode".
	HoldsScenes bool
	Blocks      []Block
}

// BlockKind is what a block is.
type BlockKind int

const (
	Paragraph BlockKind = iota
	// Heading is a heading the writer put inside a scene; Level is 1-6.
	Heading
	Quote
	// SceneBreak separates two scenes of one section, or stands where the
	// writer put a horizontal rule.
	SceneBreak
	// ListItem is one list entry; Level is its nesting depth from 1 and
	// Marker its bullet or number as written ("•", "3.").
	ListItem
	// CodeLine is one line of a code block.
	CodeLine
	// IllustrationPlaceholder stands where a picture is; Runs hold its
	// caption. A writer leaves it out unless asked for placeholders.
	IllustrationPlaceholder
)

// Block is one paragraph-level unit of the manuscript.
type Block struct {
	Kind   BlockKind
	Level  int
	Marker string
	Runs   []Run
}

// Run is a stretch of text with one set of marks, or a line break.
type Run struct {
	Text                            string
	Bold, Italic, Underline, Strike bool
	Code                            bool
	// Href is a link's target, "" for plain text.
	Href string
	// Break is a hard line break inside the paragraph; Text is empty.
	Break bool
}

// Text is the block's plain text, a line break as "\n".
func (b Block) Text() string {
	var sb strings.Builder
	for _, r := range b.Runs {
		if r.Break {
			sb.WriteString("\n")
			continue
		}
		sb.WriteString(r.Text)
	}
	return sb.String()
}

// How each node and mark the editor defines becomes blocks and runs. The
// export package's schema test holds these lists against the editor's real
// schema, so a node the editor gains cannot quietly vanish from a DOCX.
const (
	// HandledBlock: becomes blocks or runs of its own.
	HandledBlock = "block"
	// HandledText: its text joins the surrounding run, without its markup.
	HandledText = "text"
	// HandledStructure: carried by the structure (the doc, list nesting).
	HandledStructure = "structure"
	// HandledDropped: left out. A note marker is an editing aid; the note
	// behind it is internal and must never reach a reader.
	HandledDropped = "dropped"
)

// EditorNodes says what FromDoc does with each editor node.
var EditorNodes = map[string]string{
	"doc":            HandledStructure,
	"text":           HandledText,
	"paragraph":      HandledBlock,
	"heading":        HandledBlock,
	"blockquote":     HandledBlock,
	"bulletList":     HandledStructure,
	"orderedList":    HandledStructure,
	"listItem":       HandledBlock,
	"codeBlock":      HandledBlock,
	"horizontalRule": HandledBlock,
	"hardBreak":      HandledText,
	// Its label, as plain text: the entity link is the editor's.
	"mention":      HandledText,
	"noteMarker":   HandledDropped,
	"illustration": HandledBlock,
}

// EditorMarks says what FromDoc does with each editor mark.
var EditorMarks = map[string]string{
	"bold":      HandledText,
	"italic":    HandledText,
	"underline": HandledText,
	"strike":    HandledText,
	"code":      HandledText,
	"link":      HandledText,
}

// FromDoc turns one scene's Tiptap JSON into blocks. Empty paragraphs are
// dropped; a reader's document has no blank paragraphs standing in for space.
func FromDoc(raw string) ([]Block, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("manuscriptir: parse doc: %w", err)
	}
	var out []Block
	appendBlocks(&out, children(doc), blockContext{})
	return out, nil
}

type blockContext struct {
	quote     bool
	listDepth int
}

func children(n map[string]any) []map[string]any {
	raw, _ := n["content"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, c := range raw {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func appendBlocks(out *[]Block, nodes []map[string]any, ctx blockContext) {
	for _, n := range nodes {
		kind, _ := n["type"].(string)
		switch kind {
		case "paragraph":
			runs := mergeRuns(inlineRuns(children(n), marks{}))
			if !hasText(runs) {
				continue
			}
			b := Block{Kind: Paragraph, Runs: runs}
			if ctx.quote {
				b.Kind = Quote
			}
			*out = append(*out, b)
		case "heading":
			runs := mergeRuns(inlineRuns(children(n), marks{}))
			if !hasText(runs) {
				continue
			}
			*out = append(*out, Block{Kind: Heading, Level: clamp(intAttr(n, "level", 1), 1, 6), Runs: runs})
		case "blockquote":
			inner := ctx
			inner.quote = true
			appendBlocks(out, children(n), inner)
		case "bulletList", "orderedList":
			appendList(out, n, kind == "orderedList", ctx)
		case "codeBlock":
			text := plainText(children(n))
			for _, line := range strings.Split(text, "\n") {
				*out = append(*out, Block{Kind: CodeLine, Runs: []Run{{Text: line, Code: true}}})
			}
		case "horizontalRule":
			*out = append(*out, Block{Kind: SceneBreak})
		case illustref.NodeType:
			_, caption, ok := illustref.Attrs(n)
			if !ok {
				continue
			}
			*out = append(*out, Block{Kind: IllustrationPlaceholder, Runs: []Run{{Text: caption}}})
		default:
			// A node this package does not know: keep what it holds rather
			// than drop the writer's words (the schema test stops this
			// branch from being where a known node ends up).
			kids := children(n)
			if holdsInlines(kids) {
				if runs := mergeRuns(inlineRuns(kids, marks{})); hasText(runs) {
					*out = append(*out, Block{Kind: Paragraph, Runs: runs})
				}
				continue
			}
			appendBlocks(out, kids, ctx)
		}
	}
}

func appendList(out *[]Block, list map[string]any, ordered bool, ctx blockContext) {
	depth := ctx.listDepth + 1
	start := intAttr(list, "start", 1)
	for i, item := range children(list) {
		marker := "•"
		if ordered {
			marker = fmt.Sprintf("%d.", start+i)
		}
		first := true
		for _, child := range children(item) {
			kind, _ := child["type"].(string)
			if kind == "bulletList" || kind == "orderedList" {
				inner := ctx
				inner.listDepth = depth
				appendList(out, child, kind == "orderedList", inner)
				continue
			}
			var blocks []Block
			appendBlocks(&blocks, []map[string]any{child}, blockContext{})
			for _, b := range blocks {
				b.Kind, b.Level = ListItem, depth
				if first {
					b.Marker = marker
					first = false
				}
				*out = append(*out, b)
			}
		}
	}
}

type marks struct {
	bold, italic, underline, strike, code bool
	href                                  string
}

func inlineRuns(nodes []map[string]any, inherited marks) []Run {
	var out []Run
	for _, n := range nodes {
		kind, _ := n["type"].(string)
		m := readMarks(n, inherited)
		switch kind {
		case "text":
			text, _ := n["text"].(string)
			if text == "" {
				continue
			}
			out = append(out, m.run(text))
		case "hardBreak":
			out = append(out, Run{Break: true})
		case "mention":
			if label := stringAttr(n, "label"); label != "" {
				out = append(out, m.run(label))
			}
		case "noteMarker":
			// An editing aid; the note itself never leaves the app.
		default:
			out = append(out, inlineRuns(children(n), m)...)
		}
	}
	return out
}

// mergeRuns joins neighbouring runs with the same marks: a mention's label
// and the particle after it ("수아" + "는") are one stretch of text to a reader
// and to every format written from it.
func mergeRuns(runs []Run) []Run {
	out := make([]Run, 0, len(runs))
	for _, r := range runs {
		if n := len(out); n > 0 && !r.Break && !out[n-1].Break && sameMarks(out[n-1], r) {
			out[n-1].Text += r.Text
			continue
		}
		out = append(out, r)
	}
	return out
}

func sameMarks(a, b Run) bool {
	return a.Bold == b.Bold && a.Italic == b.Italic && a.Underline == b.Underline &&
		a.Strike == b.Strike && a.Code == b.Code && a.Href == b.Href
}

func (m marks) run(text string) Run {
	return Run{Text: text, Bold: m.bold, Italic: m.italic, Underline: m.underline, Strike: m.strike, Code: m.code, Href: m.href}
}

func readMarks(n map[string]any, m marks) marks {
	raw, _ := n["marks"].([]any)
	for _, r := range raw {
		mm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		switch mm["type"] {
		case "bold":
			m.bold = true
		case "italic":
			m.italic = true
		case "underline":
			m.underline = true
		case "strike":
			m.strike = true
		case "code":
			m.code = true
		case "link":
			m.href = stringAttr(mm, "href")
		}
	}
	return m
}

func holdsInlines(nodes []map[string]any) bool {
	for _, n := range nodes {
		switch n["type"] {
		case "text", "hardBreak", "mention", "noteMarker":
			return true
		}
	}
	return false
}

func plainText(nodes []map[string]any) string {
	var sb strings.Builder
	for _, n := range nodes {
		if s, ok := n["text"].(string); ok {
			sb.WriteString(s)
		}
	}
	return sb.String()
}

func hasText(runs []Run) bool {
	for _, r := range runs {
		if strings.TrimSpace(r.Text) != "" {
			return true
		}
	}
	return false
}

func intAttr(n map[string]any, key string, fallback int) int {
	attrs, _ := n["attrs"].(map[string]any)
	if f, ok := attrs[key].(float64); ok {
		return int(f)
	}
	return fallback
}

func stringAttr(n map[string]any, key string) string {
	attrs, _ := n["attrs"].(map[string]any)
	s, _ := attrs[key].(string)
	return s
}

func clamp(v, lo, hi int) int {
	return max(lo, min(hi, v))
}

// Options says which parts of the outline become headings.
type Options struct {
	// Language is the app language the labels are shown in ("ko", "en",
	// "ja"); stored labels such as "12화" are translated the way the app
	// shows them.
	Language string
	// Headings writes each container's label and title as a heading.
	Headings bool
}

// Build assembles the export for root (a container's subtree or one scene),
// or for the whole work when rootID is "". nodes are the work's nodes, in any
// order; scenes keep the outline's order.
func Build(title string, nodes []node.Node, rootID string, opts Options) (Doc, error) {
	byParent := map[string][]node.Node{}
	byID := map[string]node.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
		parent := ""
		if n.ParentID != nil {
			parent = *n.ParentID
		}
		byParent[parent] = append(byParent[parent], n)
	}
	for k := range byParent {
		sortByOrdinal(byParent[k])
	}
	doc := Doc{Title: title}
	b := builder{doc: &doc, byParent: byParent, opts: opts}
	if rootID == "" {
		if err := b.container(node.Node{}, 0, false); err != nil {
			return Doc{}, err
		}
		return doc, nil
	}
	root, ok := byID[rootID]
	if !ok {
		return Doc{}, fmt.Errorf("manuscriptir: node %s is not in this work", rootID)
	}
	if root.Kind == node.KindLeaf {
		sec := Section{}
		if err := b.scene(&sec, root); err != nil {
			return Doc{}, err
		}
		doc.Sections = append(doc.Sections, sec)
		return doc, nil
	}
	if err := b.container(root, 1, true); err != nil {
		return Doc{}, err
	}
	return doc, nil
}

type builder struct {
	doc      *Doc
	byParent map[string][]node.Node
	opts     Options
}

// container writes c's own section (its heading and the scenes directly under
// it, scene breaks between them), then its child containers one level deeper.
// The work itself is the container with no id and no heading.
func (b builder) container(c node.Node, level int, heading bool) error {
	kids := b.byParent[c.ID]
	sec := Section{Level: level}
	if heading && b.opts.Headings {
		sec.Heading = b.heading(c)
	}
	// Scenes and sub-containers interleave in outline order: each run of
	// scenes goes in a section of its own, so the reading order holds.
	open := false
	flush := func() {
		if open || sec.Heading != "" {
			b.doc.Sections = append(b.doc.Sections, sec)
		}
		sec = Section{Level: level}
		open = false
	}
	for _, k := range kids {
		if k.Kind == node.KindLeaf {
			if open {
				sec.Blocks = append(sec.Blocks, Block{Kind: SceneBreak})
			}
			if err := b.scene(&sec, k); err != nil {
				return err
			}
			sec.HoldsScenes = true
			open = true
			continue
		}
		flush()
		if err := b.container(k, level+1, true); err != nil {
			return err
		}
	}
	flush()
	return nil
}

func (b builder) scene(sec *Section, n node.Node) error {
	if n.ContentDoc == nil {
		return nil
	}
	blocks, err := FromDoc(*n.ContentDoc)
	if err != nil {
		return fmt.Errorf("manuscriptir: scene %s: %w", n.ID, err)
	}
	sec.Blocks = append(sec.Blocks, blocks...)
	return nil
}

func (b builder) heading(c node.Node) string {
	label := node.DisplayLabel(c.Label, b.opts.Language)
	title := strings.TrimSpace(c.Title)
	switch {
	case label == "":
		return title
	case title == "" || title == label:
		return label
	default:
		return label + " " + title
	}
}

func sortByOrdinal(ns []node.Node) {
	for i := 1; i < len(ns); i++ {
		for j := i; j > 0 && ns[j].Ordinal < ns[j-1].Ordinal; j-- {
			ns[j], ns[j-1] = ns[j-1], ns[j]
		}
	}
}

// TextOptions is how Text writes what has no plain-text form of its own.
type TextOptions struct {
	// SceneBreak is the line between scenes; "" leaves a blank line.
	SceneBreak string
	// Illustrations writes "[label: caption]" where a picture is.
	Illustrations     bool
	IllustrationLabel string
}

// Text renders doc as plain text: a blank line between paragraphs, headings
// on lines of their own, marks dropped.
func Text(doc Doc, opts TextOptions) string {
	var paras []string
	for _, sec := range doc.Sections {
		if sec.Heading != "" {
			paras = append(paras, sec.Heading)
		}
		for _, b := range sec.Blocks {
			switch b.Kind {
			case SceneBreak:
				paras = append(paras, opts.SceneBreak)
			case IllustrationPlaceholder:
				if !opts.Illustrations {
					continue
				}
				if caption := strings.TrimSpace(b.Text()); caption != "" {
					paras = append(paras, "["+opts.IllustrationLabel+": "+caption+"]")
				} else {
					paras = append(paras, "["+opts.IllustrationLabel+"]")
				}
			case ListItem:
				text := b.Text()
				if b.Marker != "" {
					text = b.Marker + " " + text
				}
				paras = append(paras, strings.Repeat("  ", max(0, b.Level-1))+text)
			default:
				paras = append(paras, b.Text())
			}
		}
	}
	return strings.Join(paras, "\n\n") + "\n"
}
