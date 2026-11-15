package manuscript

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/style"
)

// A chunk grows paragraph by paragraph until it holds chunkMinRunes, and does
// not take a paragraph that would carry it past chunkMaxRunes. A single
// paragraph longer than that is a chunk of its own.
const (
	chunkMinRunes = 300
	chunkMaxRunes = 600
)

// chunkSeparator joins a chunk's paragraphs. style.Paragraphs splits on it, so
// no paragraph contains it and splitting a chunk's text gives them back.
const chunkSeparator = "\n\n"

// Chunk is a run of whole, consecutive paragraphs of one scene.
type Chunk struct {
	// ParaFrom and ParaTo are 1-based and inclusive, numbered by
	// style.Paragraphs: the numbers linetta_read_scene numbered=true prints and
	// linetta_report_continuity cites.
	ParaFrom int
	ParaTo   int
	Text     string
}

// Chunks cuts a scene's content doc into paragraph-aligned chunks that do not
// overlap. Sharing a paragraph between neighbours was measured and dropped:
// with paragraphs of a couple of hundred characters, a chunk is two of them,
// and sharing one put every paragraph in two chunks.
func Chunks(contentDoc string) []Chunk {
	return chunkParagraphs(style.Paragraphs(contentDoc))
}

func chunkParagraphs(paras []string) []Chunk {
	var out []Chunk
	for start := 0; start < len(paras); {
		end, size := start, utf8.RuneCountInString(paras[start])
		for end+1 < len(paras) && size < chunkMinRunes {
			next := utf8.RuneCountInString(paras[end+1])
			if size+next > chunkMaxRunes {
				break
			}
			end++
			size += next
		}
		out = append(out, Chunk{
			ParaFrom: start + 1,
			ParaTo:   end + 1,
			Text:     strings.Join(paras[start:end+1], chunkSeparator),
		})
		start = end + 1
	}
	return out
}

func (c Chunk) paragraphs() []string {
	return strings.Split(c.Text, chunkSeparator)
}

func chunkHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
