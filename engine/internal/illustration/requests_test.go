package illustration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/illustref"
	"github.com/devlikebear/linetta/engine/internal/node"
)

// fakeGenerator stores a real picture in the gallery for every drawing, or
// fails with err.
type fakeGenerator struct {
	t    *testing.T
	repo *Repo
	mu   sync.Mutex
	got  []GenerateInput
	err  error
}

func (g *fakeGenerator) Generate(ctx context.Context, in GenerateInput) (Meta, error) {
	g.mu.Lock()
	g.got = append(g.got, in)
	err := g.err
	g.mu.Unlock()
	if err != nil {
		return Meta{}, err
	}
	return g.repo.Add(ctx, 10, NewInput{ProjectID: in.ProjectID, SourceNodeID: in.SourceNodeID, Caption: in.Caption,
		Prompt: in.Prompt, Provider: "fake", Model: "m", Data: encodeJPEG(g.t, 4, 4)})
}

type requestFixture struct {
	fixtureT
	svc    *RequestService
	gen    *fakeGenerator
	nodes  *node.Repo
	events []ChangedEvent
}

func newRequestFixture(t *testing.T) *requestFixture {
	t.Helper()
	f := &requestFixture{fixtureT: fixture(t), nodes: nil}
	f.nodes = node.NewRepo(f.s)
	f.gen = &fakeGenerator{t: t, repo: f.repo}
	clock := int64(100)
	f.svc = NewRequestService(f.repo, f.gen, f.nodes, func() int64 { clock++; return clock },
		func(method string, params any) {
			if method != "illustrations.changed" {
				t.Fatalf("method = %s", method)
			}
			f.events = append(f.events, params.(ChangedEvent))
		})
	return f
}

func (f *requestFixture) submit(t *testing.T, in SubmitInput) Request {
	t.Helper()
	if in.ProjectID == "" {
		in.ProjectID = f.project
	}
	if in.Source == "" {
		in.Source = SourceAgent
	}
	if in.Prompt == "" {
		in.Prompt = "비 오는 골목, 노란 우산"
	}
	r, err := f.svc.Submit(f.ctx, in)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return r
}

func (f *requestFixture) statuses() []string {
	var out []string
	for _, e := range f.events {
		out = append(out, e.Status)
	}
	return out
}

func TestRequest_submitDrawsNothing(t *testing.T) {
	f := newRequestFixture(t)
	r := f.submit(t, SubmitInput{TargetNodeID: f.scene, Purpose: PurposeInsert, Caption: " 우산 ", Note: "씬 1의 첫 장면", Aspect: AspectPortrait})
	if r.Status != RequestPending || r.Caption != "우산" || r.Aspect != AspectPortrait || r.Source != SourceAgent {
		t.Fatalf("request = %+v", r)
	}
	if len(f.gen.got) != 0 {
		t.Fatal("a request drew something")
	}
	list, err := f.svc.List(f.ctx, f.project, []string{RequestPending})
	if err != nil || len(list) != 1 || list[0].ID != r.ID || len(list[0].CharacterIDs) != 0 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if strings.Join(f.statuses(), ",") != "pending" {
		t.Fatalf("events = %v", f.statuses())
	}
}

