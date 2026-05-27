// Package importmd parses a subset of Markdown into Tiptap-shaped block/inline
// structures, then assembles a project + node tree (the inverse of Plan 6 export).
// This is a vendored, dependency-free parser intentionally covering only the
// subset Linetta's export produces.
package importmd

import "strings"

// TiptapMark mirrors Tiptap's mark JSON shape.
type TiptapMark struct {
	Type string `json:"type"`
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
// For "paragraph": Content is []TiptapInline (but stored as []any so blockquotes
// can recurse with nested paragraphs).
// For "blockquote": Content is []TiptapBlock.
type TiptapBlock struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []any          `json:"content,omitempty"`
}

// ParseInlines walks one line of text and emits Tiptap inline nodes,
// recognizing `**bold**`, `_italic_`, and trailing-double-space hardBreak.
// Unmatched delimiters pass through as literal text.
func ParseInlines(line string) []TiptapInline {
	// Detect trailing double-space hardBreak.
	hardBreak := false
	if strings.HasSuffix(line, "  ") {
		hardBreak = true
		line = strings.TrimRight(line, " ")
	}
	out := parseMarks(line, nil)
	if hardBreak {
		out = append(out, TiptapInline{Type: "hardBreak"})
	}
	return out
}

// parseMarks iteratively walks the string, looking for the next bold (**) or
// italic (_) delimiter. When found and a matching close exists, it recurses on
// the inner content with the new mark stacked on.
func parseMarks(s string, marks []TiptapMark) []TiptapInline {
	if s == "" {
		return nil
	}
	var out []TiptapInline
	i := 0
	for i < len(s) {
		// Try bold first (** is longer).
		if i+1 < len(s) && s[i] == '*' && s[i+1] == '*' {
			closeAt := indexClose(s, i+2, "**")
			if closeAt >= 0 {
				out = emitText(out, s[:i], marks)
				inner := s[i+2 : closeAt]
				out = append(out, emitInner(inner, marks, "bold")...)
				s = s[closeAt+2:]
				i = 0
				continue
			}
		}
		if s[i] == '_' {
			closeAt := indexClose(s, i+1, "_")
			if closeAt >= 0 {
				out = emitText(out, s[:i], marks)
				inner := s[i+1 : closeAt]
				out = append(out, emitInner(inner, marks, "italic")...)
				s = s[closeAt+1:]
				i = 0
				continue
			}
		}
		i++
	}
	out = emitText(out, s, marks)
	return out
}

func emitInner(inner string, marks []TiptapMark, addMark string) []TiptapInline {
	stacked := make([]TiptapMark, 0, len(marks)+1)
	stacked = append(stacked, marks...)
	stacked = append(stacked, TiptapMark{Type: addMark})
	return parseMarks(inner, stacked)
}

func emitText(out []TiptapInline, s string, marks []TiptapMark) []TiptapInline {
	if s == "" {
		return out
	}
	// Copy marks slice so callers' mutations don't leak.
	var m []TiptapMark
	if len(marks) > 0 {
		m = make([]TiptapMark, len(marks))
		copy(m, marks)
	}
	return append(out, TiptapInline{Type: "text", Text: s, Marks: m})
}

// indexClose finds the next occurrence of delim in s starting at start.
// Returns -1 if not found.
func indexClose(s string, start int, delim string) int {
	idx := strings.Index(s[start:], delim)
	if idx < 0 {
		return -1
	}
	return start + idx
}

// ParseBlocks splits text into Tiptap block nodes (paragraph or blockquote),
// using blank lines as paragraph separators. Multi-line paragraphs get a single
// space joining lines unless the line ends with an explicit hardBreak (two
// trailing spaces).
func ParseBlocks(text string) []TiptapBlock {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")

	var blocks []TiptapBlock
	var bufLines []string
	flush := func() {
		if len(bufLines) == 0 {
			return
		}
		blocks = append(blocks, makeBlock(bufLines))
		bufLines = nil
	}
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			flush()
			continue
		}
		bufLines = append(bufLines, ln)
	}
	flush()
	return blocks
}

// makeBlock turns a group of consecutive non-empty lines into a single block.
// If every line begins with "> " (or is just ">"), it's a blockquote (inner is
// re-parsed as a single paragraph).
func makeBlock(lines []string) TiptapBlock {
	if isBlockquote(lines) {
		stripped := make([]string, 0, len(lines))
		for _, ln := range lines {
			if ln == ">" {
				stripped = append(stripped, "")
				continue
			}
			// Strip "> " prefix.
			s := strings.TrimPrefix(ln, "> ")
			stripped = append(stripped, s)
		}
		inner := makeParagraph(stripped)
		return TiptapBlock{
			Type:    "blockquote",
			Content: []any{inner},
		}
	}
	return makeParagraph(lines)
}

func isBlockquote(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, ln := range lines {
		if ln == ">" {
			continue
		}
		if !strings.HasPrefix(ln, "> ") {
			return false
		}
	}
	return true
}

// makeParagraph joins lines (with single-space separators unless previous line
// ended with hardBreak) and produces inline nodes.
func makeParagraph(lines []string) TiptapBlock {
	var inlines []TiptapInline
	for i, ln := range lines {
		part := ParseInlines(ln)
		if i > 0 {
			// If the previous line did not end with an explicit hardBreak, prepend
			// a space joiner before this line's content.
			prevWasHardBreak := false
			if len(inlines) > 0 && inlines[len(inlines)-1].Type == "hardBreak" {
				prevWasHardBreak = true
			}
			if !prevWasHardBreak {
				inlines = append(inlines, TiptapInline{Type: "text", Text: " "})
			}
		}
		inlines = append(inlines, part...)
	}
	content := make([]any, 0, len(inlines))
	for _, in := range inlines {
		content = append(content, in)
	}
	return TiptapBlock{Type: "paragraph", Content: content}
}
