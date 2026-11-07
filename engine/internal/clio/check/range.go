package check

import (
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/clio/voice"
)

// rangeFeatures are the habits checked against the writer's P10..P90, with
// the noun used in the message. A habit is a count the writer either has or
// lacks; one the writer almost never shows is a medium finding of its own.
var rangeFeatures = []struct {
	name, noun string
	habit      bool
}{
	{"emoji", "이모지", true},
	{"laugh", "ㅋㅋ·ㅎㅎ", true},
	{"hashtag", "해시태그", true},
	{"exclaim", "느낌표", true},
	{"ellipsis", "말줄임표", true},
	{"lines", "줄 수", false},
	{"length", "글 길이", false},
}

func featureValues(body string) map[string]float64 {
	f := voice.Features(body).Values()
	vals := make(map[string]float64, len(f))
	for i, n := range voice.FeatureNames {
		vals[n] = f[i]
	}
	return vals
}

// ranges flags a habit the writer almost never shows (medium), and otherwise
// a draft value below P10 or above P90 of the context (weak).
func ranges(body string, ctx voice.ContextProfile, corpus Corpus) []Finding {
	if ctx.N == 0 {
		return nil
	}
	vals := featureValues(body)
	label, note := basis(ctx)
	var out []Finding
	for _, rf := range rangeFeatures {
		st, ok := ctx.Features[rf.name]
		if !ok {
			continue
		}
		v := vals[rf.name]
		if rf.habit && v > 0 && corpus.N > 0 && float64(corpus.HabitPosts[rf.name])/float64(corpus.N) <= rareShare {
			out = append(out, Finding{
				Rule: RuleRareHabit, Severity: Medium,
				Message:  fmt.Sprintf("내 글에 거의 없는 %s예요", rf.noun),
				Evidence: countEvidence(ctx, corpus.HabitPosts[rf.name], corpus.N, ""),
			})
			continue
		}
		var dir string
		switch {
		case v > st.P90:
			dir = "많아요"
		case v < st.P10:
			dir = "적어요"
		default:
			continue
		}
		out = append(out, Finding{
			Rule: RuleOutOfRange, Severity: Weak,
			Message:  fmt.Sprintf("%s가 평소보다 %s", rf.noun, dir),
			Evidence: fmt.Sprintf("내 %s 중 대부분(10~90%%)은 %g~%g, 이번 글은 %g%s", label, st.P10, st.P90, v, note),
		})
	}
	return out
}

// unfamiliarEndings flags segment endings that appear in none of the writer's
// posts, but only when the draft has at least two such segments.
func unfamiliarEndings(body string, ctx voice.ContextProfile, corpus Corpus) []Finding {
	if corpus.N == 0 {
		return nil
	}
	segs := voice.Segments(body)
	spans := voice.SegmentSpans(body)
	var out []Finding
	for i, s := range segs {
		e := voice.Ending(s)
		if e == "" || corpus.EndingPosts[e] > 0 {
			continue
		}
		out = append(out, Finding{
			Rule: RuleUnfamiliarEnding, Severity: Weak, Start: spans[i][0], End: spans[i][1],
			Message:  "내 글에서 한 번도 안 쓴 끝맺음이에요",
			Evidence: countEvidence(ctx, 0, corpus.N, fmt.Sprintf("에 '%s'로 끝나는 문장 있음", e)),
		})
	}
	if len(out) < minUnfamiliarSegments {
		return nil
	}
	return out
}
