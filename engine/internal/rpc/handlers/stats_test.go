package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/stats"
)

func TestTodayStatsHandler_returnsTodaysAddedChars(t *testing.T) {
	f := newNodeFixture(t)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	statRepo := stats.NewRepoWithClock(f.store, time.UTC, func() time.Time { return now })
	f.nodes.SetWritingStatsRecorder(statRepo)

	update := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return now.UnixMilli() }, nil)
	if _, err := update(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"오늘 작성\"}]}]}"}`)); err != nil {
		t.Fatalf("update: %v", err)
	}

	h := TodayStats(statRepo)
	res, err := h(context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out stats.Today
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.CharsAdded != 5 {
		t.Errorf("chars_added = %d, want 5", out.CharsAdded)
	}
}

func TestTodayStatsHandler_requiresProjectID(t *testing.T) {
	f := newNodeFixture(t)
	h := TodayStats(stats.NewRepo(f.store))

	_, err := h(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if me, ok := err.(interface{ Error() string }); !ok || me.Error() != "project_id required" {
		t.Fatalf("err = %v, want project_id required", err)
	}
}

func TestRangeStatsHandler_returnsRequestedDays(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	statRepo := stats.NewRepoWithClock(f.store, time.UTC, func() time.Time {
		return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	})
	if _, err := f.store.DB().ExecContext(ctx, `
INSERT INTO writing_stats (project_id, day, chars_added)
VALUES (?, ?, ?), (?, ?, ?)`, f.pID, "2026-06-08", 100, f.pID, "2026-06-10", 300); err != nil {
		t.Fatalf("seed stats: %v", err)
	}

	h := RangeStats(statRepo)
	res, err := h(ctx, json.RawMessage(`{"project_id":"`+f.pID+`","from_day":"2026-06-08","to_day":"2026-06-10"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out []stats.DayStat
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 3 || out[0].CharsAdded != 100 || out[1].CharsAdded != 0 || out[2].CharsAdded != 300 {
		t.Fatalf("range = %+v", out)
	}
}

func TestSummaryStatsHandler_returnsSummary(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	statRepo := stats.NewRepoWithClock(f.store, time.UTC, func() time.Time { return now })
	if _, err := f.store.DB().ExecContext(ctx, `
INSERT INTO writing_stats (project_id, day, chars_added)
VALUES (?, ?, ?), (?, ?, ?)`, f.pID, "2026-06-04", 70, f.pID, "2026-06-10", 700); err != nil {
		t.Fatalf("seed stats: %v", err)
	}

	h := SummaryStats(statRepo)
	res, err := h(ctx, json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out stats.Summary
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Today != 700 || out.WeekAvg != 110 || out.TotalDays != 2 {
		t.Fatalf("summary = %+v", out)
	}
}
