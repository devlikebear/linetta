package charcount

import (
	"encoding/json"
	"unicode/utf8"
)

// A 200-character manuscript sheet (원고지) is 20 cells across and 10 lines
// down. Korean literary contests set length in sheets rather than characters
// (episode-length Phase 4).
const (
	ManuscriptColumns = 20
	ManuscriptRows    = 10
)

// ManuscriptLines estimates how many manuscript lines a Tiptap doc (raw JSON)
// fills. It is a rough count, not the full manuscript rules: each paragraph
// starts one cell in, a line break starts a new line without the indent, and
// every character takes one cell, spaces included whatever the work's count
// basis. Punctuation and dialogue rules are left out. An empty paragraph is
// not counted, nor is anything but text, the same as Doc.
func ManuscriptLines(rawDoc []byte) int {
	if len(rawDoc) == 0 {
		return 0
	}
	var v any
	if err := json.Unmarshal(rawDoc, &v); err != nil {
		return 0
	}
	return manuscriptLines(v)
}

// ManuscriptPages is the sheets lines fill, a part sheet counted whole.
func ManuscriptPages(lines int) int {
	if lines <= 0 {
		return 0
	}
	return (lines + ManuscriptRows - 1) / ManuscriptRows
}

func manuscriptLines(v any) int {
	switch t := v.(type) {
	case map[string]any:
		switch t["type"] {
		case "paragraph", "heading":
			return paragraphLines(t["content"])
		}
		if content, ok := t["content"].([]any); ok {
			return manuscriptLines(content)
		}
	case []any:
		n := 0
		for _, c := range t {
			n += manuscriptLines(c)
		}
		return n
	}
	return 0
}

// paragraphLines counts the lines one paragraph's inline content fills.
func paragraphLines(content any) int {
	items, _ := content.([]any)
	lines := 0
	cells, written := 1, 0 // the first line starts one cell in
	endLine := func() {
		if written > 0 {
			lines += (cells + ManuscriptColumns - 1) / ManuscriptColumns
		}
		cells, written = 0, 0
	}
	for _, item := range items {
		m, _ := item.(map[string]any)
		switch m["type"] {
		case "text":
			s, _ := m["text"].(string)
			n := utf8.RuneCountInString(s)
			cells += n
			written += n
		case "hardBreak":
			endLine()
		}
	}
	endLine()
	return lines
}
