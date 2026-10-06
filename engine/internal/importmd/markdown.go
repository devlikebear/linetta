// Package importmd parses a subset of Markdown into Tiptap-shaped block/inline
// structures, then assembles a project + node tree (the inverse of Plan 6 export).
// This is a vendored, dependency-free parser intentionally covering only the
// subset Linetta's export produces.
package importmd

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// TiptapMark mirrors Tiptap's mark JSON shape. Attrs is set for marks that
// carry data (a link's href).
type TiptapMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// TiptapInline mirrors Tiptap's leaf node JSON shape (text or hardBreak).
// For "text" nodes, Text is the content and Marks may be set.
// For "hardBreak" nodes, Text/Marks are zero.
type TiptapInline struct {
	Type  string       `json:"type"`
	Text  string       `json:"text,omitempty"`
	Marks []TiptapMark `json:"marks,omitempty"`
}

// TiptapBlock mirrors Tiptap's block node JSON shape.
// Leaf blocks (paragraph, heading, codeBlock) hold []TiptapInline; container
// blocks (blockquote, lists, listItem) hold []TiptapBlock. Both are stored as
// []any so containers can nest.
type TiptapBlock struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []any          `json:"content,omitempty"`
}

// markRank is the order the editor's schema keeps marks in. Emitting marks in
// this order makes an imported document equal to what the editor would save.
var markRank = map[string]int{"link": 0, "bold": 1, "code": 2, "italic": 3, "strike": 4, "underline": 5}

// ParseInlines parses the text of one paragraph into Tiptap inline nodes.
//
// It reads what export writes: `**bold**`, `_italic_`, `~~strike~~`, a
// backtick code span, `[text](href)`, `<u>underline</u>`, backslash escapes,
// and a line ending in two spaces as a hardBreak. A newline without the two
// spaces joins the lines with one space. Unmatched delimiters pass through as
// literal text.
func ParseInlines(text string) []TiptapInline {
	// A trailing double-space is a hardBreak even on the last line; this is
	// how a paragraph that ends in a break is written.
	trailingBreak := false
	if strings.HasSuffix(text, "  ") {
		trailingBreak = true
		text = strings.TrimRight(text, " ")
	}
	out := mergeText(parseSpan(text, nil))
	if trailingBreak {
		out = append(out, TiptapInline{Type: "hardBreak"})
	}
	return out
}

// span delimiters, longest first so "**" wins over a lone "*".
var spanDelims = []struct {
	open, close, mark string
}{
	{"**", "**", "bold"},
	{"~~", "~~", "strike"},
	{"<u>", "</u>", "underline"},
	{"_", "_", "italic"},
}

func parseSpan(s string, marks []TiptapMark) []TiptapInline {
	var out []TiptapInline
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		out = append(out, textNode(buf.String(), marks))
		buf.Reset()
	}
	i := 0
scan:
	for i < len(s) {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && isPunct(s[i+1]):
			buf.WriteByte(s[i+1])
			i += 2
			continue
		case c == '\n':
			// Two trailing spaces before the newline: a hardBreak.
			pending := buf.String()
			if strings.HasSuffix(pending, "  ") {
				buf.Reset()
				buf.WriteString(strings.TrimRight(pending, " "))
				flush()
				out = append(out, TiptapInline{Type: "hardBreak"})
			} else {
				buf.WriteByte(' ')
			}
			i++
			continue
		case c == '`':
			run := runLength(s, i, '`')
			if end := closeCodeSpan(s, i+run, run); end >= 0 {
				flush()
				out = append(out, TiptapInline{
					Type:  "text",
					Text:  trimCodeSpan(s[i+run : end]),
					Marks: []TiptapMark{{Type: "code"}},
				})
				i = end + run
				continue
			}
			buf.WriteString(s[i : i+run])
			i += run
			continue
		case c == '[':
			if label, href, next, ok := parseLink(s, i); ok {
				flush()
				link := TiptapMark{Type: "link", Attrs: map[string]any{"href": href}}
				out = append(out, parseSpan(label, withMark(marks, link))...)
				i = next
				continue
			}
		}
		for _, d := range spanDelims {
			if !strings.HasPrefix(s[i:], d.open) {
				continue
			}
			end := indexClose(s, i+len(d.open), d.close)
			if end < 0 || end == i+len(d.open) {
				continue
			}
			flush()
			inner := s[i+len(d.open) : end]
			out = append(out, parseSpan(inner, withMark(marks, TiptapMark{Type: d.mark}))...)
			i = end + len(d.close)
			continue scan
		}
		buf.WriteByte(c)
		i++
	}
	flush()
	return out
}

