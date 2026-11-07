// Package check points at habits in a draft that the writer's own posts do not
// have: AI phrasing, a register out of place, and features outside the usual
// range. It is deterministic and calls no model. It may import only the
// standard library, clio/voice and clio/post, so it never pulls in the network.
package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/clio/post"
	"github.com/devlikebear/linetta/engine/internal/clio/voice"
)

// Severity is how sure a finding is.
type Severity string

const (
	Strong Severity = "strong"
	Medium Severity = "medium"
	Weak   Severity = "weak"
)

// Rule names as they appear in a Finding.
const (
	RuleAIPhrase         = "ai_phrase"
	RuleAIPattern        = "ai_pattern"
	RuleRegisterMismatch = "register_mismatch"
	RuleRegisterMixed    = "register_mixed"
	RuleOutOfRange       = "out_of_range"
	RuleUnfamiliarEnding = "unfamiliar_ending"
)

// minUnfamiliarSegments is how many unseen endings a draft needs before any is flagged.
const minUnfamiliarSegments = 2

const (
	// rareShare is the most of the writer's posts a habit may appear in and
	// still count as foreign to them.
	rareShare = 0.02
	// mixedMax is the largest share of mixed-register posts for which a mixed
	// draft is called unusual.
	mixedMax = 0.10
)

// Finding is one place a draft differs from the writer's voice.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	// Start and End are rune offsets into the body; range rules use 0, 0.
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Message  string `json:"message"`
	Evidence string `json:"evidence"`
}

// Corpus is how often habits occur in the posts of one context, counted so a
// check needs no access to the posts themselves.
type Corpus struct {
	// Key is the profile context key the posts were taken from.
	Key string `json:"key"`
	N   int    `json:"n"`
	// TellPosts maps each voice.AITells phrase to the posts containing it.
	TellPosts map[string]int `json:"tell_posts"`
	// PolitePosts and PlainPosts count posts with more polite than plain
	// segments, and more plain than polite. MixedPosts counts posts with both,
	// the same definition as voice.ContextProfile.MixedShare.
	PolitePosts int `json:"polite_posts"`
	PlainPosts  int `json:"plain_posts"`
	MixedPosts  int `json:"mixed_posts"`
	// EndingPosts maps a segment ending to the posts that have it.
	EndingPosts map[string]int `json:"ending_posts"`
}

// BuildCorpus counts habits over the posts that voice.Build groups under key:
// "x/post" is exact, "x/*" a whole platform, "*" everything.
func BuildCorpus(posts []post.Post, key string) Corpus {
	c := Corpus{Key: key, TellPosts: map[string]int{}, EndingPosts: map[string]int{}}
	for _, p := range posts {
		if !inContext(p, key) {
			continue
		}
		c.N++
		for _, t := range voice.AITells {
			if strings.Contains(p.Body, t) {
				c.TellPosts[t]++
			}
		}
		var polite, plain int
		endings := map[string]bool{}
		for _, s := range voice.Segments(p.Body) {
			e := voice.Ending(s)
			if e == "" {
				continue
			}
			endings[e] = true
			if voice.IsPolite(e) {
				polite++
			} else {
				plain++
			}
		}
		for e := range endings {
			c.EndingPosts[e]++
		}
		switch {
		case polite > plain:
			c.PolitePosts++
		case plain > polite:
			c.PlainPosts++
		}
		if polite > 0 && plain > 0 {
			c.MixedPosts++
		}
	}
	return c
}

func inContext(p post.Post, key string) bool {
	switch {
	case key == "*":
		return true
	case strings.HasSuffix(key, "/*"):
		return p.Platform+"/*" == key
	}
	return p.Platform+"/"+p.Kind == key
}

// Run checks a draft against the writer's context profile and corpus. The
// result is never nil and is sorted by position, then severity, then rule.
func Run(body string, ctx voice.ContextProfile, corpus Corpus) []Finding {
	out := []Finding{}
	if strings.TrimSpace(body) == "" {
		return out
	}
	out = append(out, aiPhrases(body, ctx, corpus)...)
	out = append(out, aiPatterns(body)...)
	out = append(out, registers(body, ctx, corpus)...)
	out = append(out, ranges(body, ctx)...)
	out = append(out, unfamiliarEndings(body, ctx, corpus)...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if ra, rb := rank(a.Severity), rank(b.Severity); ra != rb {
			return ra < rb
		}
		return a.Rule < b.Rule
	})
	return out
}

func rank(s Severity) int {
	switch s {
	case Strong:
		return 0
	case Medium:
		return 1
	}
	return 2
}

// contextLabel names a profile key in the writer's terms.
func contextLabel(key string) string {
	switch key {
	case "x/post":
		return "X 글"
	case "x/reply":
		return "X 답글"
	case "x/*":
		return "X 전체"
	case "threads/post":
		return "Threads 글"
	case "threads/reply":
		return "Threads 답글"
	case "threads/*":
		return "Threads 전체"
	case "blog/*", "blog/post", "blog/reply":
		return "블로그"
	case "*":
		return "전체"
	}
	return key
}

// basis is the context the numbers came from and, when the profile fell back
// to a wider one, a note saying so.
func basis(ctx voice.ContextProfile) (label, note string) {
	key := ctx.Fallback
	if key == "" {
		key = ctx.Key
	}
	label = contextLabel(key)
	if ctx.Fallback != "" && ctx.Fallback != ctx.Key {
		note = fmt.Sprintf(" (%s 기준)", label)
	}
	return label, note
}

// countEvidence reads "내 X 글 112편 중 N편에 있음".
func countEvidence(ctx voice.ContextProfile, n, of int, what string) string {
	label, note := basis(ctx)
	if what == "" {
		what = "에 있음"
	}
	return fmt.Sprintf("내 %s %d편 중 %d편%s%s", label, of, n, what, note)
}

// runeOffset converts a byte offset in s to a rune offset.
func runeOffset(s string, byteOff int) int {
	return len([]rune(s[:byteOff]))
}
