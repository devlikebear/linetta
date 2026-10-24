//go:build !mobile

package mcphost

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/continuity"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type continuityFixture struct {
	d       ToolDeps
	p       project.Project
	node    string
	entity  entity.Entity
	changed int
}

func newContinuityFixture(t *testing.T) (context.Context, *continuityFixture) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	entities := entity.NewRepo(st)
	facts := fact.NewRepo(st)
	beats := beat.NewRepo(st)
	threads := thread.NewRepo(st)

	p, err := projects.Create(ctx, 1, project.NewInput{
		Title: "T", Genres: []string{}, LengthTarget: "series", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	e, err := entities.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "하린"})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}

	f := &continuityFixture{p: p, node: *p.LastOpenedNodeID, entity: e}
	f.d = ToolDeps{
		Projects:   projects,
		Nodes:      nodes,
		Entities:   entities,
		Facts:      facts,
		Beats:      beats,
		Threads:    threads,
		Continuity: continuity.NewRepo(st),
		Clock:      func() int64 { return 1000 },
		Notify: func(method string, _ any) {
			if method == "mcp.changed" {
				f.changed++
			}
		},
	}
	return ctx, f
}

func (f *continuityFixture) setText(t *testing.T, ctx context.Context, text string) int {
	t.Helper()
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
	if err := f.d.Nodes.UpdateContent(ctx, f.node, doc, 1500); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	n, err := f.d.Nodes.Get(ctx, f.node)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return n.ContentVersion
}

func cleanContinuityObservation() continuity.ObservationInput {
	return continuity.ObservationInput{
		Category: continuity.CategoryOther, Kind: continuity.KindQuestion, Severity: continuity.SeverityLow,
		Summary: "확인이 필요하다", ParagraphFrom: 1, ParagraphTo: 1, Quote: "하린은 오른손으로 검을 뽑았다.",
	}
}

func TestReportContinuity_recordsACleanCheck(t *testing.T) {
	ctx, f := newContinuityFixture(t)
	version := f.setText(t, ctx, "하린은 오른손으로 검을 뽑았다.")

	res, out, err := f.d.reportContinuity(ctx, nil, reportContinuityInput{
		NodeID: f.node, SceneVersion: version, Observations: nil,
	})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("reportContinuity: res=%+v err=%v", res, err)
	}
	if out.Recorded != 0 {
		t.Errorf("Recorded = %d, want 0 for a clean check", out.Recorded)
	}
	if out.Status != continuity.ReportStatusWaitingForWriter {
		t.Errorf("Status = %q, want %q", out.Status, continuity.ReportStatusWaitingForWriter)
	}
	if f.changed != 1 {
		t.Errorf("mcp.changed fired %d times, want 1", f.changed)
	}
}

func TestReportContinuity_recordsAContradictionWithAnEntityRef(t *testing.T) {
	ctx, f := newContinuityFixture(t)
	version := f.setText(t, ctx, "하린은 오른손으로 검을 뽑았다.")

	obs := continuity.ObservationInput{
		Category: continuity.CategoryCharacter, Kind: continuity.KindContradiction, Severity: continuity.SeverityMedium,
		Summary: "하린은 왼손잡이라는 설정과 맞지 않는다", ParagraphFrom: 1, ParagraphTo: 1,
		Quote: "하린은 오른손으로 검을 뽑았다.",
		Refs:  []continuity.Ref{{Type: continuity.RefTypeEntity, ID: f.entity.ID}},
	}
	res, out, err := f.d.reportContinuity(ctx, nil, reportContinuityInput{
		NodeID: f.node, SceneVersion: version, Observations: []continuity.ObservationInput{obs},
	})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("reportContinuity: res=%+v err=%v", res, err)
	}
	if out.Recorded != 1 {
		t.Fatalf("Recorded = %d, want 1", out.Recorded)
	}

	checks, err := f.d.Continuity.List(ctx, continuity.ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(checks) != 1 || len(checks[0].Observations) != 1 {
		t.Fatalf("List = %+v, want one check with one observation", checks)
	}
	if got := checks[0].Observations[0].Refs[0].ID; got != f.entity.ID {
		t.Errorf("stored ref id = %q, want %q", got, f.entity.ID)
	}
}

