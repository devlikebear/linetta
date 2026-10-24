//go:build !mobile

package agent

import (
	"sync"

	"github.com/devlikebear/tars/pkg/llm"
)

// The writer's style notes and the latest scene summaries reach this agent
// through linetta_get_story_context and nothing else: the brief is not pasted
// into the system prompt (see systemPrompt). A turn that drafts a new scene
// reads it, because the prompt says to. A turn that only revises — "that line
// reads oddly", "make it a rollback instead" — tends not to: it reads the
// scene, fixes it, writes it back, and the approved style notes never enter
// the turn at all. Seen in practice: an evening's worth of revision turns
// in a row ran that way, each leaving prose the style notes forbid.
//
// So the turn refuses a prose write until it has read the brief. The refusal
// is an ordinary tool error the model reads and acts on, the same as a
// version conflict: one extra call, and the write goes through. This is the
// built-in agent's rule only. An external MCP client's workflow is its own.

const storyContextTool = "linetta_get_story_context"

// proseWriteTools are the tools that put prose into a scene.
var proseWriteTools = map[string]bool{
	"linetta_write_scene":  true,
	"linetta_revise_scene": true,
}

const briefFirstRefusal = "Not written. Call " + storyContextTool + " first, then write: the writer's " +
	"style notes and the latest scene summaries reach you only through it, and this turn has not read " +
	"it yet. Hold the revision to those style notes, even for a one-line fix."

// briefGate is one turn's record of whether it has read the brief. A pointer
// on loopState, so the copies of the state the loop and the CLI bridge pass
// around all share it; the mutex is for the bridge, whose calls arrive on
// HTTP handlers.
type briefGate struct {
	mu      sync.Mutex
	offered bool
	read    bool
}

// newBriefGate arms the gate only when the turn can satisfy it: a tool list
// without the brief must not leave every write refused.
func newBriefGate(schemas []llm.ToolSchema) *briefGate {
	g := &briefGate{}
	for _, s := range schemas {
		if s.Function.Name == storyContextTool {
			g.offered = true
			break
		}
	}
	return g
}

// refusal is the error to return instead of running name, or "" to run it.
func (g *briefGate) refusal(name string) string {
	if g == nil || !proseWriteTools[name] {
		return ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.offered || g.read {
		return ""
	}
	return briefFirstRefusal
}

// ran records a call that went through.
func (g *briefGate) ran(name string, ok bool) {
	if g == nil || name != storyContextTool || !ok {
		return
	}
	g.mu.Lock()
	g.read = true
	g.mu.Unlock()
}
