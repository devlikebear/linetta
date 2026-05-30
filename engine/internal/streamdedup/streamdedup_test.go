package streamdedup

import (
	"testing"
	"time"
)

// newDedupForTest constructs a Dedup with a controllable fake clock so
// back-to-back detection is deterministic. By default each Observe advances
// time well beyond the back-to-back window (so non-duplicate emissions never
// accidentally trip the back-to-back rule).
func newDedupForTest() (*Dedup, *fakeClock) {
	fc := &fakeClock{now: time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)}
	d := &Dedup{
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

func assertEmit(t *testing.T, d *Dedup, text, wantPayload string) {
	t.Helper()
	a, p := d.Observe(text)
	if a != ActionEmit {
		t.Fatalf("Observe(%q): action = %v, want emit", text, a)
	}
	if p != wantPayload {
		t.Errorf("Observe(%q): payload = %q, want %q", text, p, wantPayload)
	}
}

func assertSkip(t *testing.T, d *Dedup, text string) {
	t.Helper()
	a, _ := d.Observe(text)
	if a != ActionSkip {
		t.Fatalf("Observe(%q): action = %v, want skip", text, a)
	}
}

func assertReset(t *testing.T, d *Dedup, text, wantPayload string) {
	t.Helper()
	a, p := d.Observe(text)
	if a != ActionReset {
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
	// The user's actual bug: codex SSE emits identical delta events back-to-
	// back. With lastSeenAt extending on each suppress, an arbitrary run of
	// identical chunks folds even when each pair is just under the window
	// (which together would exceed the window from the first emit).
	d, fc := newDedupForTest()
	fc.tick = 70 * time.Millisecond // each step within 100ms but total >> 100ms
	assertEmit(t, d, "복", "복")
	assertSkip(t, d, "복") // back-to-back; 70ms since emit
	assertSkip(t, d, "복") // 70ms since prev skip — still within window
	assertSkip(t, d, "복") // 70ms since prev skip
	assertSkip(t, d, "복") // 70ms since prev skip
	if d.Final() != "복" {
		t.Errorf("Final = %q, want '복'", d.Final())
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
	if a != ActionSkip {
		t.Errorf("empty delta action = %v, want skip", a)
	}
	if d.Final() != "" {
		t.Errorf("Final = %q", d.Final())
	}
}
