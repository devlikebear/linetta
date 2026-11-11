package check

import (
	"strings"

	"github.com/devlikebear/linetta/engine/internal/clio/voice"
)

// aiPhrases flags each occurrence of an AI tell that appears in at most
// tellRare of the writer's posts. A phrase the writer uses is not flagged.
func aiPhrases(body string, ctx voice.ContextProfile, corpus Corpus) []Finding {
	if corpus.N == 0 {
		return nil
	}
	var out []Finding
	for _, tell := range voice.AITells {
		n := corpus.TellPosts[tell]
		if float64(n)/float64(corpus.N) > tellRare {
			continue
		}
		for from := 0; from < len(body); {
			i := strings.Index(body[from:], tell)
			if i < 0 {
				break
			}
			at := from + i
			start := runeOffset(body, at)
			out = append(out, Finding{
				Rule: RuleAIPhrase, Severity: Strong,
				Start: start, End: start + len([]rune(tell)),
				Message:  "내 글에 거의 없는 AI 표현이에요",
				Evidence: countEvidence(ctx, n, corpus.N, "에 있음"),
			})
			from = at + len(tell)
		}
	}
	return out
}
