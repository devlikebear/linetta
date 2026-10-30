package continuity

import (
	"strings"
	"testing"
)

// fakeLookup is a fixed, in-memory RefLookup for table-driven tests — no
// database, so Validate's own logic is what's under test, not a repo.
type fakeLookup struct {
	entities map[string]bool
	facts    map[string]bool
	beats    map[string]bool
	scenes   map[string][]string
}

func (f fakeLookup) HasEntity(id string) bool { return f.entities[id] }
func (f fakeLookup) HasFact(id string) bool   { return f.facts[id] }
func (f fakeLookup) HasBeat(id string) bool   { return f.beats[id] }
func (f fakeLookup) SceneParagraphs(nodeID string) ([]string, bool) {
	p, ok := f.scenes[nodeID]
	return p, ok
}

func validObservation() ObservationInput {
	return ObservationInput{
		Category:      CategoryCharacter,
		Kind:          KindContradiction,
		Severity:      SeverityMedium,
		Summary:       "하린이 왼손잡이라는 설정과 맞지 않는다",
		Suggestion:    "오른손 표현을 왼손으로 바꾸거나 설정을 확인한다",
		ParagraphFrom: 1,
		ParagraphTo:   1,
		Quote:         "하린은 오른손으로 검을 뽑았다.",
		Refs:          []Ref{{Type: RefTypeEntity, ID: "e-harin"}},
	}
}