func TestRequest_approveDrawsAndInsertsAtTheSceneEnd(t *testing.T) {
	f := newRequestFixture(t)
	if err := f.nodes.UpdateContent(f.ctx, f.scene, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"비가 내렸다."}]}]}`, 5); err != nil {
		t.Fatal(err)
	}
	r := f.submit(t, SubmitInput{TargetNodeID: f.scene, Purpose: PurposeInsert, Caption: "우산"})

	prompt, aspect := "고친 프롬프트", AspectLandscape
	done, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{Prompt: &prompt, Aspect: &aspect})
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != RequestDone || done.IllustrationID == "" || done.PlacedAt == nil || done.Error != "" {
		t.Fatalf("done = %+v", done)
	}
	if got := f.gen.got[0]; got.Prompt != prompt || got.Aspect != aspect || got.SourceNodeID != f.scene || got.Caption != "우산" {
		t.Fatalf("drew with %+v", got)
	}
	n, err := f.nodes.Get(f.ctx, f.scene)
	if err != nil {
		t.Fatal(err)
	}
	ids, _ := illustref.IDs(*n.ContentDoc)
	if len(ids) != 1 || ids[0] != done.IllustrationID || !strings.HasPrefix(*n.ContentDoc, `{"content":[{"content":[{"text":"비가 내렸다."`) {
		t.Fatalf("scene = %s", *n.ContentDoc)
	}
	if strings.Join(f.statuses(), ",") != "pending,generating,done" {
		t.Fatalf("events = %v", f.statuses())
	}
	if last := f.events[len(f.events)-1]; len(last.NodeIDs) != 1 || last.NodeIDs[0] != f.scene {
		t.Fatalf("done event = %+v", last)
	}
	again, err := f.svc.Get(f.ctx, r.ID)
	if err != nil || again.Prompt != prompt || again.Aspect != aspect {
		t.Fatalf("stored = %+v, %v", again, err)
	}
}

func TestRequest_approveIntoAnEmptyScene(t *testing.T) {
	f := newRequestFixture(t)
	r := f.submit(t, SubmitInput{TargetNodeID: f.scene, Purpose: PurposeInsert})
	if _, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{}); err != nil {
		t.Fatal(err)
	}
	n, _ := f.nodes.Get(f.ctx, f.scene)
	if ids, _ := illustref.IDs(*n.ContentDoc); len(ids) != 1 {
		t.Fatalf("scene = %v", n.ContentDoc)
	}
}

func TestRequest_galleryPurposeLeavesTheSceneAlone(t *testing.T) {
	f := newRequestFixture(t)
	r := f.submit(t, SubmitInput{Purpose: PurposeGallery})
	done, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{})
	if err != nil || done.Status != RequestDone || done.PlacedAt == nil {
		t.Fatalf("done = %+v, %v", done, err)
	}
	if n, _ := f.nodes.Get(f.ctx, f.scene); n.ContentDoc != nil && strings.Contains(*n.ContentDoc, "illustration") {
		t.Fatal("a gallery picture went into the scene")
	}
	if meta, _ := f.repo.Get(f.ctx, done.IllustrationID); meta.SourceNodeID != nil {
		t.Fatal("a request without a scene recorded one")
	}
}

func TestRequest_approvePassesTheCardsEarlierPictures(t *testing.T) {
	f := newRequestFixture(t)
	r := f.submit(t, SubmitInput{Purpose: PurposeGallery})
	if _, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{PriorIllustrationIDs: []string{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	if got := f.gen.got[0].PriorIllustrationIDs; strings.Join(got, ",") != "a,b" {
		t.Fatalf("prior = %v", got)
	}
}

func TestRequest_earlierPicturesAreSuggestedKeptAndSentByDefault(t *testing.T) {
	f := newRequestFixture(t)
	kept, err := f.repo.Add(f.ctx, 1, NewInput{ProjectID: f.project, Prompt: "p", Provider: "x", Model: "m", Data: encodeJPEG(t, 4, 4)})
	if err != nil {
		t.Fatal(err)
	}
	r := f.submit(t, SubmitInput{Purpose: PurposeGallery})
	if len(r.PriorIllustrationIDs) != 1 || r.PriorIllustrationIDs[0] != kept.ID {
		t.Fatalf("suggested = %v", r.PriorIllustrationIDs)
	}
	none := f.submit(t, SubmitInput{Purpose: PurposeGallery, PriorIllustrationIDs: []string{}})
	if len(none.PriorIllustrationIDs) != 0 {
		t.Fatalf("explicit none = %v", none.PriorIllustrationIDs)
	}
	if _, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{}); err != nil {
		t.Fatal(err)
	}
	if got := f.gen.got[0].PriorIllustrationIDs; len(got) != 1 || got[0] != kept.ID {
		t.Fatalf("sent = %v, want the request's own", got)
	}
	if _, err := f.svc.Submit(f.ctx, SubmitInput{ProjectID: f.project, Purpose: PurposeGallery, Prompt: "p", Source: SourceAgent,
		PriorIllustrationIDs: []string{"elsewhere"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown picture = %v", err)
	}
}

func TestRequest_approvingTwiceDrawsOnce(t *testing.T) {
	f := newRequestFixture(t)
	r := f.submit(t, SubmitInput{TargetNodeID: f.scene, Purpose: PurposeInsert})
	if _, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{}); !errors.Is(err, ErrNotPending) {
		t.Fatalf("second approve = %v", err)
	}
	if len(f.gen.got) != 1 {
		t.Fatalf("drew %d times", len(f.gen.got))
	}
}

func TestRequest_failedDrawingCanBeRetried(t *testing.T) {
	f := newRequestFixture(t)
	r := f.submit(t, SubmitInput{TargetNodeID: f.scene, Purpose: PurposeInsert})
	f.gen.err = errors.New("provider refused")
	failed, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{})
	if err == nil || failed.Status != RequestFailed || failed.Error != "provider refused" || failed.IllustrationID != "" {
		t.Fatalf("failed = %+v, %v", failed, err)
	}
	f.gen.err = nil
	done, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{})
	if err != nil || done.Status != RequestDone || done.Error != "" {
		t.Fatalf("retry = %+v, %v", done, err)
	}
}

func TestRequest_reject(t *testing.T) {
	f := newRequestFixture(t)
	r := f.submit(t, SubmitInput{Purpose: PurposeGallery})
	rejected, err := f.svc.Reject(f.ctx, r.ID)
	if err != nil || rejected.Status != RequestRejected || rejected.DecidedAt == nil {
		t.Fatalf("rejected = %+v, %v", rejected, err)
	}
	if _, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{}); !errors.Is(err, ErrNotPending) {
		t.Fatalf("approve after reject = %v", err)
	}
	if _, err := f.svc.Reject(f.ctx, r.ID); !errors.Is(err, ErrNotPending) {
		t.Fatalf("reject twice = %v", err)
	}
	if _, err := f.svc.Reject(f.ctx, "nope"); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("reject missing = %v", err)
	}
}

func TestRequest_submitChecksBeforeStoring(t *testing.T) {
	f := newRequestFixture(t)
	part, err := f.nodes.CreateRoot(f.ctx, f.project, node.KindContainer, "1화", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]SubmitInput{
		"empty prompt":                  {Purpose: PurposeGallery, Prompt: " "},
		"insert without scene":          {Purpose: PurposeInsert},
		"insert into episode":           {Purpose: PurposeInsert, TargetNodeID: part.ID},
		"representative without a node": {Purpose: PurposeRepresentative},
		"unknown purpose":               {Purpose: "cover"},
		"bad aspect":                    {Purpose: PurposeGallery, Aspect: "wide"},
		"bad source":                    {Purpose: PurposeGallery, Source: "writer"},
		"unknown character":             {Purpose: PurposeGallery, CharacterIDs: []string{"ghost"}},
	}
	for name, in := range cases {
		in.ProjectID = f.project
		if in.Source == "" {
			in.Source = SourceAgent
		}
		if in.Prompt == "" {
			in.Prompt = "p"
		} else if strings.TrimSpace(in.Prompt) == "" {
			in.Prompt = " "
		}
		if _, err := f.svc.Submit(f.ctx, in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	other := fixture(t)
	if _, err := f.svc.Submit(f.ctx, SubmitInput{ProjectID: f.project, TargetNodeID: other.scene, Purpose: PurposeInsert, Prompt: "p", Source: SourceAgent}); !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("another work's scene: err = %v", err)
	}
	if list, _ := f.svc.List(f.ctx, f.project, nil); len(list) != 0 {
		t.Fatalf("stored %d bad requests", len(list))
	}
}

func TestRequest_waitingRequestsAreBounded(t *testing.T) {
	f := newRequestFixture(t)
	for i := 0; i < MaxPendingRequests; i++ {
		f.submit(t, SubmitInput{Purpose: PurposeGallery})
	}
	if _, err := f.svc.Submit(f.ctx, SubmitInput{ProjectID: f.project, Purpose: PurposeGallery, Prompt: "p", Source: SourceExternal}); !errors.Is(err, ErrTooManyPending) {
		t.Fatalf("err = %v", err)
	}
	// A decided request frees its place.
	list, _ := f.svc.List(f.ctx, f.project, nil)
	if _, err := f.svc.Reject(f.ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	f.submit(t, SubmitInput{Purpose: PurposeGallery})
}

func TestRequest_deletedSceneCancelsItsRequests(t *testing.T) {
	f := newRequestFixture(t)
	other, err := f.nodes.CreateRoot(f.ctx, f.project, node.KindLeaf, "씬 2", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	r := f.submit(t, SubmitInput{TargetNodeID: other.ID, Purpose: PurposeInsert})
	if err := f.nodes.Delete(f.ctx, other.ID, 4); err != nil {
		t.Fatal(err)
	}
	list, err := f.svc.List(f.ctx, f.project, nil)
	if err != nil || len(list) != 1 || list[0].Status != RequestCancelled || list[0].Error == "" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if _, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{}); !errors.Is(err, ErrNotPending) {
		t.Fatalf("approve = %v", err)
	}
	if len(f.gen.got) != 0 {
		t.Fatal("drew for a deleted scene")
	}
}

func TestRequest_recoverInterrupted(t *testing.T) {
	f := newRequestFixture(t)
	r := f.submit(t, SubmitInput{Purpose: PurposeGallery})
	if _, err := f.s.DB().Exec(`UPDATE illustration_requests SET status = 'generating' WHERE id = ?`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.RecoverInterrupted(f.ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.Get(f.ctx, r.ID)
	if got.Status != RequestFailed || got.Error == "" {
		t.Fatalf("got = %+v", got)
	}
}

func TestRequest_representativePurposeSetsTheNodesPicture(t *testing.T) {
	f := newRequestFixture(t)
	part, err := f.nodes.CreateRoot(f.ctx, f.project, node.KindContainer, "1화", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	r := f.submit(t, SubmitInput{TargetNodeID: part.ID, Purpose: PurposeRepresentative, Aspect: AspectLandscape})
	done, err := f.svc.Approve(f.ctx, r.ID, ApproveEdits{})
	if err != nil || done.PlacedAt == nil {
		t.Fatalf("done = %+v, %v", done, err)
	}
	reps, err := f.repo.Representatives(f.ctx, f.project)
	if err != nil || reps[part.ID] != done.IllustrationID {
		t.Fatalf("representatives = %v, %v", reps, err)
	}
	if n, _ := f.nodes.Get(f.ctx, f.scene); n.ContentDoc != nil && strings.Contains(*n.ContentDoc, "illustration") {
		t.Fatal("a representative picture went into a scene")
	}
}
