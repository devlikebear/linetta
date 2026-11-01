package companion

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/pkg/memory"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
)

func memoryService(t *testing.T) (context.Context, *Service, string) {
	t.Helper()
	ctx, st, _, p := seedWork(t)
	return ctx, NewService(t.TempDir()).WithCuratedMemory(agentmemory.NewRepo(st.DB())), p.ID
}

func TestRememberAndRecall(t *testing.T) {
	_, svc, id := memoryService(t)
	if err := svc.Remember(id, "작가는 단문을 선호한다", "preference"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Remember(id, "세계관: 마법은 금지되어 있다", "lore"); err != nil {
		t.Fatal(err)
	}
	hits := svc.Recall(id, "단문", 5)
	if len(hits) != 1 || hits[0] != "작가는 단문을 선호한다" {
		t.Fatalf("recall(단문) = %+v", hits)
	}
	if got := svc.Recall(id, "존재하지않는키워드", 5); len(got) != 0 {
		t.Fatalf("expected no hits, got %+v", got)
	}
	if got := svc.Recall(id, "", 5); len(got) != 2 {
		t.Fatalf("an empty query lists them, got %+v", got)
	}
	if got := svc.Recall("p2", "단문", 5); len(got) != 0 {
		t.Fatalf("p2 should have no memory, got %+v", got)
	}
}

func TestRememberRequiresText(t *testing.T) {
	_, svc, id := memoryService(t)
	if err := svc.Remember(id, "", "x"); err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestRememberNeedsTheArchive(t *testing.T) {
	svc := NewService(t.TempDir())
	if err := svc.Remember("p1", "기억", "x"); err == nil {
		t.Fatal("a build with no archive must say so")
	}
}

func writeExperience(t *testing.T, base, projectID, text string) {
	t.Helper()
	if err := memory.AppendExperience(memRoot(base, projectID), memory.Experience{Summary: text, Category: "lore"}); err != nil {
		t.Fatalf("AppendExperience: %v", err)
	}
}

func TestImportExperiencesRunsOnceSoADeletedLineStaysDeleted(t *testing.T) {
	ctx, svc, id := memoryService(t)
	writeExperience(t, svc.memBase, id, "곰배는 조연")
	if report := svc.ImportExperiences(ctx); len(report) != 0 {
		t.Fatalf("report = %v", report)
	}
	list, err := svc.memories.ListArchived(ctx, id, 0)
	if err != nil || len(list) != 1 || list[0].Body != "곰배는 조연" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := svc.memories.DeleteArchived(ctx, id, list[0].ID); err != nil {
		t.Fatalf("DeleteArchived: %v", err)
	}
	svc.ImportExperiences(ctx)
	if list, _ := svc.memories.ListArchived(ctx, id, 0); len(list) != 0 {
		t.Errorf("a deleted line came back: %+v", list)
	}
	log := filepath.Join(memRoot(svc.memBase, id), memoryWorkspaceMarker)
	if _, err := os.Stat(log); err != nil {
		t.Errorf("the log must stay for the export: %v", err)
	}
}

func TestImportExperiencesLeavesAnUnknownWorkUnmarked(t *testing.T) {
	ctx, svc, _ := memoryService(t)
	writeExperience(t, svc.memBase, "gone", "사라진 작품의 기억")
	if report := svc.ImportExperiences(ctx); len(report) != 1 {
		t.Fatalf("report = %v", report)
	}
	marker := filepath.Join(filepath.Dir(filepath.Join(memRoot(svc.memBase, "gone"), memoryWorkspaceMarker)), importedMarker)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a log nothing was imported from must not be marked done")
	}
}

func TestImportExperiencesSkipsWhatIsNotAMemoryWorkspace(t *testing.T) {
	ctx, svc, _ := memoryService(t)
	if err := os.MkdirAll(filepath.Join(svc.memBase, "backups"), 0o755); err != nil {
		t.Fatal(err)
	}
	if report := svc.ImportExperiences(ctx); len(report) != 0 {
		t.Errorf("report = %v", report)
	}
}