func TestValidate_tableDriven(t *testing.T) {
	paragraphs := []string{
		"하린은 오른손으로 검을 뽑았다.",
		"두 번째 문단이다.",
	}
	lookup := fakeLookup{
		entities: map[string]bool{"e-harin": true},
		facts:    map[string]bool{"f-1": true},
		beats:    map[string]bool{"b-1": true},
		scenes:   map[string][]string{"scene-prev": {"이전 회차의 문단.", "앞 씬 둘째 문단이다."}},
	}

	tests := []struct {
		name    string
		in      ReportInput
		wantErr bool
		// substr is checked against every problem's Message concatenated,
		// when wantErr is true — so a test both proves an error fired and
		// pins the wording an agent would see.
		substr string
	}{
		{
			name: "valid observation passes clean",
			in:   ReportInput{NodeID: "n1", SceneVersion: 1, Observations: []ObservationInput{validObservation()}},
		},
		{
			name: "empty observations is a valid clean check",
			in:   ReportInput{NodeID: "n1", SceneVersion: 1, Observations: nil},
		},
		{
			name: "paragraph range out of bounds",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput { o := validObservation(); o.ParagraphFrom, o.ParagraphTo = 1, 41; return o }(),
			}},
			wantErr: true,
			substr:  "observation 1: paragraph 41 is out of range; the scene has 2 paragraphs",
		},
		{
			name: "paragraph_to before paragraph_from",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput { o := validObservation(); o.ParagraphFrom, o.ParagraphTo = 2, 1; return o }(),
			}},
			wantErr: true,
			substr:  "paragraph_to 1 is before paragraph_from 2",
		},
		{
			name: "quote differing only in whitespace passes",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput {
					o := validObservation()
					o.Quote = "하린은   오른손으로\n검을 뽑았다."
					return o
				}(),
			}},
		},
		{
			name: "invented quote is rejected",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput {
					o := validObservation()
					o.Quote = "이런 문장은 본문에 없다."
					return o
				}(),
			}},
			wantErr: true,
			substr:  "quote not found in paragraph 1",
		},
		{
			name: "entity ref from another work is rejected",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput {
					o := validObservation()
					o.Refs = []Ref{{Type: RefTypeEntity, ID: "e-other-work"}}
					return o
				}(),
			}},
			wantErr: true,
			substr:  "entity \"e-other-work\" not found in this work",
		},
		{
			name: "contradiction with no refs is rejected",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput { o := validObservation(); o.Refs = nil; return o }(),
			}},
			wantErr: true,
			substr:  "kind is contradiction but has no refs",
		},
		{
			name: "gap and question need no refs",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput { o := validObservation(); o.Kind, o.Refs = KindGap, nil; return o }(),
				func() ObservationInput { o := validObservation(); o.Kind, o.Refs = KindQuestion, nil; return o }(),
			}},
		},
		{
			name: "fact and beat refs resolve",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput {
					o := validObservation()
					o.Refs = []Ref{{Type: RefTypeFact, ID: "f-1"}, {Type: RefTypeBeat, ID: "b-1"}}
					return o
				}(),
			}},
		},
		{
			name: "scene ref to a real leaf with a real quote resolves",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput {
					o := validObservation()
					o.Refs = []Ref{{Type: RefTypeScene, ID: "scene-prev", Paragraph: 1, Quote: "이전 회차의 문단."}}
					return o
				}(),
			}},
		},
		{
			name: "scene ref with a fabricated quote is rejected",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput {
					o := validObservation()
					o.Refs = []Ref{{Type: RefTypeScene, ID: "scene-prev", Paragraph: 1, Quote: "지어낸 앞 씬 인용."}}
					return o
				}(),
			}},
			wantErr: true,
			substr:  "quote not found in scene",
		},
		{
			name: "scene ref to an unknown node is rejected",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				func() ObservationInput {
					o := validObservation()
					o.Refs = []Ref{{Type: RefTypeScene, ID: "no-such-scene"}}
					return o
				}(),
			}},
			wantErr: true,
			substr:  "scene \"no-such-scene\" is not a leaf scene in this work",
		},
		{
			name: "invalid category, kind, severity, empty summary and quote all reported together",
			in: ReportInput{NodeID: "n1", Observations: []ObservationInput{
				{Category: "nonsense", Kind: "nonsense", Severity: "nonsense", ParagraphFrom: 1, ParagraphTo: 1},
			}},
			wantErr: true,
		},
		{
			name: "more than 20 observations is refused as a whole",
			in: ReportInput{NodeID: "n1", Observations: func() []ObservationInput {
				out := make([]ObservationInput, 21)
				for i := range out {
					out[i] = validObservation()
				}
				return out
			}()},
			wantErr: true,
			substr:  "report has 21 observations",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := Validate(tc.in, paragraphs, lookup)
			if tc.wantErr && len(problems) == 0 {
				t.Fatalf("Validate() returned no problems, want at least one")
			}
			if !tc.wantErr && len(problems) != 0 {
				t.Fatalf("Validate() = %+v, want no problems", problems)
			}
			if tc.substr != "" {
				joined := joinMessages(problems)
				if !strings.Contains(joined, tc.substr) {
					t.Errorf("Validate() messages = %q, want substring %q", joined, tc.substr)
				}
			}
		})
	}
}

// TestValidate_reportsEveryProblemAtOnce pins the "all problems, not just
// the first" contract: an agent that fixes every field Validate complained
// about should need exactly one more call, not a retry per field.
func TestValidate_reportsEveryProblemAtOnce(t *testing.T) {
	in := ReportInput{NodeID: "n1", Observations: []ObservationInput{
		{Category: "nonsense", Kind: "nonsense", Severity: "nonsense", ParagraphFrom: 0, ParagraphTo: 0, Quote: ""},
	}}
	problems := Validate(in, []string{"하나.", "둘."}, fakeLookup{})
	// category, kind, severity, summary, quote, paragraph_from — six
	// independent defects on one observation, all in one pass.
	if got := len(problems); got < 6 {
		t.Fatalf("Validate() returned %d problems, want at least 6 (one call should surface every defect): %+v", got, problems)
	}
}

func joinMessages(problems []Problem) string {
	var b strings.Builder
	for _, p := range problems {
		b.WriteString(p.Message)
		b.WriteString("\n")
	}
	return b.String()
}
