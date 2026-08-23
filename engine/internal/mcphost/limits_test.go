//go:build !mobile

package mcphost

import (
	"testing"
	"time"
)

// A runaway loop must hit a wall; a human-paced session must not.
func TestLimiterStopsABurstAndRefills(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLimiter(60)
	l.now = func() time.Time { return now }

	for i := 0; i < 60; i++ {
		if !l.allow() {
			t.Fatalf("call %d was refused while the bucket should still be full", i+1)
		}
	}
	if l.allow() {
		t.Fatal("the 61st call in the same instant should be refused")
	}

	// One second of refill buys exactly one more call at 60/min.
	now = now.Add(time.Second)
	if !l.allow() {
		t.Fatal("the bucket should refill over time")
	}
	if l.allow() {
		t.Fatal("refill must not hand out more than it earned")
	}
}

// The bucket must not accumulate an unbounded burst while nobody is calling.
func TestLimiterCapsRefill(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLimiter(10)
	l.now = func() time.Time { return now }
	l.allow()

	now = now.Add(time.Hour)
	granted := 0
	for i := 0; i < 100; i++ {
		if l.allow() {
			granted++
		}
	}
	if granted != 10 {
		t.Fatalf("after a long idle the bucket granted %d calls, want the %d-call capacity", granted, 10)
	}
}
