//go:build !mobile

package mcphost

import (
	"context"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callsPerMinute caps how fast one server instance will serve tool calls. A
// runaway agent loop should hit a wall the writer can see in the activity log,
// not rewrite forty scenes. The limit is deliberately generous for a human-
// paced session and only bites on a loop.
const callsPerMinute = 120

// limiter is a simple token bucket refilled continuously. One bucket covers
// reads and writes alike: an agent stuck in a read loop is a problem too, and
// two buckets would be two things to reason about.
type limiter struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	perSec   float64
	last     time.Time
	now      func() time.Time
}

func newLimiter(perMinute int) *limiter {
	return &limiter{
		tokens:   float64(perMinute),
		capacity: float64(perMinute),
		perSec:   float64(perMinute) / 60,
		now:      time.Now,
	}
}

// allow reports whether a call may proceed, consuming one token if so.
func (l *limiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.last.IsZero() {
		l.last = now
	}
	l.tokens += now.Sub(l.last).Seconds() * l.perSec
	if l.tokens > l.capacity {
		l.tokens = l.capacity
	}
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

// limited wraps a typed tool handler with the rate limit. Applied at
// registration next to the activity decorator, so no tool can be added without
// one — the same "cannot forget" property.
func limited[In, Out any](l *limiter, h mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	if l == nil {
		return h
	}
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		if !l.allow() {
			var zero Out
			return toolErr(
				"too many Linetta tool calls in a short window (limit %d per minute). "+
					"Slow down, or ask the writer to review what you have changed so far.",
				callsPerMinute), zero, nil
		}
		return h(ctx, req, in)
	}
}

// NewLimiter returns the shared rate limiter for one engine tool layer.
func NewLimiter() *limiter { return newLimiter(callsPerMinute) }
