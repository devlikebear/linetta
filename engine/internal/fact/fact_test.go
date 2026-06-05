package fact

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type factFixture struct {
	repo      *Repo
	projectID string
	nodeID    string
}

func newFixture(t *testing.T) factFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	p, err := project.NewRepo(s).Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("project.Create: %v", err)
	}
	return factFixture{repo: NewRepo(s), projectID: p.ID, nodeID: *p.LastOpenedNodeID}
}

func TestRepo_CreateRequiresSourceURL(t *testing.T) {
	f := newFixture(t)
	_, err := f.repo.Create(context.Background(), 10, NewInput{
		ProjectID: f.projectID,
		Claim:    "서울 지하철 막차는 보통 새벽까지 운행한다",
		Result:   "노선별로 다르므로 최신 시간표 확인이 필요하다.",
		Status:   StatusUncertain,
	})
	if err == nil {
		t.Fatal("expected source-required error")
	}
}

func TestRepo_CreateListUpdateDelete(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	card, err := f.repo.Create(ctx, 10, NewInput{
		ProjectID: f.projectID,
		NodeID:    &f.nodeID,
		Claim:    "런던 경찰은 제복 근무 중 총을 항상 휴대한다",
		Result:   "일반 경찰은 통상 총기를 휴대하지 않는다.",
		Status:   StatusVerified,
		Category: "police",
		Sources: []SourceInput{{
			URL:       "https://www.met.police.uk/",
			Title:     "Met Police",
			Snippet:   "Official policing reference",
			AccessedAt: 10,
		}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if card.ID == "" || len(card.Sources) != 1 {
		t.Fatalf("card = %+v", card)
	}

	list, err := f.repo.List(ctx, ListFilter{ProjectID: f.projectID, NodeID: &f.nodeID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Claim != card.Claim {
		t.Fatalf("list = %+v", list)
	}

	claim := "수정된 주장"
	status := StatusStale
	updated, err := f.repo.Update(ctx, 20, UpdateInput{ID: card.ID, Claim: &claim, Status: &status})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Claim != claim || updated.Status != StatusStale || updated.UpdatedAt != 20 {
		t.Fatalf("updated = %+v", updated)
	}

	if err := f.repo.Delete(ctx, card.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := f.repo.Get(ctx, card.ID); err != ErrNotFound {
		t.Fatalf("Get deleted err = %v, want ErrNotFound", err)
	}
}

func TestRepo_ListWithNodeIncludesProjectWideCards(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	projectWide, err := f.repo.Create(ctx, 10, NewInput{
		ProjectID: f.projectID,
		Claim:    "프로젝트 전체 자료",
		Result:   "전체 배경에 쓰는 자료",
		Status:   StatusVerified,
		Sources:  []SourceInput{{URL: "https://example.com/project", AccessedAt: 10}},
	})
	if err != nil {
		t.Fatalf("Create project-wide: %v", err)
	}
	sceneCard, err := f.repo.Create(ctx, 20, NewInput{
		ProjectID: f.projectID,
		NodeID:    &f.nodeID,
		Claim:    "현재 씬 자료",
		Result:   "현재 씬에만 연결된 자료",
		Status:   StatusUncertain,
		Sources:  []SourceInput{{URL: "https://example.com/scene", AccessedAt: 20}},
	})
	if err != nil {
		t.Fatalf("Create scene: %v", err)
	}

	list, err := f.repo.List(ctx, ListFilter{ProjectID: f.projectID, NodeID: &f.nodeID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(list), list)
	}
	seen := map[string]bool{}
	for _, card := range list {
		seen[card.ID] = true
	}
	if !seen[projectWide.ID] || !seen[sceneCard.ID] {
		t.Fatalf("list missing project-wide or scene card: %+v", list)
	}
}
