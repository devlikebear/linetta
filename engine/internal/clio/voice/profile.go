package voice

import (
	"math"
	"sort"

	"github.com/devlikebear/linetta/engine/internal/clio/post"
)

const (
	// MinContextN is the fewest posts a context needs before For stops falling back.
	MinContextN = 20
	topEndings  = 8
	maxExamples = 5
)

// Stat summarises one feature over a context. Percentiles use the
// nearest-rank method: P(q) is the value at 1-based rank ceil(q*N) of the
// sorted values, so it is always an observed value.
type Stat struct {
	Mean float64 `json:"mean"`
	P10  float64 `json:"p10"`
	P50  float64 `json:"p50"`
	P90  float64 `json:"p90"`
}

// EndingShare is one ending and its share of all endings.
type EndingShare struct {
	Ending string  `json:"ending"`
	Share  float64 `json:"share"`
}

// ContextProfile is the voice of one platform/kind context.
type ContextProfile struct {
	Key         string             `json:"key"`
	Fallback    string             `json:"fallback"`
	N           int                `json:"n"`
	Features    map[string]Stat    `json:"features"`
	Endings     map[string]float64 `json:"endings"`
	Top         []EndingShare      `json:"top"`
	PoliteShare float64            `json:"polite_share"`
	// MixedShare is the share of posts with both a polite and a plain segment,
	// as classified by Register.
	MixedShare float64  `json:"mixed_share"`
	Examples   []string `json:"examples"`
}

// Profile holds a ContextProfile per context key: "x/post", "x/reply",
// "x/*" and "*" (and likewise for other platforms present).
type Profile struct {
	Contexts map[string]ContextProfile `json:"contexts"`
}

// Build summarises posts per context. Order of posts only affects Examples:
// the first five of each context, in input order.
func Build(posts []post.Post) Profile {
	groups := map[string][]post.Post{}
	add := func(k string, p post.Post) { groups[k] = append(groups[k], p) }
	for _, p := range posts {
		add(p.Platform+"/"+p.Kind, p)
		add(p.Platform+"/*", p)
		add("*", p)
	}
	out := Profile{Contexts: make(map[string]ContextProfile, len(groups))}
	for k, g := range groups {
		out.Contexts[k] = buildContext(k, g)
	}
	return out
}

func buildContext(key string, posts []post.Post) ContextProfile {
	cp := ContextProfile{Key: key, Fallback: key, N: len(posts), Features: map[string]Stat{}, Top: []EndingShare{}, Examples: []string{}}
	bodies := make([]string, len(posts))
	cols := make([][]float64, len(FeatureNames))
	var mixed int
	for i, p := range posts {
		bodies[i] = p.Body
		for j, v := range Features(p.Body).Values() {
			cols[j] = append(cols[j], v)
		}
		var hasPolite, hasPlain bool
		for _, s := range Segments(p.Body) {
			switch Register(s) {
			case 1:
				hasPolite = true
			case -1:
				hasPlain = true
			}
		}
		if hasPolite && hasPlain {
			mixed++
		}
		if len(cp.Examples) < maxExamples {
			cp.Examples = append(cp.Examples, p.ID)
		}
	}
	for j, name := range FeatureNames {
		cp.Features[name] = summarize(cols[j])
	}
	cp.PoliteShare = cp.Features["polite"].Mean
	if len(posts) > 0 {
		cp.MixedShare = float64(mixed) / float64(len(posts))
	}
	cp.Endings = EndingsOf(bodies)
	for e, s := range cp.Endings {
		cp.Top = append(cp.Top, EndingShare{e, s})
	}
	sort.Slice(cp.Top, func(i, j int) bool {
		if cp.Top[i].Share != cp.Top[j].Share {
			return cp.Top[i].Share > cp.Top[j].Share
		}
		return cp.Top[i].Ending < cp.Top[j].Ending
	})
	if len(cp.Top) > topEndings {
		cp.Top = cp.Top[:topEndings]
	}
	return cp
}

func summarize(v []float64) Stat {
	if len(v) == 0 {
		return Stat{}
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	var sum float64
	for _, x := range s {
		sum += x
	}
	return Stat{Mean: sum / float64(len(s)), P10: nearestRank(s, 0.10), P50: nearestRank(s, 0.50), P90: nearestRank(s, 0.90)}
}

func nearestRank(sorted []float64, q float64) float64 {
	r := int(math.Ceil(q*float64(len(sorted)) - 1e-9))
	return sorted[min(max(r, 1), len(sorted))-1]
}

// For returns the profile for platform/kind, falling back to platform/* and
// then * while a context holds fewer than MinContextN posts. Key is the context
// asked for and Fallback the key actually used. If nothing qualifies it returns the widest context found
// (or an empty one for an empty Profile).
func (p Profile) For(platform, kind string) ContextProfile {
	want := platform + "/" + kind
	var last ContextProfile
	for _, k := range []string{want, platform + "/*", "*"} {
		c, ok := p.Contexts[k]
		if !ok {
			continue
		}
		last = c
		if c.N >= MinContextN {
			break
		}
	}
	if last.Key == "" {
		last = ContextProfile{Key: "*", Features: map[string]Stat{}, Endings: map[string]float64{}, Top: []EndingShare{}, Examples: []string{}}
	}
	last.Fallback = last.Key
	last.Key = want
	return last
}