func textNode(text string, marks []TiptapMark) TiptapInline {
	var m []TiptapMark
	if len(marks) > 0 {
		// Copy so callers' mutations don't leak.
		m = make([]TiptapMark, len(marks))
		copy(m, marks)
	}
	return TiptapInline{Type: "text", Text: text, Marks: m}
}

func withMark(marks []TiptapMark, add TiptapMark) []TiptapMark {
	out := make([]TiptapMark, 0, len(marks)+1)
	for _, m := range marks {
		if m.Type != add.Type {
			out = append(out, m)
		}
	}
	out = append(out, add)
	sort.SliceStable(out, func(a, b int) bool { return markRank[out[a].Type] < markRank[out[b].Type] })
	return out
}

// mergeText joins neighbouring text nodes that carry the same marks, the way
// the editor normalises them.
func mergeText(in []TiptapInline) []TiptapInline {
	out := make([]TiptapInline, 0, len(in))
	for _, n := range in {
		if n.Type == "text" && n.Text == "" {
			continue
		}
		if last := len(out) - 1; last >= 0 && n.Type == "text" && out[last].Type == "text" && sameMarks(out[last].Marks, n.Marks) {
			out[last].Text += n.Text
			continue
		}
		out = append(out, n)
	}
	return out
}

func sameMarks(a, b []TiptapMark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type {
			return false
		}
		ha, _ := a[i].Attrs["href"].(string)
		hb, _ := b[i].Attrs["href"].(string)
		if ha != hb {
			return false
		}
	}
	return true
}

func isPunct(c byte) bool {
	return c < 0x80 && strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0
}

func runLength(s string, i int, ch byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == ch {
		n++
	}
	return n
}

// closeCodeSpan finds the closing run of exactly `run` backticks at or after
// start. Returns its index, or -1.
func closeCodeSpan(s string, start, run int) int {
	for i := start; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		n := runLength(s, i, '`')
		if n == run {
			return i
		}
		i += n
	}
	return -1
}

// trimCodeSpan drops the single padding space export adds around a code span
// that starts or ends with a backtick or a space.
func trimCodeSpan(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) >= 2 && s[0] == ' ' && s[len(s)-1] == ' ' && strings.TrimSpace(s) != "" {
		return s[1 : len(s)-1]
	}
	return s
}

// indexClose finds the next unescaped occurrence of delim in s at or after
// start, skipping code spans. Returns -1 if not found.
func indexClose(s string, start int, delim string) int {
	for i := start; i < len(s); {
		switch {
		case s[i] == '\\' && i+1 < len(s) && isPunct(s[i+1]):
			i += 2
		case s[i] == '`':
			run := runLength(s, i, '`')
			if end := closeCodeSpan(s, i+run, run); end >= 0 {
				i = end + run
			} else {
				i += run
			}
		case strings.HasPrefix(s[i:], delim):
			return i
		default:
			i++
		}
	}
	return -1
}

// parseLink reads `[label](href)` or `[label](<href>)` starting at s[i] == '['.
func parseLink(s string, i int) (label, href string, next int, ok bool) {
	depth := 0
	closeAt := -1
	for j := i; j < len(s) && closeAt < 0; {
		switch {
		case s[j] == '\\' && j+1 < len(s) && isPunct(s[j+1]):
			j += 2
			continue
		case s[j] == '`':
			run := runLength(s, j, '`')
			if end := closeCodeSpan(s, j+run, run); end >= 0 {
				j = end + run
			} else {
				j += run
			}
			continue
		case s[j] == '[':
			depth++
		case s[j] == ']':
			depth--
			if depth == 0 {
				closeAt = j
			}
		}
		j++
	}
	if closeAt < 0 || closeAt+1 >= len(s) || s[closeAt+1] != '(' {
		return "", "", 0, false
	}
	rest := s[closeAt+2:]
	if strings.HasPrefix(rest, "<") {
		end := strings.Index(rest, ">)")
		if end < 0 {
			return "", "", 0, false
		}
		href = strings.NewReplacer("%3C", "<", "%3E", ">").Replace(rest[1:end])
		return s[i+1 : closeAt], href, closeAt + 2 + end + 2, true
	}
	end := strings.IndexAny(rest, ") \n")
	if end <= 0 || rest[end] != ')' {
		return "", "", 0, false
	}
	return s[i+1 : closeAt], rest[:end], closeAt + 2 + end + 1, true
}

