package publish

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type serviceFixture struct {
	site     *fakeSite
	svc      *Service
	settings *settings.Store
	state    *StateRepo
	nodes    *node.Repo
	project  project.Project
	store    *store.Store
	arcID    string
	// episodes are the leaf episodes 1화, 2화 … under 1권, in order.
	episodes []string
}

func doc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}

// newServiceFixture makes a web-novel work with n leaf episodes, each with
// text, and a device connected to a fake site.
func newServiceFixture(t *testing.T, n int) *serviceFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	projects, nodes := project.NewRepo(st), node.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{
		Title: "신의 K-직장 생존기", Genres: []string{"현대판타지"}, LengthTarget: "series", DefaultPOV: "first",
		OutlinePreset: project.OutlinePresetWebNovel,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := nodes.Get(ctx, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatal(err)
	}
	f := &serviceFixture{site: newFakeSite(t), nodes: nodes, project: p, store: st, arcID: *first.ParentID}
	f.episodes = []string{first.ID}
	for i := 1; i < n; i++ {
		prev := f.episodes[len(f.episodes)-1]
		ep, err := nodes.CreateSibling(ctx, prev, node.KindLeaf, strconv.Itoa(i+1)+"화", "", int64(10+i))
		if err != nil {
			t.Fatal(err)
		}
		f.episodes = append(f.episodes, ep.ID)
	}
	for i, id := range f.episodes {
		if err := nodes.UpdateContent(ctx, id, doc("본문 "+strconv.Itoa(i+1)), int64(100+i)); err != nil {
			t.Fatal(err)
		}
	}
	f.settings = newTestSettings(t)
	f.connect(t)
	creds := NewCredentials(f.settings)
	f.state = NewStateRepo(st.DB())
	client := f.site.client()
	connector := NewConnector(client, creds, f.state.Clear)
	clock := int64(1000)
	f.svc = NewService(client, creds, connector, f.state, projects, nodes,
		func(context.Context) string { return "ko" }, func() int64 { clock++; return clock })
	return f
}

func (f *serviceFixture) connect(t *testing.T) {
	t.Helper()
	if err := NewCredentials(f.settings).Save(settings.PublishAccount{SiteURL: f.site.srv.URL, Handle: "devlikebear", PenName: "마곰"}, fakeToken); err != nil {
		t.Fatal(err)
	}
}

func (f *serviceFixture) register(t *testing.T) {
	t.Helper()
	if _, err := f.svc.UpsertWork(context.Background(), UpsertInput{ProjectID: f.project.ID, Blurb: "소개글", SerialStatus: SerialOngoing}); err != nil {
		t.Fatal(err)
	}
}

func (f *serviceFixture) publish(t *testing.T, nodeID string) PublishResult {
	t.Helper()
	res, err := f.svc.PublishEpisode(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("publish %s: %v", nodeID, err)
	}
	return res
}

func remoteOrder(eps []RemoteEpisode) []string {
	out := []string{}
	for _, e := range eps {
		out = append(out, e.NodeID)
	}
	return out
}

