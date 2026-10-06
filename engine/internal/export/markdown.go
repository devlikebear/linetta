// Package export converts Tiptap JSON documents to markdown.
// Mentions are rendered as plain `@label`. The whole-project export
// (project.go) walks the node tree and produces a single document with
// H1/H2/H3 headings derived from depth.
package export

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DocToMarkdown parses a Tiptap JSON document and returns its markdown rendering.
// Returns an error only when the input is not valid JSON.
//
// Every node and mark the editor can produce has to be named in
// editorNodes/editorMarks below (#160): a node this function does not know
// used to fall through to "children only", which turned lists, code blocks
// and links into unmarked prose without telling anyone.
func DocToMarkdown(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return "", fmt.Errorf("export: parse doc: %w", err)
	}
	blocks := renderBlocks(blockChildren(node))
	if len(blocks) == 0 {
		return "", nil
	}
	return strings.Join(blocks, "\n\n") + "\n\n", nil
}

// How DocToMarkdown treats each node and mark the editor schema defines.
// schema_test.go holds this against the editor's real schema, so adding an
// extension to the editor without deciding its export fails a test.
const (
	// handlingMarkdown: written as markdown that importmd reads back.
	handlingMarkdown = "markdown"
	// handlingStructure: carried by the document structure or the frontmatter
	// rather than by inline syntax.
	handlingStructure = "structure"
)

var editorNodes = map[string]string{
	"doc":            handlingStructure,
	"text":           handlingMarkdown,
	"paragraph":      handlingMarkdown,
	"heading":        handlingMarkdown,
	"blockquote":     handlingMarkdown,
	"bulletList":     handlingMarkdown,
	"orderedList":    handlingMarkdown,
	"listItem":       handlingMarkdown,
	"codeBlock":      handlingMarkdown,
	"horizontalRule": handlingMarkdown,
	"hardBreak":      handlingMarkdown,
	// Written as `@label`; the entity itself travels in the frontmatter.
	"mention": handlingStructure,
	// A margin-note anchor. The note travels in the frontmatter (mdmeta.Note).
	"noteMarker": handlingStructure,
}

var editorMarks = map[string]string{
	"bold":      handlingMarkdown,
	"italic":    handlingMarkdown,
	"strike":    handlingMarkdown,
	"code":      handlingMarkdown,
	"link":      handlingMarkdown,
	"underline": handlingMarkdown,
}

var multiBlankRE = regexp.MustCompile(`\n{3,}`)

// collapseBlankLines squeezes runs of blank lines to one, except inside fenced
// code blocks, where the writer's blank lines are content.
func collapseBlankLines(s string) string {
	if !strings.Contains(s, "```") {
		return multiBlankRE.ReplaceAllString(s, "\n\n")
	}
	head, body := splitFrontMatter(s)
	// The final "\n" terminates the last line; it is not a blank line.
	tail := ""
	if strings.HasSuffix(body, "\n") {
		body, tail = body[:len(body)-1], "\n"
	}
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	fence := ""
	blanks := 0
	for _, ln := range lines {
		if fence != "" {
			out = append(out, ln)
			if closesFence(ln, fence) {
				fence = ""
			}
			continue
		}
		if ln == "" {
			blanks++
			if blanks > 1 {
				continue
			}
			out = append(out, ln)
			continue
		}
		blanks = 0
		if f := opensFence(ln); f != "" {
			fence = f
		}
		out = append(out, ln)
	}
	return multiBlankRE.ReplaceAllString(head, "\n\n") + strings.Join(out, "\n") + tail
}

// splitFrontMatter separates a leading `---` YAML block from the body, so a
// "```" inside a frontmatter string is never mistaken for a fence.
func splitFrontMatter(s string) (head, body string) {
	if !strings.HasPrefix(s, "---\n") {
		return "", s
	}
	end := strings.Index(s[4:], "\n---\n")
	if end < 0 {
		return "", s
	}
	cut := 4 + end + len("\n---\n")
	return s[:cut], s[cut:]
}

var fenceOpenRE = regexp.MustCompile("^ *(`{3,})[^`]*$")

func opensFence(line string) string {
	m := fenceOpenRE.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return m[1]
}

func closesFence(line, fence string) bool {
	t := strings.TrimSpace(line)
	return len(t) >= len(fence) && strings.Trim(t, "`") == ""
}

func blockChildren(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case map[string]any:
		if content, ok := t["content"].([]any); ok {
			return content
		}
	}
	return nil
}

// renderBlocks renders each block node to its markdown text (no trailing
// newline). Blocks that render to nothing — an empty paragraph — are skipped.
func renderBlocks(nodes []any) []string {
	var out []string
	for _, n := range nodes {
		m, ok := n.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, renderBlock(m)...)
	}
	return out
}