var (
	fenceRE     = regexp.MustCompile("^( *)(`{3,})\\s*([^`\\s]*)\\s*$")
	ruleRE      = regexp.MustCompile(`^ {0,3}(-{3,}|\*{3,}|_{3,})\s*$`)
	listItemRE  = regexp.MustCompile(`^( *)([-+*]|\d{1,9}[.)])( +)(.*)$`)
	blockHeadRE = regexp.MustCompile(`^ {0,3}(#{1,6})\s+(.+?)\s*$`)
)

// ParseBlocks splits text into Tiptap block nodes: paragraphs, blockquotes,
// bullet and ordered lists, fenced code blocks, rules and headings.
//
// Blank lines separate blocks. A paragraph runs until the next blank line,
// and its lines are joined with one space unless a line ends with an explicit
// hardBreak (two trailing spaces).
func ParseBlocks(text string) []TiptapBlock {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return nil
	}
	return parseBlockLines(strings.Split(text, "\n"), false)
}

// parseBlockLines parses lines into blocks. inItem is true for the content of
// a list item, where a nested list or fence follows its paragraph without a
// blank line between them.
func parseBlockLines(lines []string, inItem bool) []TiptapBlock {
	var blocks []TiptapBlock
	for i := 0; i < len(lines); {
		ln := lines[i]
		if strings.TrimSpace(ln) == "" {
			i++
			continue
		}
		if m := fenceRE.FindStringSubmatch(ln); m != nil {
			block, next := parseFence(lines, i, m)
			blocks = append(blocks, block)
			i = next
			continue
		}
		if ruleRE.MatchString(ln) {
			blocks = append(blocks, TiptapBlock{Type: "horizontalRule"})
			i++
			continue
		}
		if m := blockHeadRE.FindStringSubmatch(ln); m != nil {
			blocks = append(blocks, TiptapBlock{
				Type:    "heading",
				Attrs:   map[string]any{"level": len(m[1])},
				Content: inlineContent(ParseInlines(m[2])),
			})
			i++
			continue
		}
		if isQuoteLine(ln) {
			block, next := parseQuote(lines, i)
			blocks = append(blocks, block)
			i = next
			continue
		}
		if m := listItemRE.FindStringSubmatch(ln); m != nil {
			block, next := parseList(lines, i)
			blocks = append(blocks, block)
			i = next
			continue
		}
		// Paragraph: every line up to the next blank one. Only a blank line
		// ends it, so "- " or "> " on a continuation line is still prose.
		j := i + 1
		for j < len(lines) && strings.TrimSpace(lines[j]) != "" {
			if inItem && (listItemRE.MatchString(lines[j]) || fenceRE.MatchString(lines[j])) {
				break
			}
			j++
		}
		blocks = append(blocks, makeParagraph(lines[i:j]))
		i = j
	}
	return blocks
}

func parseFence(lines []string, i int, m []string) (TiptapBlock, int) {
	indent, fence, lang := len(m[1]), m[2], m[3]
	var code []string
	j := i + 1
	for ; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if len(t) >= len(fence) && strings.Trim(t, "`") == "" {
			j++
			break
		}
		code = append(code, stripIndent(lines[j], indent))
	}
	block := TiptapBlock{Type: "codeBlock", Attrs: map[string]any{"language": nil}}
	if lang != "" {
		block.Attrs["language"] = lang
	}
	if text := strings.Join(code, "\n"); text != "" {
		block.Content = []any{TiptapInline{Type: "text", Text: text}}
	}
	return block, j
}

