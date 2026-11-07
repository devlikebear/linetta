package check

import (
	"fmt"
	"math"
	"unicode"

	"github.com/devlikebear/linetta/engine/internal/clio/voice"
)

type regSeg struct {
	start, end int
	polite     bool
}

// registerSegments are the draft's segments that have a Hangul ending and sit
// outside quotation marks, with their register.
func registerSegments(body string) []regSeg {
	quotes := quoteRanges([]rune(body))
	r := []rune(body)
	var out []regSeg
	spans := voice.SegmentSpans(body)
	for i, s := range voice.Segments(body) {
		sp := spans[i]
		reg := voice.Register(s)
		if reg == 0 || inQuote(quotes, lastHangul(r, sp)) {
			continue
		}
		out = append(out, regSeg{sp[0], sp[1], reg == 1})
	}
	return out
}

func lastHangul(r []rune, sp [2]int) int {
	for i := sp[1] - 1; i >= sp[0]; i-- {
		if r[i] >= '가' && r[i] <= '힣' {
			return i
		}
	}
	return sp[0]
}

func inQuote(q [][2]int, at int) bool {
	for _, x := range q {
		if at >= x[0] && at < x[1] {
			return true
		}
	}
	return false
}

// quoteRanges finds closed quotations: "…", “…”, '…', ‘…’ and 「…」. A straight
// single quote opens only at a word boundary, so apostrophes do not count. An
// unclosed quote is ignored.
func quoteRanges(r []rune) [][2]int {
	var out [][2]int
	closeOf := map[rune]rune{'"': '"', '“': '”', '‘': '’', '「': '」', '\'': '\''}
	for i := 0; i < len(r); i++ {
		c, ok := closeOf[r[i]]
		if !ok {
			continue
		}
		if r[i] == '\'' && i > 0 && (unicode.IsLetter(r[i-1]) || unicode.IsDigit(r[i-1])) {
			continue
		}
		for j := i + 1; j < len(r); j++ {
			if r[j] != c {
				continue
			}
			if r[j] == '\'' && j+1 < len(r) && (unicode.IsLetter(r[j+1]) || unicode.IsDigit(r[j+1])) {
				continue
			}
			out = append(out, [2]int{i, j + 1})
			i = j
			break
		}
	}
	return out
}

func registerName(polite bool) string {
	if polite {
		return "존댓말"
	}
	return "반말"
}

// registers flags segments whose register the writer almost never uses in
// this context (register_mismatch), and the minority register inside a draft
// that mixes both when the writer rarely mixes (register_mixed).
func registers(body string, ctx voice.ContextProfile, corpus Corpus) []Finding {
	segs := registerSegments(body)
	if len(segs) == 0 {
		return nil
	}
	var polite, plain int
	for _, s := range segs {
		if s.polite {
			polite++
		} else {
			plain++
		}
	}
	// The first segment's register is the majority on a tie.
	majority := segs[0].polite
	if polite > plain {
		majority = true
	} else if plain > polite {
		majority = false
	}
	var out []Finding
	if corpus.N > 0 {
		dominated := corpus.PlainPosts
		if majority {
			dominated = corpus.PolitePosts
		}
		if float64(dominated)/float64(corpus.N) <= rareShare {
			for _, s := range segs {
				if s.polite == majority {
					out = append(out, Finding{
						Rule: RuleRegisterMismatch, Severity: Medium, Start: s.start, End: s.end,
						Message:  fmt.Sprintf("이 맥락에서 거의 안 쓰는 %s이에요", registerName(majority)),
						Evidence: countEvidence(ctx, dominated, corpus.N, "이 "+registerName(majority)+" 우세"),
					})
				}
			}
		}
	}
	if polite > 0 && plain > 0 && ctx.N > 0 && ctx.MixedShare <= mixedMax {
		label, note := basis(ctx)
		for _, s := range segs {
			if s.polite != majority {
				out = append(out, Finding{
					Rule: RuleRegisterMixed, Severity: Medium, Start: s.start, End: s.end,
					Message:  "한 글에 존댓말과 반말이 섞였어요",
					Evidence: fmt.Sprintf("내 %s 중 두 말투가 섞인 글은 %d%%%s", label, int(math.Round(ctx.MixedShare*100)), note),
				})
			}
		}
	}
	return out
}
