//go:build !mobile

package engineapp

import (
	"encoding/json"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/provider"
)

// SetProviderFactoryForTest replaces the client factory so a test can drive
// the loop without a network. Test-only: production wires llm.NewProvider in
// provider.NewSource.
//
// This lives in a _test.go file, not agent_enabled.go: an earlier version of
// this task's brief believed a non-test file was required because "a
// non-test file returns [notificationLog], so it cannot live in a _test.go"
// — that reasoning was wrong. A _test.go file in package engineapp can
// declare methods on *App and its own types just as well as any other file
// in the package; only a DIFFERENT package's tests would be unable to import
// them. Keeping these three symbols in a _test.go file means they are never
// compiled into a shipped binary at all, not merely unused by one.
func (a *App) SetProviderFactoryForTest(f provider.ClientFactory) {
	a.providerSrc.WithFactory(f)
}

// notificationLog records what the engine emitted, method and params both.
// The agent's contract is largely a notification contract — a run id comes
// back immediately and everything after it arrives as agent.* — so a test
// that cannot see the notifications can only assert on side effects, and
// would pass on an engine that did the work but told the panel nothing.
// Params are kept (not just the method name) so a test can read WHY a turn
// ended when it ended in agent.error rather than agent.done — see
// waitForNotification in agent_run_test.go.
type notificationLog struct {
	mu     sync.Mutex
	events []notificationEvent
}

type notificationEvent struct {
	method string
	params json.RawMessage
}

func (n *notificationLog) add(method string, params json.RawMessage) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, notificationEvent{method, params})
}

func (n *notificationLog) saw(method string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, e := range n.events {
		if e.method == method {
			return true
		}
	}
	return false
}

// paramsFor returns the params of the most recent event for method, or nil
// if method was never emitted.
func (n *notificationLog) paramsFor(method string) json.RawMessage {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := len(n.events) - 1; i >= 0; i-- {
		if n.events[i].method == method {
			return n.events[i].params
		}
	}
	return nil
}

// CaptureNotificationsForTest routes notifications into a log. It REPLACES
// the current notifier rather than chaining: in a test the notifier is the
// stdio default, which nothing is reading, and a getter on rpc.Server exists
// only to serve a chain nobody needs.
func (a *App) CaptureNotificationsForTest() *notificationLog {
	log := &notificationLog{}
	a.SetNotifier(func(method string, params json.RawMessage) { log.add(method, params) })
	return log
}