func stripIndent(line string, n int) string {
	i := 0
	for i < n && i < len(line) && line[i] == ' ' {
		i++
	}
	return line[i:]
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func isQuoteLine(line string) bool {
	t := strings.TrimLeft(line, " ")
	return len(line)-len(t) <= 3 && strings.HasPrefix(t, ">")
}

func parseQuote(lines []string, i int) (TiptapBlock, int) {
	var inner []string
	j := i
	for ; j < len(lines) && isQuoteLine(lines[j]); j++ {
		s := strings.TrimPrefix(strings.TrimLeft(lines[j], " "), ">")
		inner = append(inner, strings.TrimPrefix(s, " "))
	}
	return TiptapBlock{Type: "blockquote", Content: blockContent(parseBlockLines(inner, false))}, j
}

// parseList reads one list starting at lines[i]. Items belong to the list
// while they share its indent and kind (bullet vs ordered); lines indented to
// an item's content column belong to that item, blank lines included.
func parseList(lines []string, i int) (TiptapBlock, int) {
	first := listItemRE.FindStringSubmatch(lines[i])
	base := len(first[1])
	ordered := isOrderedMarker(first[2])
	list := TiptapBlock{Type: "bulletList"}
	if ordered {
		start, _ := strconv.Atoi(strings.TrimRight(first[2], ".)"))
		list = TiptapBlock{Type: "orderedList", Attrs: map[string]any{"start": start}}
	}

	j := i
	for j < len(lines) {
		m := listItemRE.FindStringSubmatch(lines[j])
		if m == nil || len(m[1]) != base || isOrderedMarker(m[2]) != ordered || ruleRE.MatchString(lines[j]) {
			break
		}
		// Content column: marker plus one space. Extra spaces after the
		// marker are content only when they are too many to be padding.
		pad := len(m[3])
		if pad > 4 {
			pad = 1
		}
		column := base + len(m[2]) + pad
		item := []string{m[4]}
		if pad != len(m[3]) {
			item[0] = strings.Repeat(" ", len(m[3])-pad) + m[4]
		}
		j++
		for j < len(lines) {
			if strings.TrimSpace(lines[j]) == "" {
				// A blank line stays in the item only if indented content follows.
				k := j
				for k < len(lines) && strings.TrimSpace(lines[k]) == "" {
					k++
				}
				if k < len(lines) && leadingSpaces(lines[k]) >= column {
					for ; j < k; j++ {
						item = append(item, "")
					}
					continue
				}
				break
			}
			if leadingSpaces(lines[j]) < column {
				break
			}
			item = append(item, lines[j][column:])
			j++
		}
		list.Content = append(list.Content, listItem(parseBlockLines(item, true)))
		// Skip blank lines between items of a loose list.
		k := j
		for k < len(lines) && strings.TrimSpace(lines[k]) == "" {
			k++
		}
		if k < len(lines) && k > j {
			if next := listItemRE.FindStringSubmatch(lines[k]); next != nil && len(next[1]) == base && isOrderedMarker(next[2]) == ordered {
				j = k
			}
		}
	}
	return list, j
}

func isOrderedMarker(marker string) bool {
	return marker[0] >= '0' && marker[0] <= '9'
}

// listItem wraps blocks as a list item. The editor's schema requires an item
// to open with a paragraph, so one is supplied when the content does not.
func listItem(blocks []TiptapBlock) TiptapBlock {
	if len(blocks) == 0 || blocks[0].Type != "paragraph" {
		blocks = append([]TiptapBlock{{Type: "paragraph"}}, blocks...)
	}
	return TiptapBlock{Type: "listItem", Content: blockContent(blocks)}
}

func blockContent(blocks []TiptapBlock) []any {
	out := make([]any, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b)
	}
	return out
}

func inlineContent(inlines []TiptapInline) []any {
	out := make([]any, 0, len(inlines))
	for _, in := range inlines {
		out = append(out, in)
	}
	return out
}

// makeParagraph turns consecutive non-empty lines into one paragraph.
func makeParagraph(lines []string) TiptapBlock {
	return TiptapBlock{Type: "paragraph", Content: inlineContent(ParseInlines(strings.Join(lines, "\n")))}
}

// fenceTracker reports whether a line sits inside a fenced code block, so the
// outline pass does not read a "# comment" in code as a heading.
type fenceTracker struct{ fence string }

// inside feeds one line and reports whether it belongs to a code block
// (the fence lines themselves included).
func (f *fenceTracker) inside(line string) bool {
	if f.fence != "" {
		t := strings.TrimSpace(line)
		if len(t) >= len(f.fence) && strings.Trim(t, "`") == "" {
			f.fence = ""
		}
		return true
	}
	if m := fenceRE.FindStringSubmatch(line); m != nil {
		f.fence = m[2]
		return true
	}
	return false
}
