// Package voice counts a writer's habits in short posts and summarises them
// per context. It is a port of scripts/eval-voice.py: the definitions of a
// segment, an ending and a polite ending are the same, so the two agree.
package voice

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// MinLetters is the fewest Hangul and Latin letters a post needs to be kept.
const MinLetters = 15

// pySpace is Python's Unicode-aware \s (str.isspace). RE2's \s is ASCII-only
// and lacks \v, so URL and hashtag boundaries would differ on NBSP, U+3000 etc.
const pySpace = `\x09-\x0d\x1c-\x20\x85\xa0\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}`

var (
	// URLRe matches an http(s) link up to the next whitespace.
	URLRe     = regexp.MustCompile(`https?://[^` + pySpace + `]+`)
	emojiRe   = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}\x{1F1E6}-\x{1F1FF}]`)
	laughRe   = regexp.MustCompile(`[ㅋㅎ]{2,}`)
	cryRe     = regexp.MustCompile(`[ㅠㅜ]{2,}`)
	hashtagRe = regexp.MustCompile(`#[^` + pySpace + `#]+`)
	ellipsis  = regexp.MustCompile(`\.{2,}|…`)
	segmentRe = regexp.MustCompile(`[^.!?…\n]+[.!?…]*`)
	hangulRe  = regexp.MustCompile(`[가-힣]`)
	latinRe   = regexp.MustCompile(`[A-Za-z]`)
)

// Polite are the endings that count as 존댓말, matched with HasSuffix on a
// segment's two-letter ending.
var Polite = []string{"요", "니다", "세요", "까요", "죠"}

// AITells are phrases a model reaches for and a person posting rarely does.
// Matched as written, counted without overlap.
var AITells = []string{
	"결론적으로", "요약하자면", "핵심은", "중요한 것은", "중요한 건", "다양한", "효과적", "진정한",
	"여정", "인사이트", "에 대해 알아보", "여러분", "해 보세요", "해보세요", "어떠신가요", "단순히",
	"뿐만 아니라", "이처럼", "그렇다면", "무엇보다", "한 걸음", "되새기", "소중한", "깨달았",
	"—", "**", "##", "✨", "🚀", "💡", "👉", "🙌",
}

// Feature holds the counted habits of one post.
type Feature struct {
	Length        float64 `json:"length"`
	Lines         float64 `json:"lines"`
	SegmentLength float64 `json:"segment_length"`
	Punctuated    float64 `json:"punctuated"`
	Polite        float64 `json:"polite"`
	Emoji         float64 `json:"emoji"`
	Laugh         float64 `json:"laugh"`
	Cry           float64 `json:"cry"`
	Hashtag       float64 `json:"hashtag"`
	Ellipsis      float64 `json:"ellipsis"`
	Exclaim       float64 `json:"exclaim"`
	AITells       float64 `json:"ai_tells"`
}

// FeatureNames lists the features in a fixed order, matching Feature.Values.
var FeatureNames = []string{"length", "lines", "segment_length", "punctuated", "polite", "emoji", "laugh", "cry", "hashtag", "ellipsis", "exclaim", "ai_tells"}

// Values returns the feature values in FeatureNames order.
func (f Feature) Values() []float64 {
	return []float64{f.Length, f.Lines, f.SegmentLength, f.Punctuated, f.Polite, f.Emoji, f.Laugh, f.Cry, f.Hashtag, f.Ellipsis, f.Exclaim, f.AITells}
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// Segments splits a post into sentence-like pieces: runs of text up to and
// including trailing . ! ? … marks, kept only if they hold Hangul. Unlike
// style.Sentences it does not need terminal punctuation.
func Segments(post string) []string {
	var out []string
	for _, s := range segmentRe.FindAllString(post, -1) {
		if hangulRe.MatchString(s) {
			// Python str.strip() strips Unicode whitespace; so does TrimSpace.
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// Ending is the last two Hangul letters of a segment, or "" with fewer than two.
func Ending(seg string) string {
	l := hangulRe.FindAllString(seg, -1)
	if len(l) < 2 {
		return ""
	}
	return l[len(l)-2] + l[len(l)-1]
}

// IsPolite reports whether an ending is polite.
func IsPolite(ending string) bool {
	for _, p := range Polite {
		if strings.HasSuffix(ending, p) {
			return true
		}
	}
	return false
}

// Features counts one post. Rounding is left to callers.
func Features(post string) Feature {
	segs := Segments(post)
	n := float64(max(len(segs), 1))
	var segLen float64
	var punct, polite float64
	for _, s := range segs {
		segLen += float64(runes(s))
		if last, _ := utf8.DecodeLastRuneInString(s); strings.ContainsRune(".!?…", last) {
			punct++
		}
		if IsPolite(Ending(s)) {
			polite++
		}
	}
	if len(segs) > 0 {
		segLen /= float64(len(segs))
	}
	var tells int
	for _, t := range AITells {
		tells += strings.Count(post, t)
	}
	return Feature{
		Length:        float64(runes(post)),
		Lines:         float64(strings.Count(post, "\n") + 1),
		SegmentLength: segLen,
		Punctuated:    punct / n,
		Polite:        polite / n,
		Emoji:         float64(len(emojiRe.FindAllStringIndex(post, -1))),
		Laugh:         float64(len(laughRe.FindAllStringIndex(post, -1))),
		Cry:           float64(len(cryRe.FindAllStringIndex(post, -1))),
		Hashtag:       float64(len(hashtagRe.FindAllStringIndex(post, -1))),
		Ellipsis:      float64(len(ellipsis.FindAllStringIndex(post, -1))),
		Exclaim:       float64(strings.Count(post, "!")),
		AITells:       float64(tells),
	}
}

// EndingsOf is the share each ending has among all endings in the posts.
func EndingsOf(posts []string) map[string]float64 {
	counts := map[string]int{}
	total := 0
	for _, p := range posts {
		for _, s := range Segments(p) {
			if e := Ending(s); e != "" {
				counts[e]++
				total++
			}
		}
	}
	if total == 0 {
		total = 1
	}
	out := make(map[string]float64, len(counts))
	for e, n := range counts {
		out[e] = float64(n) / float64(total)
	}
	return out
}

// Letters counts Hangul syllables and ASCII letters, the measure of whether a
// post is long enough to learn from.
func Letters(s string) int {
	return len(hangulRe.FindAllStringIndex(s, -1)) + len(latinRe.FindAllStringIndex(s, -1))
}
