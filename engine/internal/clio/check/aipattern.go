package check

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The sentence-pattern code below is copied from engine/internal/style
// (patterns.go: sentencePattern, isParallelContrast, endsPast, opensOnTopic,
// balanced, isDialogue; check.go: Sentences) so this package need not import
// internal/style, which pulls in tars and net/http. Keep the two in step.

var (
	// "~가 아니라 ~였다".
	notButContrast = regexp.MustCompile(`\S+[이가] 아니라 \S+[이였]+다`)
	// The narrator stopping to say what the scene meant.
	narratorExplains = regexp.MustCompile(`(였기|었기|았기|했기) 때문이다|인 셈이다|ㄴ 셈이었다|셈이었다|는 뜻이었다|의미했다|말해 주고 있었다`)
)

const maxTopicWord = 3

// aiPatterns flags sentences shaped like a paired contrast or a tidy
// explanation. They hold whatever the writer's habits are.
func aiPatterns(body string) []Finding {
	var out []Finding
	for _, sp := range sentenceSpans(body) {
		kind := sentencePattern(sp.text)
		if kind == "" {
			continue
		}
		out = append(out, Finding{
			Rule: RuleAIPattern, Severity: Strong, Start: sp.start, End: sp.end,
			Message:  "AI가 즐겨 쓰는 문장 틀이에요",
			Evidence: kind,
		})
	}
	return out
}

func sentencePattern(s string) string {
	if isDialogue(s) {
		return ""
	}
	switch {
	case isParallelContrast(s), notButContrast.MatchString(s):
		return "'A는 ~했고, B는 ~했다' 같은 짝 대조 문장"
	case narratorExplains.MatchString(s):
		return "장면의 뜻을 설명하며 맺는 문장"
	}
	return ""
}

// isParallelContrast finds "A는 ~했고, B는 ~했다": two topics given matching
// past-tense verbs in one sentence.
func isParallelContrast(s string) bool {
	s = strings.TrimRight(strings.TrimSpace(s), ".…!?\"”’' ")
	if !strings.HasSuffix(s, "다") {
		return false
	}
	for at := 0; ; {
		i := strings.Index(s[at:], "고, ")
		if i < 0 {
			return false
		}
		left, right := s[:at+i], s[at+i+len("고, "):]
		if endsPast(left) && opensOnTopic(left) && endsPast(strings.TrimSuffix(right, "다")) && opensOnTopic(right) && balanced(left, right) {
			return true
		}
		at += i + len("고, ")
	}
}

func endsPast(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r >= 0xAC00 && r <= 0xD7A3 && (r-0xAC00)%28 == 20
}

func opensOnTopic(s string) bool {
	words := strings.Fields(s)
	for i := 0; i < len(words) && i < maxTopicWord; i++ {
		if strings.HasSuffix(words[i], "은") || strings.HasSuffix(words[i], "는") {
			return true
		}
	}
	return false
}

func balanced(a, b string) bool {
	x, y := utf8.RuneCountInString(a), utf8.RuneCountInString(b)
	return x*2 >= y && y*2 >= x
}

func isDialogue(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, `"`) || strings.HasPrefix(s, "“") || strings.HasPrefix(s, "'") || strings.HasPrefix(s, "‘")
}

type span struct {
	text       string
	start, end int // rune offsets
}

// sentenceSpans splits like style.Sentences but keeps rune offsets. A sentence
// ends at . ! ? … or full-width forms (with closing quotes that follow) when
// whitespace or the end comes next, and at a line break.
func sentenceSpans(text string) []span {
	var out []span
	runes := []rune(text)
	flush := func(from, to int) {
		for from < to && unicode.IsSpace(runes[from]) {
			from++
		}
		for to > from && unicode.IsSpace(runes[to-1]) {
			to--
		}
		if from < to {
			out = append(out, span{string(runes[from:to]), from, to})
		}
	}
	start := 0
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\n' {
			flush(start, i)
			start = i + 1
			continue
		}
		if !isTerminator(runes[i]) {
			continue
		}
		end := i + 1
		for end < len(runes) && (isTerminator(runes[end]) || isCloser(runes[end])) {
			end++
		}
		if end == len(runes) || unicode.IsSpace(runes[end]) {
			flush(start, end)
			start = end
		}
		i = end - 1
	}
	flush(start, len(runes))
	return out
}

func isTerminator(r rune) bool {
	switch r {
	case '.', '!', '?', '…', '。', '！', '？':
		return true
	}
	return false
}

func isCloser(r rune) bool {
	switch r {
	case '"', '\'', ')', ']', '”', '’', '」', '』', '》', '〉':
		return true
	}
	return false
}