func renderBlock(n map[string]any) []string {
	kind, _ := n["type"].(string)
	content, _ := n["content"].([]any)
	switch kind {
	case "paragraph":
		text := escapeLineStarts(renderInlines(content))
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []string{text}
	case "heading":
		level := intAttr(n, "level", 1)
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		return []string{strings.Repeat("#", level) + " " + renderInlines(content)}
	case "blockquote":
		inner := strings.Join(renderBlocks(content), "\n\n")
		if inner == "" {
			return []string{">"}
		}
		return []string{prefixLines(inner, "> ")}
	case "horizontalRule":
		return []string{"---"}
	case "codeBlock":
		return []string{renderCodeBlock(n, content)}
	case "bulletList":
		return []string{renderList(content, func(int) string { return "- " })}
	case "orderedList":
		start := intAttr(n, "start", 1)
		return []string{renderList(content, func(i int) string { return strconv.Itoa(start+i) + ". " })}
	default:
		// Unknown node. Keep whatever it holds rather than dropping it: block
		// children as blocks, inline children as a paragraph.
		if len(content) == 0 {
			return nil
		}
		if holdsInlines(content) {
			text := escapeLineStarts(renderInlines(content))
			if strings.TrimSpace(text) == "" {
				return nil
			}
			return []string{text}
		}
		return renderBlocks(content)
	}
}

func holdsInlines(content []any) bool {
	for _, c := range content {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "text", "hardBreak", "mention", "noteMarker":
			return true
		}
	}
	return false
}

func intAttr(n map[string]any, key string, fallback int) int {
	attrs, ok := n["attrs"].(map[string]any)
	if !ok {
		return fallback
	}
	if f, ok := attrs[key].(float64); ok {
		return int(f)
	}
	return fallback
}

func stringAttr(n map[string]any, key string) string {
	attrs, ok := n["attrs"].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := attrs[key].(string)
	return s
}

func renderCodeBlock(n map[string]any, content []any) string {
	var sb strings.Builder
	for _, c := range content {
		if m, ok := c.(map[string]any); ok {
			if s, ok := m["text"].(string); ok {
				sb.WriteString(s)
			}
		}
	}
	code := sb.String()
	fence := strings.Repeat("`", max(3, longestRun(code, '`')+1))
	return fence + stringAttr(n, "language") + "\n" + code + "\n" + fence
}

func longestRun(s string, ch byte) int {
	best, run := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == ch {
			run++
			if run > best {
				best = run
			}
			continue
		}
		run = 0
	}
	return best
}

// renderList renders list items. A list whose items each hold one paragraph
// (plus, optionally, nested lists) is written tight; anything else is loose,
// so a second paragraph inside an item stays inside it.
func renderList(items []any, marker func(i int) string) string {
	type item struct {
		blocks []string
		tight  bool
	}
	rendered := make([]item, 0, len(items))
	tight := true
	for _, raw := range items {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		children, _ := m["content"].([]any)
		it := item{blocks: renderBlocks(children), tight: tightItem(children)}
		if !it.tight {
			tight = false
		}
		rendered = append(rendered, it)
	}
	itemSep, blockSep := "\n\n", "\n\n"
	if tight {
		itemSep, blockSep = "\n", "\n"
	}
	parts := make([]string, 0, len(rendered))
	for i, it := range rendered {
		mk := marker(i)
		body := strings.Join(it.blocks, blockSep)
		parts = append(parts, mk+indentContinuation(body, strings.Repeat(" ", len(mk))))
	}
	return strings.Join(parts, itemSep)
}

func tightItem(children []any) bool {
	for i, c := range children {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := m["type"].(string)
		switch {
		case i == 0 && kind == "paragraph":
		case i > 0 && (kind == "bulletList" || kind == "orderedList"):
		default:
			return false
		}
	}
	return true
}

