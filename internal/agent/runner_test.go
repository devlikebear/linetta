package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/internal/memory"
	"github.com/devlikebear/linetta/internal/store"
	"github.com/devlikebear/linetta/internal/work"
	"github.com/devlikebear/tessera/pkg/observe"
	tesserarun "github.com/devlikebear/tessera/pkg/run"
)

func TestRunnerRunsEpisodeThroughTesseraAndStoresArtifacts(t *testing.T) {
	ctx := context.Background()
	db, workRepo, memoryRepo, workID, episodeID := newRunnerFixture(t)
	runner := NewRunner(db, workRepo, memoryRepo)
	sink := &observe.MemorySink{}

	result, err := runner.RunEpisode(ctx, EpisodeRunInput{
		WorkID:     workID,
		EpisodeID:  episodeID,
		ApprovedBy: "tester",
		ApprovedAt: time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
		EventSink:  sink,
	})
	if err != nil {
		t.Fatalf("RunEpisode() error = %v", err)
	}

	if result.RunID == "" {
		t.Fatal("RunEpisode() returned empty run ID")
	}
	if result.Closure != string(tesserarun.ClosureNormal) {
		t.Fatalf("closure = %q, want normal", result.Closure)
	}
	if len(result.Artifacts) != 7 {
		t.Fatalf("artifacts len = %d, want 7", len(result.Artifacts))
	}
	if !hasArtifact(result.Artifacts, ArtifactKindDraft) {
		t.Fatalf("artifacts = %+v, want draft artifact", result.Artifacts)
	}

	stored, err := runner.ListArtifacts(ctx, result.RunID)
	if err != nil {
		t.Fatalf("ListArtifacts() error = %v", err)
	}
	if len(stored) != len(result.Artifacts) {
		t.Fatalf("stored artifacts len = %d, want %d", len(stored), len(result.Artifacts))
	}

	events := sink.Events()
	if len(events) == 0 {
		t.Fatal("expected external sink events")
	}
	if !hasEventType(events, tesserarun.EventTaskSucceeded) {
		t.Fatalf("events = %+v, want task.succeeded event", events)
	}
	storedEvents, err := runner.ListEvents(ctx, result.RunID)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(storedEvents) != len(events) {
		t.Fatalf("stored events len = %d, want %d", len(storedEvents), len(events))
	}
}

func newRunnerFixture(t *testing.T) (*store.DB, *work.Repository, *memory.Repository, string, string) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "linetta.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	workRepo := work.NewRepository(db)
	memoryRepo := memory.NewRepository(db, workRepo)

	workItem, err := workRepo.CreateWork(ctx, work.CreateWorkInput{
		Title:   "Green Harbor",
		Genre:   "web novel",
		Premise: "A caretaker protects a city memory system.",
	})
	if err != nil {
		t.Fatalf("CreateWork() error = %v", err)
	}
	episode, err := workRepo.CreateEpisode(ctx, workItem.ID, "Episode 1")
	if err != nil {
		t.Fatalf("CreateEpisode() error = %v", err)
	}
	if _, err := workRepo.SaveBlueprint(ctx, workItem.ID, episode.ID, work.SaveBlueprintInput{
		Premise:        "Mira hears an old pump singing.",
		Theme:          "Memory as infrastructure",
		Situation:      "A rhythm changes before sunset.",
		MustInclude:    "lullaby clue",
		MustAvoid:      "exposition dump",
		StructureNotes: "Open with a ritual, end with a message.",
	}); err != nil {
		t.Fatalf("SaveBlueprint() error = %v", err)
	}
	if _, err := memoryRepo.CreateItem(ctx, memory.CreateItemInput{
		WorkID: workItem.ID,
		Kind:   memory.KindCharacter,
		Title:  "Mira",
		Body:   "A tide-garden caretaker.",
		Status: memory.StatusCanon,
	}); err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	return db, workRepo, memoryRepo, workItem.ID, episode.ID
}

func hasArtifact(items []Artifact, kind ArtifactKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func hasEventType(events []tesserarun.Event, eventType tesserarun.EventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
