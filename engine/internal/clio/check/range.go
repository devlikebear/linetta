package check

import (
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/clio/voice"
)

// rangeFeatures are the habits checked against the writer's P10..P90, with
// the noun used in the message.
var rangeFeatures = []struct{ name, noun string }{
	{"emoji", "이모지"},
	{"laugh", "ㅋㅋ·ㅎㅎ"},
	{"hashtag", "해시태그"},
	{"exclaim", "느낌표"},
	{"ellipsis", "말줄임표"},
	{"lines", "줄 수"},
	{"length", "글 길이"},
}

// ranges flags a draft habit below P10 or above P90 of the context.
func ranges(body string, ctx voice.ContextProfile) []Finding {
	if ctx.N == 0 {
		return nil
	}
	f := voice.Features(body)
	vals := map[string]float64{}
	for i, n := range voice.FeatureNames {
		vals[n] = f.Values()[i]
	}
	label, note := basis(ctx)
	var out []Finding
	for _, rf := range rangeFeatures {
		st, ok := ctx.Features[rf.name]
		if !ok {
			continue
		}
		v := vals[rf.name]
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
