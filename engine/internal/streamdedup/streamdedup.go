package streamdedup

import (
	"strings"
	"sync"
	"time"
)

// Dedup filters OnDelta callbacks against two upstream-provider quirks:
//
//  1. **Tars stream retry.** The openai-codex provider in tars retries the
//     entire SSE stream on certain transient errors, replaying every OnDelta
//     callback from the start. Detected by prefix matching against the
//     accumulated buf.
//  2. **Codex duplicate-delta events.** The openai-codex SSE stream sometimes
//     emits `response.output_text.delta` events with identical `delta` text
//     back-to-back (different sequence_numbers, same content). Detected by
//     comparing each delta against the most recent emission within a tight
//     time window (default 100ms).
//
// Both rules suppress notifications without losing the deduplicated buf, so
// the cancelled/error paths still record what the user actually saw.
//
// State machine (prefix-match):
//   - state 0 (normal): try back-to-back check first; then prefix-match.
//     If the incoming delta is a prefix of buf, buffer it as `pending` and
//     move to state 1 (no emission yet).
//   - state 1 (armed): if the next delta matches buf[cursor:], confirm and
//     move to state 2; otherwise emit pending+text combined and revert to 0.
//   - state 2 (confirmed retry): each matching delta advances cursor and is
//     suppressed. When cursor catches up to len(buf), drop to state 0.
//     If a delta diverges, truncate buf at cursor, append it, surface a
//     reset action so the caller can REPLACE the frontend's running text.
type Dedup struct {
	mu  sync.Mutex
	buf string

	// Prefix-match retry detection.
	state   int    // 0 normal, 1 armed, 2 confirmed retry
	cursor  int    // matched prefix position in buf when in state 1 or 2
	pending string // text saved during state 1; dropped on confirmation, emitted on false positive

	// Back-to-back duplicate detection.
	lastDelta  string
	lastSeenAt time.Time
	nowFn      func() time.Time

	// Configurable in tests; default 100ms.
	backToBackWindow time.Duration
}

// New constructs a Dedup with default time-based config.
func New() *Dedup {
	return &Dedup{
		nowFn:            time.Now,
		backToBackWindow: 100 * time.Millisecond,
	}
}

// Action represents the result of an Observe call.
type Action int

const (
	ActionEmit  Action = iota // notify ai.delta with payloadText (new content)
	ActionSkip                // suppress notification (retry replay, armed-buffer, or back-to-back dup)
	ActionReset               // notify ai.reset with payloadText (replace running text)
)

// Observe runs the dedup state machine on one incoming OnDelta text.
func (d *Dedup) Observe(text string) (Action, string) {
	if text == "" {
		return ActionSkip, ""
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	switch d.state {
	case 0:
		now := d.nowFn()
		withinWindow := now.Sub(d.lastSeenAt) < d.backToBackWindow
		// Back-to-back duplicate: same content as the most recent observation
		// (emit OR suppress), arriving within the tight window. Catches the
		// codex provider's quirk of emitting the same delta content in
		// consecutive SSE events. lastSeenAt extends on each suppress so a
		// rapid run of identical chunks all gets folded.
		if text == d.lastDelta && d.lastDelta != "" && withinWindow {
			d.lastSeenAt = now
			return ActionSkip, ""
		}
		// Plausible retry-from-start: incoming delta is a prefix of buf AND
		// arrives within the retry-likely time window. The window check
		// protects legitimate model stutters (e.g., "라 라 라" paced 200ms
		// apart) from being incorrectly recognized as a retry replay.
		if withinWindow && len(d.buf) > 0 && len(text) < len(d.buf) && strings.HasPrefix(d.buf, text) {
			d.state = 1
			d.cursor = len(text)
			d.pending = text
			d.lastSeenAt = now
			return ActionSkip, ""
		}
		// Genuine incremental delta.
		d.buf += text
		d.lastDelta = text
		d.lastSeenAt = now
		return ActionEmit, text

	case 1:
		remaining := d.buf[d.cursor:]
		if strings.HasPrefix(remaining, text) {
			d.state = 2
			d.cursor += len(text)
			if d.cursor >= len(d.buf) {
				d.state = 0
				d.cursor = 0
			}
			d.pending = ""
			return ActionSkip, ""
		}
		// False positive: pending was genuinely new content that happened to
		// match a buf prefix. Emit pending+text as a single catch-up.
		combined := d.pending + text
		d.buf += combined
		d.state = 0
		d.cursor = 0
		d.pending = ""
		now := d.nowFn()
		d.lastDelta = text
		d.lastSeenAt = now
		return ActionEmit, combined

	case 2:
		remaining := d.buf[d.cursor:]
		if strings.HasPrefix(remaining, text) {
			d.cursor += len(text)
			if d.cursor >= len(d.buf) {
				d.state = 0
				d.cursor = 0
			}
			return ActionSkip, ""
		}
		// Divergence during confirmed retry: tars's retried response diverged
		// from the first attempt past the matched prefix. Truncate, append,
		// surface a reset so the frontend replaces its running text.
		d.buf = d.buf[:d.cursor] + text
		d.state = 0
		d.cursor = 0
		now := d.nowFn()
		d.lastDelta = text
		d.lastSeenAt = now
		return ActionReset, d.buf
	}
	return ActionSkip, ""
}

// Final returns the deduplicated accumulated buffer.
func (d *Dedup) Final() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.buf
}
