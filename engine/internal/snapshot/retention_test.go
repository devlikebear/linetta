package snapshot

import (
	"context"
	"testing"
	"time"
)

func TestThin_keepsLast24hUntouched(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	// Anchor "now"
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).UnixMilli()
	for i := 0; i < 10; i++ {
		_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, now-int64(i*5*60*1000)) // every 5 min
	}
	if err := Thin(ctx, r.s.DB(), now); err != nil {
		t.Fatalf("Thin: %v", err)
	}
	got, err := countAutosaves(ctx, r, nodeID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 10 {
		t.Errorf("within-24h autosaves: kept %d, want 10", got)
	}
}

func TestThin_thinsToOnePerHourBetween24hAnd30d(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).UnixMilli()
	// Two autosaves in the same hour, 25h ago.
	old := now - int64(25*time.Hour/time.Millisecond)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, old)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, old+60*1000) // 1 min later, same hour
	if err := Thin(ctx, r.s.DB(), now); err != nil {
		t.Fatalf("Thin: %v", err)
	}
	got, _ := countAutosaves(ctx, r, nodeID)
	if got != 1 {
		t.Errorf("kept %d in same hour bucket, want 1", got)
	}
}

func TestThin_thinsToOnePerDayBeyond30d(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).UnixMilli()
	old := now - int64(31*24*time.Hour/time.Millisecond)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, old)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, old+3*60*60*1000) // 3h later, same day
	if err := Thin(ctx, r.s.DB(), now); err != nil {
		t.Fatalf("Thin: %v", err)
	}
	got, _ := countAutosaves(ctx, r, nodeID)
	if got != 1 {
		t.Errorf("kept %d in same day bucket beyond 30d, want 1", got)
	}
}

func TestThin_preservesManualAndAIReplace(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).UnixMilli()
	old := now - int64(60*24*time.Hour/time.Millisecond)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonManual, old)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAIReplace, old+1000)
	if err := Thin(ctx, r.s.DB(), now); err != nil {
		t.Fatalf("Thin: %v", err)
	}
	var n int
	if err := r.s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_snapshots WHERE node_id = ?`, nodeID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("manual+ai-replace kept count = %d, want 2", n)
	}
}

func countAutosaves(ctx context.Context, r *Repo, nodeID string) (int, error) {
	var n int
	err := r.s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_snapshots WHERE node_id = ? AND reason = 'autosave'`, nodeID).Scan(&n)
	return n, err
}
