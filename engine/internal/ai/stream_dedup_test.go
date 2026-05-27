package ai

import (
	"testing"
	"time"
)

// newDedupForTest constructs a streamDedup with a controllable fake clock so
// back-to-back detection is deterministic. By default each Observe advances
// time well beyond the back-to-back window (so non-duplicate emissions never
// accidentally trip the back-to-back rule).
func newDedupForTest() (*streamDedup, *fakeClock) {
	fc := &fakeClock{now: time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)}
	d := &streamDedup{
		nowFn:            fc.Now,
		backToBackWindow: 100 * time.Millisecond,
	}
	return d, fc
}

type fakeClock struct {
	now time.Time
	// Each Now() call advances by `tick` (default 1s — well beyond window).
	tick time.Duration
}

func (c *fakeClock) Now() time.Time {
	t := c.now
	if c.tick == 0 {
		c.now = c.now.Add(time.Second)
	} else {
		c.now = c.now.Add(c.tick)
	}
	return t
}

func assertEmit(t *testing.T, d *streamDedup, text, wantPayload string) {
	t.Helper()
	a, p := d.Observe(text)
	if a != dedupEmit {
		t.Fatalf("Observe(%q): action = %v, want emit", text, a)
	}
	if p != wantPayload {
		t.Errorf("Observe(%q): payload = %q, want %q", text, p, wantPayload)
	}
}

func assertSkip(t *testing.T, d *streamDedup, text string) {
	t.Helper()
	a, _ := d.Observe(text)
	if a != dedupSkip {
		t.Fatalf("Observe(%q): action = %v, want skip", text, a)
	}
}

func assertReset(t *testing.T, d *streamDedup, text, wantPayload string) {
	t.Helper()
	a, p := d.Observe(text)
	if a != dedupReset {
		t.Fatalf("Observe(%q): action = %v, want reset", text, a)
	}
	if p != wantPayload {
		t.Errorf("Observe(%q): payload = %q, want %q", text, p, wantPayload)
	}
}

func TestStreamDedup_genuineIncremental(t *testing.T) {
	d, _ := newDedupForTest()
	assertEmit(t, d, "안", "안")
	assertEmit(t, d, "녕", "녕")
	if d.Final() != "안녕" {
		t.Errorf("Final = %q", d.Final())
	}
}

func TestStreamDedup_backToBackDup_isSuppressed(t *testing.T) {
	// The user's actual bug: codex SSE emits two events with the same delta
	// back-to-back. Within the 100ms window, the second is suppressed.
	d, fc := newDedupForTest()
	fc.tick = 10 * time.Millisecond // tight succession
	assertEmit(t, d, "그리고", "그리고")
	assertSkip(t, d, "그리고") // back-to-back dup
	assertSkip(t, d, "그리고") // still dup
	assertSkip(t, d, "그리고") // still dup
	if d.Final() != "그리고" {
		t.Errorf("Final = %q, want '그리고'", d.Final())
	}
}

func TestStreamDedup_legitStutter_outsideWindow_isKept(t *testing.T) {
	// Genuine "라라라" model output, paced like a real stutter (>100ms apart):
	// each "라" must be kept.
	d, fc := newDedupForTest()
	fc.tick = 200 * time.Millisecond
	assertEmit(t, d, "라", "라")
	assertEmit(t, d, "라", "라") // different time, NOT back-to-back
	assertEmit(t, d, "라", "라")
	if d.Final() != "라라라" {
		t.Errorf("Final = %q", d.Final())
	}
}

func TestStreamDedup_confirmedRetry_suppressesReplay(t *testing.T) {
	d, fc := newDedupForTest()
	fc.tick = 50 * time.Millisecond // within back-to-back / retry window
	assertEmit(t, d, "안", "안")
	assertEmit(t, d, "녕", "녕")
	assertEmit(t, d, "그", "그")
	assertEmit(t, d, "리고", "리고")
	// Tars retries from the start (full replay).
	assertSkip(t, d, "안")  // armed
	assertSkip(t, d, "녕")  // confirmed
	assertSkip(t, d, "그")  // continued
	assertSkip(t, d, "리고") // catches up
	// Continuation past the original end:
	assertEmit(t, d, " 좋다", " 좋다")
	if d.Final() != "안녕그리고 좋다" {
		t.Errorf("Final = %q", d.Final())
	}
}

func TestStreamDedup_falsePositive_catchesUpWithoutLoss(t *testing.T) {
	d, fc := newDedupForTest()
	fc.tick = 50 * time.Millisecond
	assertEmit(t, d, "안", "안")
	assertEmit(t, d, "녕", "녕")
	// "안" matches start of buf → armed.
	assertSkip(t, d, "안")
	// "다" doesn't match buf[1:] = "녕" → false positive; emit combined "안다".
	assertEmit(t, d, "다", "안다")
	if d.Final() != "안녕안다" {
		t.Errorf("Final = %q (want 안녕안다)", d.Final())
	}
}

func TestStreamDedup_divergenceDuringRetry_emitsReset(t *testing.T) {
	d, fc := newDedupForTest()
	fc.tick = 50 * time.Millisecond
	assertEmit(t, d, "안", "안")
	assertEmit(t, d, "녕", "녕")
	assertEmit(t, d, "그", "그")
	assertEmit(t, d, "리고", "리고")
	// Retry restart.
	assertSkip(t, d, "안") // armed
	assertSkip(t, d, "녕") // confirmed; cursor = 2
	// Diverging continuation.
	assertReset(t, d, "다른", "안녕다른")
	if d.Final() != "안녕다른" {
		t.Errorf("Final = %q", d.Final())
	}
}

func TestStreamDedup_emptyDeltaIsSkipped(t *testing.T) {
	d, _ := newDedupForTest()
	a, _ := d.Observe("")
	if a != dedupSkip {
		t.Errorf("empty delta action = %v, want skip", a)
	}
	if d.Final() != "" {
		t.Errorf("Final = %q", d.Final())
	}
}
