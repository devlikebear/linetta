// Package node owns Node domain logic for the Tiptap-content recursive tree.
package node

import "github.com/devlikebear/linetta/engine/internal/charcount"

// CountChars walks a Tiptap doc (raw JSON) and returns the total visible
// character count including spaces — what Korean writing UIs label as "자".
// Returns 0 for any malformed or empty input. A scene's stored word_count
// follows its work's basis instead; see charcount.Doc.
func CountChars(rawDoc []byte) int {
	return charcount.Doc(rawDoc, charcount.WithSpaces)
}