// indentContinuation indents every line after the first; blank lines stay empty.
func indentContinuation(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = indent + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

// markOrder is the nesting order marks are opened in, outermost first. `code`
// is not here: a code span cannot hold other syntax, so it is written alone.
var markOrder = []string{"link", "bold", "italic", "strike", "underline"}

type inlineRun struct {
	text  string // already escaped
	marks []openMark
	brk   bool
}

type openMark struct {
	kind string
	href string
}

func (m openMark) open() string {
	switch m.kind {
	case "bold":
		return "**"
	case "italic":
		return "_"
	case "strike":
		return "~~"
	case "underline":
		return "<u>"
	case "link":
		return "["
	}
	return ""
}

func (m openMark) close() string {
	switch m.kind {
	case "underline":
		return "</u>"
	case "link":
		return "](" + linkDestination(m.href) + ")"
	}
	return m.open()
}

func linkDestination(href string) string {
	if strings.ContainsAny(href, " ()<>") {
		return "<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(href) + ">"
	}
	return href
}

// renderInlines writes a run of inline nodes. Marks are opened and closed as
// the set changes between neighbours, so "**a _b_ c**" comes out as one bold
// span rather than three.
func renderInlines(content []any) string {
	runs := collectRuns(content)
	// A hard break carries the marks its neighbours share, so a span that
	// crosses the break is not closed and reopened around it.
	for i := range runs {
		if !runs[i].brk {
			continue
		}
		if i > 0 && i+1 < len(runs) {
			runs[i].marks = sharedPrefix(runs[i-1].marks, runs[i+1].marks)
		}
	}
	var sb strings.Builder
	var stack []openMark
	for _, r := range runs {
		keep := len(sharedPrefix(stack, r.marks))
		for i := len(stack) - 1; i >= keep; i-- {
			sb.WriteString(stack[i].close())
		}
		stack = stack[:keep]
		for _, m := range r.marks[keep:] {
			sb.WriteString(m.open())
			stack = append(stack, m)
		}
		if r.brk {
			sb.WriteString("  \n")
			continue
		}
		sb.WriteString(r.text)
	}
	for i := len(stack) - 1; i >= 0; i-- {
		sb.WriteString(stack[i].close())
	}
	return sb.String()
}

func sharedPrefix(a, b []openMark) []openMark {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return a[:n]
}

func collectRuns(content []any) []inlineRun {
	var runs []inlineRun
	for _, c := range content {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := m["type"].(string)
		switch kind {
		case "text":
			text, _ := m["text"].(string)
			if text == "" {
				continue
			}
			marks, code := readMarks(m)
			if code {
				runs = append(runs, inlineRun{text: codeSpan(text), marks: marks})
				continue
			}
			runs = append(runs, inlineRun{text: escapeText(text), marks: marks})
		case "hardBreak":
			runs = append(runs, inlineRun{brk: true})
		case "mention":
			if label := stringAttr(m, "label"); label != "" {
				marks, _ := readMarks(m)
				runs = append(runs, inlineRun{text: "@" + escapeText(label), marks: marks})
			}
		default:
			// Unknown inline node — keep its children.
			if inner, ok := m["content"].([]any); ok {
				runs = append(runs, collectRuns(inner)...)
			}
		}
	}
	return runs
}

// readMarks returns the node's marks in nesting order and whether it is code.
func readMarks(n map[string]any) (marks []openMark, code bool) {
	raw, ok := n["marks"].([]any)
	if !ok {
		return nil, false
	}
	have := map[string]openMark{}
	for _, r := range raw {
		mm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := mm["type"].(string)
		if kind == "code" {
			code = true
			continue
		}
		om := openMark{kind: kind}
		if kind == "link" {
			om.href = stringAttr(mm, "href")
		}
		have[kind] = om
	}
	for _, kind := range markOrder {
		if om, ok := have[kind]; ok {
			marks = append(marks, om)
		}
	}
	return marks, code
}

func codeSpan(text string) string {
	fence := strings.Repeat("`", longestRun(text, '`')+1)
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") ||
		(strings.HasPrefix(text, " ") && strings.HasSuffix(text, " ") && strings.TrimSpace(text) != "") {
		return fence + " " + text + " " + fence
	}
	return fence + text + fence
}

var (
	tildeRunRE = regexp.MustCompile(`~{2,}`)
	uTagRE     = regexp.MustCompile(`(?i)<(/?u>)`)
)

// escapeText backslash-escapes what importmd would otherwise read as syntax.
// It is deliberately narrow: a lone `~` or `[` is ordinary in prose (and `[`
// opens every system message in a web novel), so only the sequences that
// actually form a span are escaped.
func escapeText(s string) string {
	if !strings.ContainsAny(s, "\\*_`~]<") {
		return s
	}
	s = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `_`, `\_`, "`", "\\`", `](`, `\](`).Replace(s)
	s = tildeRunRE.ReplaceAllStringFunc(s, func(run string) string {
		return strings.Repeat(`\~`, len(run))
	})
	return uTagRE.ReplaceAllString(s, `\<$1`)
}

var (
	lineStartRE   = regexp.MustCompile(`^( *)(#{1,6}(?:\s|$)|>|[-+](?:\s|$)|-{3,}\s*$)`)
	orderedLineRE = regexp.MustCompile(`^( *\d{1,9})([.)](?:\s|$))`)
)

// escapeLineStarts keeps a paragraph line that merely begins like a block —
// "- 그는 웃었다", "1. 첫째" — from being read back as one.
func escapeLineStarts(text string) string {
	if text == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		if m := lineStartRE.FindStringSubmatchIndex(ln); m != nil {
			lines[i] = ln[:m[4]] + `\` + ln[m[4]:]
			continue
		}
		if m := orderedLineRE.FindStringSubmatchIndex(ln); m != nil {
			lines[i] = ln[:m[4]] + `\` + ln[m[4]:]
		}
	}
	return strings.Join(lines, "\n")
}

// prefixLines applies `prefix` to every line of `s`; an empty line gets the
// prefix without its trailing space, so a quoted blank line is ">".
func prefixLines(s, prefix string) string {
	if s == "" {
		return prefix
	}
	bare := strings.TrimRight(prefix, " ")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = bare
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
