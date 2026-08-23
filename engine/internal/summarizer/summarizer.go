// Package summarizer keeps node.summary in sync with node.content_doc by
// running a background LLM summarization job after every content change.
package summarizer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
)

const queueSize = 256
const minRunesForLLM = 60
const containerSummaryMaxRunes = 4000
const maxSummarizeDepth = 6

// Summaries run in the background with no UI-language signal, so the prompt
// asks the model to follow the manuscript's own language instead.

type Summarizer struct {
	nodes   *node.Repo
	src     ai.ProviderSource
	factory ai.ClientFactory
	ch      chan string
	ops     *opsstatus.Repo
	now     func() int64

	statusMu     sync.Mutex
	failureCount int
}

func New(nodes *node.Repo, src ai.ProviderSource, factory ai.ClientFactory) *Summarizer {
	return &Summarizer{
		nodes: nodes, src: src, factory: factory,
		ch: make(chan string, queueSize),
	}
}

func (s *Summarizer) WithOpsStatus(repo *opsstatus.Repo, now func() int64) *Summarizer {
	s.ops = repo
	s.now = now
	return s
}

func (s *Summarizer) Start(parent context.Context) (stop func()) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-s.ch:
				s.summarizeOne(ctx, id)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (s *Summarizer) Enqueue(nodeID string) {
	if nodeID == "" {
		return
	}
	select {
	case s.ch <- nodeID:
	default:
		// queue full — drop; a later save will re-enqueue.
	}
}

// summarizeOne is the thin entry point used by the background queue. It
// delegates to summarizeOneDepth with depth=0.
func (s *Summarizer) summarizeOne(ctx context.Context, nodeID string) {
	s.summarizeOneDepth(ctx, nodeID, 0)
}

// RefreshNow synchronously summarizes the node (and any stale descendants).
// Implements storycontext.SummaryRefresher so ContextBuilder can populate the
// hierarchical layer without waiting on the background queue.
func (s *Summarizer) RefreshNow(ctx context.Context, nodeID string) {
	s.summarizeOneDepth(ctx, nodeID, 0)
}

// summarizeOneDepth dispatches on node.Kind. Container nodes recurse into
// their children with depth+1; the depth cap prevents runaway recursion if
// the parent_id graph were ever to become cyclic.
func (s *Summarizer) summarizeOneDepth(ctx context.Context, nodeID string, depth int) {
	if depth > maxSummarizeDepth {
		msg := fmt.Sprintf("depth cap hit at %s (depth=%d)", nodeID, depth)
		fmt.Fprintf(os.Stderr, "summarizer: %s\n", msg)
		s.recordError(ctx, nodeID, msg)
		return
	}
	n, err := s.nodes.Get(ctx, nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: get %s: %v\n", nodeID, err)
		s.recordError(ctx, nodeID, err.Error())
		return
	}
	if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
		return
	}
	switch n.Kind {
	case "leaf":
		s.summarizeLeaf(ctx, n)
	case "container":
		s.summarizeContainer(ctx, n, depth)
	}
}

func (s *Summarizer) summarizeLeaf(ctx context.Context, n node.Node) {
	if n.ContentDoc == nil {
		return
	}
	capturedVersion := n.ContentVersion
	plain := strings.TrimSpace(docToPlainText(*n.ContentDoc))
	if plain == "" {
		return
	}

	if runeLen(plain) < minRunesForLLM {
		if err := s.nodes.SetSummary(ctx, n.ID, plain, capturedVersion); err != nil {
			fmt.Fprintf(os.Stderr, "summarizer: SetSummary (short) %s: %v\n", n.ID, err)
			s.recordError(ctx, n.ID, err.Error())
			return
		}
		s.recordOK(ctx, n.ID)
		return
	}

	summary, ok := s.summarizeViaLLM(ctx, n.ID, "", systemPrompt, plain)
	if !ok {
		return
	}
	if err := s.nodes.SetSummary(ctx, n.ID, summary, capturedVersion); err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: SetSummary %s: %v\n", n.ID, err)
		s.recordError(ctx, n.ID, err.Error())
		return
	}
	s.recordOK(ctx, n.ID)
}

