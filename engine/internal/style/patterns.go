package style

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// The patterns below catch prose shapes a list of phrases cannot: the paired
// contrast and the narrator's tidy conclusion, in Korean, plus lines and endings
// that recur across scenes in any language. They report and change nothing. A
// writer may mean every one of them.
var (
	// "A는 ~했고, B는 ~했다": two topics given matching verbs in one sentence.
	parallelContrast = regexp.MustCompile(`\S+[은는] [^,]*[었았였겠]고, (?:\S+ ){0,3}\S+[은는] [^,]*[었았였겠]다`)
	// "A는 생각보다 ~. B는 생각보다 ~" and "~가 아니라 ~였다".
	notButContrast = regexp.MustCompile(`\S+[이가] 아니라 \S+[이였]+다`)
	// The narrator stopping to say what the scene meant.
	narratorExplains = regexp.MustCompile(`(였기|었기|았기|했기) 때문이다|인 셈이다|ㄴ 셈이었다|셈이었다|는 뜻이었다|의미했다|말해 주고 있었다`)
)

// A repeated line has to be long enough that repeating it is a choice, and two
// endings have to share this much of their opening to read as one formula.
const (
	minRepeatedRunes = 12
	minFormulaRunes  = 4
)

func sentencePattern(s string) string {
	if isDialogue(s) {
		return ""
	}
	switch {
	case parallelContrast.MatchString(s), notButContrast.MatchString(s):
		return RuleParallel
	case narratorExplains.MatchString(s):
		return RuleExplainer
	}
	return ""
}

// isDialogue spares speech: people talk in pairs and conclusions, and a
// character's phrasing is not the narrator's habit.
func isDialogue(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, `"`) || strings.HasPrefix(s, "“") || strings.HasPrefix(s, "'") || strings.HasPrefix(s, "‘")
}

// acrossScenes finds what only shows when scenes are read together: a sentence
// used again word for word, and consecutive scenes that end the same way.
func acrossScenes(scenes []Scene) []Violation {
	var out []Violation
	seen := map[string]string{}
	for _, sc := range scenes {
		para := 0
		for _, text := range sc.Paragraphs {
			if strings.TrimSpace(text) == "" {
				continue
			}
			para++
			for _, s := range Sentences(text) {
				key := strings.Join(strings.Fields(s), " ")
				if isDialogue(key) || utf8.RuneCountInString(key) < minRepeatedRunes {
					continue
				}
				first, ok := seen[key]
				if !ok {
					seen[key] = sc.Label
					continue
				}
				if first != sc.Label {
					out = append(out, Violation{
						NodeID: sc.NodeID, Label: sc.Label, Rule: RuleRepeatedLine, Paragraph: para,
						Phrase: first, Excerpt: clip(key, excerptContext*3),
					})
				}
			}
		}
	}
	prev := ""
	prevLabel := ""
	for _, sc := range scenes {
		last, para := lastSentence(sc)
		if last == "" {
			continue
		}
		if prev != "" {
			if shared := sharedPrefix(prev, last); utf8.RuneCountInString(shared) >= minFormulaRunes {
				out = append(out, Violation{
					NodeID: sc.NodeID, Label: sc.Label, Rule: RuleEndingFormula, Paragraph: para,
					Phrase: prevLabel, Excerpt: clip(last, excerptContext*3),
				})
			}
		}
		prev, prevLabel = last, sc.Label
	}
	return out
}

func lastSentence(sc Scene) (string, int) {
	last, para, count := "", 0, 0
	for _, text := range sc.Paragraphs {
		if strings.TrimSpace(text) == "" {
			continue
		}
		count++
		if ss := Sentences(text); len(ss) > 0 {
			last, para = ss[len(ss)-1], count
		}
	}
	return strings.TrimSpace(last), para
}

func sharedPrefix(a, b string) string {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return strings.TrimSpace(string(ra[:n]))
}
