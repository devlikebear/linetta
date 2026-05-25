package agent

import (
	"context"
	"sync"

	tesserarun "github.com/devlikebear/tessera/pkg/run"
)

// Broadcaster fans tessera Events out to per-run subscribers in real time so
// the HTTP SSE endpoint can stream live progress while a run is executing.
// One Broadcaster is shared across all in-flight runs; each run's eventRecorder
// calls Broadcaster.Publish to push events.
type Broadcaster struct {
	mu          sync.Mutex
	subscribers map[string][]chan tesserarun.Event
	// closed runs keep a short-lived completion signal so subscribers that
	// arrived after the run finished can immediately get a "done" sentinel
	// instead of blocking forever. We just close the channels.
	closed map[string]struct{}
}

// NewBroadcaster returns a Broadcaster ready to accept subscribers.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[string][]chan tesserarun.Event),
		closed:      make(map[string]struct{}),
	}
}

// Subscribe registers a new buffered channel that receives every event for
// runID until the broadcaster closes the run. The returned unsub function
// removes the subscriber early (e.g., when the HTTP client disconnects).
func (b *Broadcaster) Subscribe(runID string) (<-chan tesserarun.Event, func()) {
	ch := make(chan tesserarun.Event, 64)
	b.mu.Lock()
	if _, alreadyClosed := b.closed[runID]; alreadyClosed {
		// Run is already finished. Return a closed channel — receiver sees EOF.
		close(ch)
		b.mu.Unlock()
		return ch, func() {}
	}
	b.subscribers[runID] = append(b.subscribers[runID], ch)
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subscribers[runID]
		for i, s := range subs {
			if s == ch {
				b.subscribers[runID] = append(subs[:i], subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, unsub
}

// Publish delivers event to every subscriber of event.RunID. Slow subscribers
// drop events past their buffer (we don't want a stuck SSE client to back up
// the actual run execution).
func (b *Broadcaster) Publish(event tesserarun.Event) {
	b.mu.Lock()
	subs := append([]chan tesserarun.Event(nil), b.subscribers[event.RunID]...)
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// buffer full; skip rather than block
		}
	}
}

// Close marks the run as finished, closing all subscriber channels so SSE
// clients can finish naturally. Future Subscribe calls for this runID return
// an immediately-closed channel.
func (b *Broadcaster) Close(runID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers[runID] {
		close(ch)
	}
	delete(b.subscribers, runID)
	b.closed[runID] = struct{}{}
}

// broadcastingSink wraps the existing eventRecorder so the same events are
// both persisted to the DB AND fanned out via the broadcaster.
type broadcastingSink struct {
	inner       tesserarun.EventSink
	broadcaster *Broadcaster
}

func (s *broadcastingSink) OnEvent(ctx context.Context, event tesserarun.Event) error {
	if s.broadcaster != nil {
		s.broadcaster.Publish(event)
	}
	if s.inner == nil {
		return nil
	}
	return s.inner.OnEvent(ctx, event)
}
