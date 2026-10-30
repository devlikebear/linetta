package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// fakeSheets is the writer's sheets: real entity rows (the choice table
// points at them) with relationships and portraits kept here.
type fakeSheets struct {
	repo      *entity.Repo
	projectID string
	relations []LocalRelation
	portraits map[string][]byte
}

func (s *fakeSheets) ProjectSheets(ctx context.Context, projectID string) ([]LocalSheet, []LocalRelation, error) {
	list, err := s.repo.ListByProject(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	out := []LocalSheet{}
	for i := len(list) - 1; i >= 0; i-- { // ListByProject is newest first
		e := list[i]
		out = append(out, LocalSheet{ID: e.ID, Kind: e.Kind, Name: e.Name, Aliases: e.Aliases, Role: e.Role, Summary: e.Summary, Attributes: e.Attributes})
	}
	return out, s.relations, nil
}

func (s *fakeSheets) Portrait(_ context.Context, entityID string) (string, []byte, bool, error) {
	data, ok := s.portraits[entityID]
	return "image/png", data, ok, nil
}

func (s *fakeSheets) SheetProject(ctx context.Context, entityID string) (string, error) {
	e, err := s.repo.Get(ctx, entityID)
	if errors.Is(err, entity.ErrNotFound) {
		return "", ErrSheetNotFound
	}
	return e.ProjectID, err
}

type sheetsFixture struct {
	*serviceFixture
	sheets  *fakeSheets
	gallery fakeGallery
	hero    string
	town    string
	secret  string
}

func newSheetsFixture(t *testing.T) *sheetsFixture {
	t.Helper()
	ctx := context.Background()
	f := newServiceFixture(t, 1)
	repo := entity.NewRepo(f.store)
	mk := func(kind, name, summary string, attrs map[string]string, now int64) string {
		e, err := repo.Create(ctx, now, entity.NewInput{ProjectID: f.project.ID, Kind: kind, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Update(ctx, now, entity.UpdateInput{ID: e.ID, Summary: &summary, Attributes: &attrs}); err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	sf := &sheetsFixture{serviceFixture: f}
	sf.hero = mk(entity.KindCharacter, "강도윤", "기억을 잃은 신", map[string]string{"약점": "노래방", "출신": "천계"}, 1)
	sf.town = mk(entity.KindPlace, "회사", "", nil, 2)
	sf.secret = mk(entity.KindConcept, "진짜 정체", "스포일러", nil, 3)
	sf.sheets = &fakeSheets{
		repo: repo, projectID: f.project.ID,
		relations: []LocalRelation{{From: sf.hero, To: sf.town, Label: "직장"}, {From: sf.hero, To: sf.secret, Label: "숨긴 것"}},
		portraits: map[string][]byte{sf.hero: []byte("PNG-HERO")},
	}
	sf.gallery = fakeGallery{"cover-a": []byte("PNG-COVER-A"), "cover-b": []byte("PNG-COVER-B")}
	f.svc.WithIllustrations(sf.gallery).WithSheets(sf.sheets)
	return sf
}

func (f *sheetsFixture) choose(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := f.svc.SetSheetPublished(context.Background(), id, true); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *sheetsFixture) sync(t *testing.T) SheetsStatus {
	t.Helper()
	out, err := f.svc.SyncSheets(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func countPrefix(requests []string, prefix string) int {
	n := 0
	for _, r := range requests {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func TestSheets_onlyTheChosenSheetsGoOut(t *testing.T) {
	f := newSheetsFixture(t)
	f.register(t)
	f.choose(t, f.hero, f.town)

	out := f.sync(t)
	if out.Chosen != 2 || !out.Remote || len(out.Sheets) != 2 || out.Sheets[0].State != EpisodePending {
		t.Fatalf("status = %+v", out)
	}
	work := f.site.work(f.project.ID)
	if len(work.sheets) != 2 || work.sheets[0].Name != "강도윤" || work.sheets[1].Name != "회사" {
		t.Fatalf("sent sheets = %+v", work.sheets)
	}
	hero := work.sheets[0]
	// Attributes go by key; the portrait is named by hash.
	if len(hero.Attributes) != 2 || hero.Attributes[0].Key != "약점" || hero.Attributes[1].Key != "출신" {
		t.Fatalf("attributes = %+v", hero.Attributes)
	}
	sum := sha256.Sum256([]byte("PNG-HERO"))
	if hero.PortraitSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("portrait = %q", hero.PortraitSHA256)
	}
	// The relationship to the secret sheet stays here: one end is not chosen.
	if len(work.relations) != 1 || work.relations[0].Label != "직장" {
		t.Fatalf("relations = %+v", work.relations)
	}
	for _, s := range work.sheets {
		if s.EntityID == f.secret {
			t.Fatal("a sheet the writer did not turn on was sent")
		}
	}
}

func TestSheets_portraitsAreSentOnceAndTurningOffRemoves(t *testing.T) {
	f := newSheetsFixture(t)
	f.register(t)
	f.choose(t, f.hero, f.town)
	f.sync(t)
	before := len(f.site.seen())
	f.sync(t)
	if n := countPrefix(f.site.seen()[before:], "PUT /api/artworks/works/"+f.project.ID+"/entities/"); n != 0 {
		t.Fatalf("portrait re-uploaded %d times", n)
	}

	if err := f.svc.SetSheetPublished(context.Background(), f.hero, false); err != nil {
		t.Fatal(err)
	}
	out := f.sync(t)
	if out.Chosen != 1 || len(f.site.work(f.project.ID).sheets) != 1 || len(f.site.work(f.project.ID).relations) != 0 {
		t.Fatalf("after turning off: status %+v, sent %+v", out, f.site.work(f.project.ID).sheets)
	}
}

func TestSheets_coverIsSentReplacedAndTakenDown(t *testing.T) {
	f := newSheetsFixture(t)
	f.register(t)
	ctx := context.Background()
	if err := f.svc.SetCover(ctx, f.project.ID, "cover-a"); err != nil {
		t.Fatal(err)
	}
	out := f.sync(t)
	if out.CoverID != "cover-a" || out.Cover == nil || out.Cover.IllustrationID != "cover-a" || out.Cover.State != EpisodePending {
		t.Fatalf("status = %+v", out)
	}
	before := len(f.site.seen())
	f.sync(t)
	if n := countPrefix(f.site.seen()[before:], "PUT /api/artworks/works/"+f.project.ID+"/cover"); n != 0 {
		t.Fatalf("same cover re-uploaded %d times", n)
	}

	if err := f.svc.SetCover(ctx, f.project.ID, "cover-b"); err != nil {
		t.Fatal(err)
	}
	if out := f.sync(t); out.Cover == nil || out.Cover.IllustrationID != "cover-b" {
		t.Fatalf("replaced cover = %+v", out.Cover)
	}
	if err := f.svc.SetCover(ctx, f.project.ID, ""); err != nil {
		t.Fatal(err)
	}
	if out := f.sync(t); out.Cover != nil || f.site.work(f.project.ID).work.Cover != nil {
		t.Fatalf("cover not taken down: %+v", out.Cover)
	}

	err := f.svc.SetCover(ctx, f.project.ID, "not-in-gallery")
	var me *rpc.MethodError
	if !errors.As(err, &me) {
		t.Fatalf("unknown picture = %v, want not found", err)
	}
}

func TestPublishCover_uploadsAndChoosesTheCoverAndLeavesSheetsAlone(t *testing.T) {
	f := newSheetsFixture(t)
	f.register(t)
	f.choose(t, f.hero)
	ctx := context.Background()

	cover, err := f.svc.PublishCover(ctx, f.project.ID, "cover-a")
	if err != nil {
		t.Fatal(err)
	}
	if cover.IllustrationID != "cover-a" || cover.State != EpisodePending {
		t.Fatalf("cover = %+v", cover)
	}
	if got, _, err := f.svc.state.Cover(ctx, f.project.ID); err != nil || got != "cover-a" {
		t.Fatalf("saved choice = %q, %v", got, err)
	}
	work := f.site.work(f.project.ID)
	if work.work.Cover == nil || work.work.Cover.IllustrationID != "cover-a" {
		t.Fatalf("site cover = %+v", work.work.Cover)
	}
	if len(work.sheets) != 0 || countPrefix(f.site.seen(), "PUT /api/artworks/works/"+f.project.ID+"/sheets") != 0 {
		t.Fatalf("a cover upload sent sheets: %+v", work.sheets)
	}

	before := len(f.site.seen())
	if _, err := f.svc.PublishCover(ctx, f.project.ID, "cover-a"); err != nil {
		t.Fatal(err)
	}
	if n := countPrefix(f.site.seen()[before:], "PUT /api/artworks/works/"+f.project.ID+"/cover"); n != 0 {
		t.Fatalf("same cover re-uploaded %d times", n)
	}

	if cover, err = f.svc.PublishCover(ctx, f.project.ID, "cover-b"); err != nil || cover.IllustrationID != "cover-b" {
		t.Fatalf("replaced cover = %+v, %v", cover, err)
	}
}

func TestPublishCover_refusalsKeepTheSiteAndTheChoice(t *testing.T) {
	f := newSheetsFixture(t)
	f.register(t)
	ctx := context.Background()
	if _, err := f.svc.PublishCover(ctx, f.project.ID, "cover-a"); err != nil {
		t.Fatal(err)
	}
	f.gallery["too-big"] = make([]byte, maxPictureBytes+1)

	for _, id := range []string{"not-in-gallery", "too-big"} {
		if _, err := f.svc.PublishCover(ctx, f.project.ID, id); err == nil {
			t.Fatalf("%s: want a refusal", id)
		}
		if held := f.site.work(f.project.ID).work.Cover; held == nil || held.IllustrationID != "cover-a" {
			t.Fatalf("%s: site cover = %+v", id, held)
		}
		if got, _, err := f.svc.state.Cover(ctx, f.project.ID); err != nil || got != "cover-a" {
			t.Fatalf("%s: saved choice = %q, %v", id, got, err)
		}
	}

	f.site.failNext("PUT /api/artworks/works/"+f.project.ID+"/cover", 413, CodeTooLarge)
	if _, err := f.svc.PublishCover(ctx, f.project.ID, "cover-b"); err == nil {
		t.Fatal("want the site's failure")
	}
	if got, _, err := f.svc.state.Cover(ctx, f.project.ID); err != nil || got != "cover-a" {
		t.Fatalf("failed upload changed the choice to %q, %v", got, err)
	}
}

func TestPublishCover_refusesAWorkNotOnTheSite(t *testing.T) {
	f := newSheetsFixture(t)
	f.connect(t)
	_, err := f.svc.PublishCover(context.Background(), f.project.ID, "cover-a")
	var re *rpc.ReasonError
	if !errors.As(err, &re) || re.Reason != rpc.ReasonPublishWorkNotRegistered {
		t.Fatalf("err = %v", err)
	}
}

func TestSheets_workStatusShowsChoicesAndTheSite(t *testing.T) {
	f := newSheetsFixture(t)
	ctx := context.Background()
	f.choose(t, f.town)
	st, err := f.svc.WorkStatus(ctx, f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Sheets.Chosen != 1 || st.Sheets.Remote {
		t.Fatalf("unregistered status = %+v", st.Sheets)
	}
	f.register(t)
	f.sync(t)
	f.site.work(f.project.ID).sheetState = map[string]string{f.town: "rejected"}
	st, err = f.svc.WorkStatus(ctx, f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Sheets.Remote || len(st.Sheets.Sheets) != 1 || st.Sheets.Sheets[0].State != EpisodeRejected {
		t.Fatalf("registered status = %+v", st.Sheets)
	}
}

func TestSheets_needsARegisteredWorkAndAKnownSheet(t *testing.T) {
	f := newSheetsFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SyncSheets(ctx, f.project.ID); reasonOf(err) != rpc.ReasonPublishWorkNotRegistered {
		t.Fatalf("unregistered sync = %v", err)
	}
	if err := f.svc.SetSheetPublished(ctx, "no-such-sheet", true); err == nil {
		t.Fatal("unknown sheet accepted")
	}
	on, err := f.svc.SheetPublished(ctx, f.hero)
	if err != nil || on {
		t.Fatalf("SheetPublished = %v, %v", on, err)
	}
}

func TestSheets_longTextIsTrimmedToTheSiteLimits(t *testing.T) {
	in := sheetInput(LocalSheet{
		ID: "e", Kind: "character", Name: strings.Repeat("가", 120), Summary: strings.Repeat("나", 6000),
		Aliases: []string{" ", "별칭"}, Attributes: map[string]string{"빈 값": " ", "키": "값"},
	})
	if len([]rune(in.Name)) != maxSheetNameRunes || len([]rune(in.Summary)) != maxSummaryRunes {
		t.Fatalf("name %d, summary %d", len([]rune(in.Name)), len([]rune(in.Summary)))
	}
	if len(in.Aliases) != 1 || len(in.Attributes) != 1 || in.Attributes[0].Key != "키" {
		t.Fatalf("aliases %v, attributes %v", in.Aliases, in.Attributes)
	}
}