// summarizeContainer rolls a container up from its children's Label+summary.
// Stale children are summarized first via a depth-first recursion.
func (s *Summarizer) summarizeContainer(ctx context.Context, n node.Node, depth int) {
	capturedVersion := n.ContentVersion
	children, err := s.nodes.ListChildren(ctx, n.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: ListChildren %s: %v\n", n.ID, err)
		s.recordError(ctx, n.ID, err.Error())
		return
	}
	if len(children) == 0 {
		return
	}
	// Recurse into stale children first so we have fresh summaries to roll up.
	for _, c := range children {
		if c.Summary == "" || c.SummaryForVersion != c.ContentVersion {
			s.summarizeOneDepth(ctx, c.ID, depth+1)
		}
	}
	// Re-read children after recursion to pick up fresh summaries.
	children, err = s.nodes.ListChildren(ctx, n.ID)
	if err != nil {
		s.recordError(ctx, n.ID, err.Error())
		return
	}
	var b strings.Builder
	for _, c := range children {
		if c.Summary == "" {
			continue
		}
		b.WriteString(c.Label)
		b.WriteString("\n")
		b.WriteString(c.Summary)
		b.WriteString("\n\n")
	}
	input := strings.TrimSpace(b.String())
	if input == "" {
		return
	}
	// Truncate trailing runes if over budget (keep the earlier children which
	// generally correspond to the start of the unit).
	if r := []rune(input); len(r) > containerSummaryMaxRunes {
		input = string(r[:containerSummaryMaxRunes])
	}

	summary, ok := s.summarizeViaLLM(ctx, n.ID, " (container)", containerSystemPrompt, input)
	if !ok {
		return
	}
	if err := s.nodes.SetSummary(ctx, n.ID, summary, capturedVersion); err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: SetSummary (container) %s: %v\n", n.ID, err)
		s.recordError(ctx, n.ID, err.Error())
		return
	}
	s.recordOK(ctx, n.ID)
}

func (s *Summarizer) recordError(ctx context.Context, nodeID string, msg string) {
	if s.ops == nil {
		return
	}
	now := s.nowMillis()
	s.statusMu.Lock()
	s.failureCount++
	count := s.failureCount
	s.statusMu.Unlock()
	_ = s.ops.Record(ctx, opsstatus.JobSummarizer, now, now, false, msg, map[string]any{
		"node_id":       nodeID,
		"failure_count": count,
	})
}

func (s *Summarizer) recordOK(ctx context.Context, nodeID string) {
	if s.ops == nil {
		return
	}
	now := s.nowMillis()
	s.statusMu.Lock()
	s.failureCount = 0
	s.statusMu.Unlock()
	_ = s.ops.Record(ctx, opsstatus.JobSummarizer, now, now, true, "", map[string]any{
		"node_id":       nodeID,
		"failure_count": 0,
	})
}

func (s *Summarizer) nowMillis() int64 {
	if s.now != nil {
		return s.now()
	}
	return timeNowMillis()
}

func timeNowMillis() int64 {
	return timeNow().UnixMilli()
}

var timeNow = func() time.Time { return time.Now() }

func runeLen(s string) int { return len([]rune(s)) }

func docToPlainText(raw string) string {
	if raw == "" {
		return ""
	}
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return ""
	}
	var sb strings.Builder
	walkDoc(v, &sb)
	return sb.String()
}

func walkDoc(v interface{}, sb *strings.Builder) {
	switch t := v.(type) {
	case map[string]interface{}:
		kind, _ := t["type"].(string)
		if kind == "mention" {
			attrs, _ := t["attrs"].(map[string]interface{})
			label, _ := attrs["label"].(string)
			if label != "" {
				sb.WriteString("@")
				sb.WriteString(label)
			}
			return
		}
		if kind == "text" {
			if s, ok := t["text"].(string); ok {
				sb.WriteString(s)
			}
			return
		}
		if content, ok := t["content"].([]interface{}); ok {
			for _, c := range content {
				walkDoc(c, sb)
			}
		}
		if kind == "paragraph" || kind == "heading" || kind == "blockquote" {
			sb.WriteString("\n")
		}
	case []interface{}:
		for _, c := range t {
			walkDoc(c, sb)
		}
	}
}