// The version check: a scene edited after it was read must be refused with a
// message that tells the model exactly what to do next.
func TestReportContinuity_refusesAStaleSceneVersion(t *testing.T) {
	ctx, f := newContinuityFixture(t)
	version := f.setText(t, ctx, "첫 본문.")
	f.setText(t, ctx, "바뀐 본문.")

	res, _, err := f.d.reportContinuity(ctx, nil, reportContinuityInput{
		NodeID: f.node, SceneVersion: version, Observations: nil,
	})
	if err != nil {
		t.Fatalf("reportContinuity: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("a stale scene_version must be refused as a tool error")
	}
}

// An invented quote never reaches storage: Validate's refusal becomes the
// tool's error, and nothing is recorded.
func TestReportContinuity_refusesAnInventedQuote(t *testing.T) {
	ctx, f := newContinuityFixture(t)
	version := f.setText(t, ctx, "하린은 오른손으로 검을 뽑았다.")

	obs := cleanContinuityObservation()
	obs.Quote = "이런 문장은 본문에 없다."
	res, _, err := f.d.reportContinuity(ctx, nil, reportContinuityInput{
		NodeID: f.node, SceneVersion: version, Observations: []continuity.ObservationInput{obs},
	})
	if err != nil {
		t.Fatalf("reportContinuity: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("an invented quote must be refused")
	}
	checks, err := f.d.Continuity.List(ctx, continuity.ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(checks) != 0 {
		t.Errorf("a refused report left %d checks behind, want 0", len(checks))
	}
}

// An entity id from a different work must be refused, proving the ref
// lookup is scoped per-project rather than checking existence globally.
func TestReportContinuity_refusesAnEntityFromAnotherWork(t *testing.T) {
	ctx, f := newContinuityFixture(t)
	version := f.setText(t, ctx, "하린은 오른손으로 검을 뽑았다.")

	other, err := f.d.Projects.Create(ctx, 1, project.NewInput{Title: "다른 작품", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("create other project: %v", err)
	}
	otherEntity, err := f.d.Entities.Create(ctx, 1, entity.NewInput{ProjectID: other.ID, Kind: "character", Name: "다른 인물"})
	if err != nil {
		t.Fatalf("create other entity: %v", err)
	}

	obs := continuity.ObservationInput{
		Category: continuity.CategoryCharacter, Kind: continuity.KindContradiction, Severity: continuity.SeverityMedium,
		Summary: "모순", ParagraphFrom: 1, ParagraphTo: 1, Quote: "하린은 오른손으로 검을 뽑았다.",
		Refs: []continuity.Ref{{Type: continuity.RefTypeEntity, ID: otherEntity.ID}},
	}
	res, _, err := f.d.reportContinuity(ctx, nil, reportContinuityInput{
		NodeID: f.node, SceneVersion: version, Observations: []continuity.ObservationInput{obs},
	})
	if err != nil {
		t.Fatalf("reportContinuity: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("an entity ref from another work must be refused")
	}
}

func TestReportContinuity_refusesANonLeafNode(t *testing.T) {
	ctx, f := newContinuityFixture(t)
	container, err := f.d.Nodes.CreateSibling(ctx, f.node, node.KindContainer, "1장", "", 1000)
	if err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	res, _, err := f.d.reportContinuity(ctx, nil, reportContinuityInput{
		NodeID: container.ID, SceneVersion: 0, Observations: nil,
	})
	if err != nil {
		t.Fatalf("reportContinuity: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("reporting on a container node must be refused")
	}
}

func TestReportContinuity_unavailableWithNilRepo(t *testing.T) {
	ctx, f := newContinuityFixture(t)
	f.d.Continuity = nil
	res, _, err := f.d.reportContinuity(ctx, nil, reportContinuityInput{NodeID: f.node, SceneVersion: 0})
	if err != nil {
		t.Fatalf("reportContinuity: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("a nil Continuity repo must refuse rather than panic")
	}
}
