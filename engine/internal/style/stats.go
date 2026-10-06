package style

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxEndings is how many sentence endings Stats lists.
const maxEndings = 8

// Stats are the measurable facts of a body of prose (#163): what a writer
// would otherwise have to describe about their own style from memory. They
// are counted, not judged — the same scenes always give the same numbers —
// and they are raw material for a style profile, not the profile itself.
type Stats struct {
	Scenes     int `json:"scenes"`
	Paragraphs int `json:"paragraphs"`
	Sentences  int `json:"sentences"`
	// Characters counts letters and punctuation, not whitespace.
	Characters int `json:"characters"`

	// Sentence length in characters.
	SentenceMean   float64 `json:"sentence_mean"`
	SentenceMedian int     `json:"sentence_median"`
	SentenceP90    int     `json:"sentence_p90"`
	SentenceMax    int     `json:"sentence_max"`

	SentencesPerParagraph float64 `json:"sentences_per_paragraph"`
	// DialogueShare is the fraction of characters inside quotation marks.
	DialogueShare float64 `json:"dialogue_share"`
	// Endings are the most common ways a sentence closes — its last two
	// letters — with the share of sentences that close that way.
	Endings []Ending `json:"endings"`
}

// Ending is one sentence ending and how often it occurs.
type Ending struct {
	Ending string  `json:"ending"`
	Count  int     `json:"count"`
	Share  float64 `json:"share"`
}

// Measure counts scenes. It is a pure function: no model, no judgement.
func Measure(scenes []Scene) Stats {
	st := Stats{Scenes: len(scenes), Endings: []Ending{}}
	var lengths []int
	endings := map[string]int{}
	quoted := 0
	for _, sc := range scenes {
		for _, para := range sc.Paragraphs {
			if strings.TrimSpace(para) == "" {
				continue
			}
			st.Paragraphs++
			total, inQuotes := countCharacters(para)
			st.Characters += total
			quoted += inQuotes
			for _, s := range Sentences(para) {
				lengths = append(lengths, utf8.RuneCountInString(s))
				if e := sentenceEnding(s); e != "" {
					endings[e]++
				}
			}
		}
	}
	st.Sentences = len(lengths)
	if st.Sentences == 0 {
		return st
	}
	sort.Ints(lengths)
	sum := 0
	for _, n := range lengths {
		sum += n
	}
	st.SentenceMean = round2(float64(sum) / float64(len(lengths)))
	st.SentenceMedian = percentile(lengths, 50)
	st.SentenceP90 = percentile(lengths, 90)
	st.SentenceMax = lengths[len(lengths)-1]
	st.SentencesPerParagraph = round2(float64(st.Sentences) / float64(st.Paragraphs))
	if st.Characters > 0 {
		st.DialogueShare = round2(float64(quoted) / float64(st.Characters))
	}
	st.Endings = topEndings(endings, st.Sentences)
	return st
}

// percentile returns the nearest-rank percentile of an ascending slice.
func percentile(sorted []int, p int) int {
	rank := int(math.Ceil(float64(p) / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// quote pairs that open and close with different marks. The straight double
// quote is handled apart: it is its own closer.
var quotePairs = map[rune]rune{'“': '”', '「': '」', '『': '』', '«': '»'}

// countCharacters returns the paragraph's non-space characters and how many
// of them sit inside quotation marks. The marks themselves count as outside.
// A quote left open runs to the end of the paragraph, which is how dialogue
// continuing into the next paragraph is conventionally set.
func countCharacters(para string) (total, quoted int) {
	var closer rune
	for _, r := range para {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		switch {
		case closer != 0 && r == closer:
			closer = 0
		case closer == 0 && r == '"':
			closer = '"'
		case closer == 0 && quotePairs[r] != 0:
			closer = quotePairs[r]
		case closer != 0:
			quoted++
		}
	}
	return total, quoted
}

// sentenceEnding returns the last two letters of a sentence that ends with a
// terminator, or "" for a fragment that does not — a heading-like line, a
// sentence cut off mid-paragraph — which says nothing about how this writer
// closes a sentence.
func sentenceEnding(s string) string {
	r := []rune(s)
	end := len(r)
	terminated := false
	for end > 0 && (isTerminator(r[end-1]) || isCloser(r[end-1])) {
		if isTerminator(r[end-1]) {
			terminated = true
		}
		end--
	}
	if !terminated {
		return ""
	}
	var letters []rune
	for i := end - 1; i >= 0 && len(letters) < 2; i-- {
		if !unicode.IsLetter(r[i]) {
			break
		}
		letters = append([]rune{unicode.ToLower(r[i])}, letters...)
	}
	if len(letters) < 2 {
		return ""
	}
	return string(letters)
}

func topEndings(counts map[string]int, sentences int) []Ending {
	out := make([]Ending, 0, len(counts))
	for e, n := range counts {
		out = append(out, Ending{Ending: e, Count: n, Share: round2(float64(n) / float64(sentences))})
	}
	// Most common first; ties in a fixed order so two runs agree.
	sort.Slice(out, func(a, b int) bool {
		if out[a].Count != out[b].Count {
			return out[a].Count > out[b].Count
		}
		return out[a].Ending < out[b].Ending
	})
	if len(out) > maxEndings {
		out = out[:maxEndings]
	}
	return out
}