func TestService_notConnectedSendsNothing(t *testing.T) {
	f := newServiceFixture(t, 2)
	if err := NewCredentials(f.settings).Clear(); err != nil {
		t.Fatal(err)
	}
	ws, err := f.svc.WorkStatus(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Connected || len(ws.Episodes) != 2 || ws.Episodes[0].State != EpisodeUnpublished {
		t.Fatalf("status = %+v", ws)
	}
	if _, err := f.svc.PublishEpisode(context.Background(), f.episodes[0]); reasonOf(err) != rpc.ReasonPublishNotConnected {
		t.Fatalf("publish err = %v", err)
	}
	if got := f.site.seen(); len(got) != 0 {
		t.Fatalf("a disconnected app sent requests: %v", got)
	}
}

func TestService_publishRequiresARegisteredWork(t *testing.T) {
	f := newServiceFixture(t, 1)
	_, err := f.svc.PublishEpisode(context.Background(), f.episodes[0])
	if reasonOf(err) != rpc.ReasonPublishWorkNotRegistered {
		t.Fatalf("err = %v", err)
	}
	if n, _ := f.nodes.Get(context.Background(), f.episodes[0]); n.Status == node.StatusPublished {
		t.Fatal("a refused publish marked the node published")
	}
}

func TestService_upsertSendsProjectTitleGenresAndLanguage(t *testing.T) {
	f := newServiceFixture(t, 1)
	if _, err := f.svc.UpsertWork(context.Background(), UpsertInput{ProjectID: f.project.ID, Blurb: "  "}); reasonOf(err) != rpc.ReasonPublishBlurbRequired {
		t.Fatalf("empty blurb: %v", err)
	}
	w, err := f.svc.UpsertWork(context.Background(), UpsertInput{ProjectID: f.project.ID, Blurb: "소개글", SerialStatus: SerialCompleted, Slug: "k-office-god"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Slug != "k-office-god" || w.SerialStatus != SerialCompleted || w.Blurb != "소개글" {
		t.Fatalf("work = %+v", w)
	}
	site := f.site.works[f.project.ID].work
	if site.Title != "신의 K-직장 생존기" || !slices.Equal(site.Genres, []string{"현대판타지"}) || site.Language != "ko" {
		t.Fatalf("site work = %+v", site)
	}
	saved, ok, _ := f.state.Work(context.Background(), f.project.ID)
	if !ok || saved.Slug != "k-office-god" {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestService_firstPublishThenModifiedThenRepublishKeepsTheAddress(t *testing.T) {
	f := newServiceFixture(t, 2)
	ctx := context.Background()
	f.register(t)

	first := f.publish(t, f.episodes[0])
	if !first.Created || first.Episode.Number != 1 || first.Episode.State != EpisodePublic || first.Episode.URL == "" {
		t.Fatalf("first publish = %+v", first)
	}
	n, _ := f.nodes.Get(ctx, f.episodes[0])
	if n.Status != node.StatusPublished {
		t.Fatalf("node status = %q, want published", n.Status)
	}

	ws, err := f.svc.WorkStatus(ctx, f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Episodes[0].State != EpisodePublic || ws.Episodes[1].State != EpisodeUnpublished {
		t.Fatalf("status = %+v", ws.Episodes)
	}

	if err := f.nodes.UpdateContent(ctx, f.episodes[0], doc("고친 본문"), 500); err != nil {
		t.Fatal(err)
	}
	ws, _ = f.svc.WorkStatus(ctx, f.project.ID)
	if ws.Episodes[0].State != EpisodeModified || !ws.Episodes[0].Modified {
		t.Fatalf("after edit: %+v", ws.Episodes[0])
	}

	again := f.publish(t, f.episodes[0])
	if again.Created || again.Episode.URL != first.Episode.URL || again.Episode.State != EpisodePublic {
		t.Fatalf("republish = %+v (first %s)", again, first.Episode.URL)
	}
	if got := f.site.episodes(f.project.ID); len(got) != 1 {
		t.Fatalf("site has %d episodes, want 1", len(got))
	}
}

func TestService_publishingAContainerSceneSendsTheWholeEpisode(t *testing.T) {
	f := newServiceFixture(t, 1)
	ctx := context.Background()
	f.register(t)
	ep, err := f.nodes.CreateSibling(ctx, f.episodes[0], node.KindContainer, "2화", "첫 출근", 300)
	if err != nil {
		t.Fatal(err)
	}
	var scenes []string
	for i := 0; i < 2; i++ {
		s, err := f.nodes.CreateChild(ctx, ep.ID, node.KindLeaf, "씬", "", int64(301+i))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.nodes.UpdateContent(ctx, s.ID, doc("장면 "+strconv.Itoa(i+1)), int64(310+i)); err != nil {
			t.Fatal(err)
		}
		scenes = append(scenes, s.ID)
	}
	res := f.publish(t, scenes[1])
	if res.Episode.NodeID != ep.ID || res.Episode.Title != "첫 출근" || res.Episode.Number != 1 {
		t.Fatalf("result = %+v", res.Episode)
	}
	for _, id := range append([]string{ep.ID}, scenes...) {
		if n, _ := f.nodes.Get(ctx, id); n.Status != node.StatusPublished {
			t.Errorf("node %s status = %q", id, n.Status)
		}
	}
}

func TestService_insertingBeforePublishedEpisodesRenumbers(t *testing.T) {
	f := newServiceFixture(t, 3)
	f.register(t)
	// Publish 1화 and 3화 first: they become 1 and 2 on the site.
	f.publish(t, f.episodes[0])
	f.publish(t, f.episodes[2])
	if got := remoteOrder(f.site.episodes(f.project.ID)); !slices.Equal(got, []string{f.episodes[0], f.episodes[2]}) {
		t.Fatalf("site order = %v", got)
	}
	// Now 2화, which sits between them in the outline.
	res := f.publish(t, f.episodes[1])
	if res.Episode.Number != 2 {
		t.Fatalf("2화 got number %d", res.Episode.Number)
	}
	got := f.site.episodes(f.project.ID)
	if !slices.Equal(remoteOrder(got), f.episodes) || got[2].Number != 3 {
		t.Fatalf("site = %+v", got)
	}
}

func TestService_movingAnEpisodeReordersBeforeRepublishing(t *testing.T) {
	f := newServiceFixture(t, 2)
	ctx := context.Background()
	f.register(t)
	f.publish(t, f.episodes[0])
	f.publish(t, f.episodes[1])
	// Move 2화 before 1화 in the outline.
	if err := f.nodes.MoveUp(ctx, f.episodes[1], 900); err != nil {
		t.Fatal(err)
	}
	res := f.publish(t, f.episodes[1])
	if res.Episode.Number != 1 {
		t.Fatalf("moved episode number = %d", res.Episode.Number)
	}
	if got := remoteOrder(f.site.episodes(f.project.ID)); !slices.Equal(got, []string{f.episodes[1], f.episodes[0]}) {
		t.Fatalf("site order = %v", got)
	}
}

func TestService_siteOnlyEpisodesKeepTheirPlaceAndAreListed(t *testing.T) {
	f := newServiceFixture(t, 3)
	ctx := context.Background()
	f.register(t)
	f.publish(t, f.episodes[0])
	f.publish(t, f.episodes[1])
	if err := f.nodes.Delete(ctx, f.episodes[1], 900); err != nil {
		t.Fatal(err)
	}
	f.publish(t, f.episodes[2])
	if got := remoteOrder(f.site.episodes(f.project.ID)); !slices.Equal(got, f.episodes) {
		t.Fatalf("site order = %v", got)
	}
	ws, err := f.svc.WorkStatus(ctx, f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.SiteOnly) != 1 || ws.SiteOnly[0].NodeID != f.episodes[1] || ws.SiteOnly[0].State != EpisodeSiteOnly {
		t.Fatalf("site only = %+v", ws.SiteOnly)
	}
	v, err := f.svc.SetEpisodeVisibility(ctx, f.project.ID, f.episodes[1], VisibilityPrivate)
	if err != nil || v.State != EpisodePrivate {
		t.Fatalf("visibility = %+v, %v", v, err)
	}
}

func TestService_failuresChangeNothingLocally(t *testing.T) {
	f := newServiceFixture(t, 1)
	ctx := context.Background()
	f.register(t)
	before, _ := f.nodes.Get(ctx, f.episodes[0])
	f.site.failNext("PUT /api/artworks/works/"+f.project.ID+"/episodes/", 413, CodeTooLarge)
	if _, err := f.svc.PublishEpisode(ctx, f.episodes[0]); reasonOf(err) != rpc.ReasonPublishTooLarge {
		t.Fatalf("err = %v", err)
	}
	after, _ := f.nodes.Get(ctx, f.episodes[0])
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("the node changed:\n%+v\n%+v", before, after)
	}
	if eps, _ := f.state.Episodes(ctx, f.project.ID); len(eps) != 0 {
		t.Fatalf("state saved after a failure: %+v", eps)
	}

	f.site.srv.Close() // the network is gone
	_, err := f.svc.PublishEpisode(ctx, f.episodes[0])
	if r := reasonOf(err); r != rpc.ReasonPublishUnreachable {
		t.Fatalf("offline err = %v (%s)", err, r)
	}
	after, _ = f.nodes.Get(ctx, f.episodes[0])
	if !reflect.DeepEqual(before, after) {
		t.Fatal("the node changed while offline")
	}
}

func TestService_offlineStatusComesFromTheSavedCopy(t *testing.T) {
	f := newServiceFixture(t, 2)
	ctx := context.Background()
	f.register(t)
	f.publish(t, f.episodes[0])
	f.site.srv.Close()
	ws, err := f.svc.WorkStatus(ctx, f.project.ID)
	if err != nil {
		t.Fatalf("offline status errored: %v", err)
	}
	if !ws.Offline || !ws.Registered || ws.Episodes[0].State != EpisodePublic || ws.Episodes[1].State != EpisodeUnpublished {
		t.Fatalf("offline status = %+v", ws)
	}
}

func TestService_aRevokedTokenDisconnects(t *testing.T) {
	f := newServiceFixture(t, 1)
	f.register(t)
	f.site.mu.Lock()
	f.site.token = "lwk_other" // the writer removed this device on the web
	f.site.mu.Unlock()
	_, err := f.svc.PublishEpisode(context.Background(), f.episodes[0])
	if reasonOf(err) != rpc.ReasonPublishUnauthorized {
		t.Fatalf("err = %v", err)
	}
	if f.settings.PublishTokenExists() {
		t.Fatal("the refused token was kept")
	}
	if st := f.svc.connector.Status(); st.State != StateFailed || st.FailureReason != FailureRevoked {
		t.Fatalf("connector = %+v", st)
	}
}

func TestService_moderationStatesAreShown(t *testing.T) {
	f := newServiceFixture(t, 1)
	ctx := context.Background()
	f.register(t)
	f.publish(t, f.episodes[0])
	for status, want := range map[string]string{
		RemotePending: EpisodePending, RemoteRejected: EpisodeRejected,
		RemoteNeedsReview: EpisodeNeedsReview, RemotePrivate: EpisodePrivate,
	} {
		f.site.mu.Lock()
		f.site.works[f.project.ID].episodes[0].Status = status
		f.site.works[f.project.ID].episodes[0].ModerationReason = "사유"
		f.site.mu.Unlock()
		ws, err := f.svc.WorkStatus(ctx, f.project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := ws.Episodes[0]; got.State != want || got.ModerationReason != "사유" {
			t.Errorf("%s: %+v", status, got)
		}
	}
}

func TestService_emptyEpisodeAndNonEpisodeAreRefused(t *testing.T) {
	f := newServiceFixture(t, 1)
	ctx := context.Background()
	f.register(t)
	empty, err := f.nodes.CreateSibling(ctx, f.episodes[0], node.KindLeaf, "2화", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.PublishEpisode(ctx, empty.ID); reasonOf(err) != rpc.ReasonPublishEmptyEpisode {
		t.Fatalf("empty: %v", err)
	}
	if _, err := f.svc.PublishEpisode(ctx, f.arcID); reasonOf(err) != rpc.ReasonPublishNotEpisode {
		t.Fatalf("arc: %v", err)
	}
}

func TestDesiredOrder(t *testing.T) {
	cases := []struct {
		current, local []string
		target         string
		want           []string
	}{
		{nil, []string{"a"}, "a", []string{"a"}},
		{[]string{"a", "c"}, []string{"a", "b", "c"}, "b", []string{"a", "b", "c"}},
		{[]string{"a", "b"}, []string{"b", "a"}, "a", []string{"b", "a"}},
		// x is gone locally; it stays after a, and the lead y stays first.
		{[]string{"y", "a", "x", "b"}, []string{"a", "b", "c"}, "c", []string{"y", "a", "x", "b", "c"}},
	}
	for _, c := range cases {
		if got := desiredOrder(c.current, c.local, c.target); !slices.Equal(got, c.want) {
			t.Errorf("desiredOrder(%v, %v, %s) = %v, want %v", c.current, c.local, c.target, got, c.want)
		}
	}
}
