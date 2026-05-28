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

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/tars/pkg/llm"
)

const queueSize = 256
const minRunesForLLM = 60
const containerSummaryMaxRunes = 4000
const maxSummarizeDepth = 6
const systemPrompt = "다음 한국어 본문을 3~5문장으로 요약하라. 등장인물·장소·핵심 사건은 반드시 보존하라. 새 정보 추가 금지."
const containerSystemPrompt = "다음은 한국어 소설의 하위 단위 요약들이다. 이 단위 전체를 3~5문장으로 요약하라. 등장인물·장소·핵심 사건은 반드시 보존하라. 새 정보 추가 금지."

type Summarizer struct {
	nodes   *node.Repo
	src     ai.ProviderSource
	factory ai.ClientFactory
	ch      chan string
}

func New(nodes *node.Repo, src ai.ProviderSource, factory ai.ClientFactory) *Summarizer {
	return &Summarizer{
		nodes: nodes, src: src, factory: factory,
		ch: make(chan string, queueSize),
	}
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

// summarizeOneDepth dispatches on node.Kind. Container nodes recurse into
// their children with depth+1; the depth cap prevents runaway recursion if
// the parent_id graph were ever to become cyclic.
func (s *Summarizer) summarizeOneDepth(ctx context.Context, nodeID string, depth int) {
	if depth > maxSummarizeDepth {
		fmt.Fprintf(os.Stderr, "summarizer: depth cap hit at %s (depth=%d)\n", nodeID, depth)
		return
	}
	n, err := s.nodes.Get(ctx, nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: get %s: %v\n", nodeID, err)
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
		}
		return
	}

	provider := s.src.Provider()
	client, err := s.factory(provider, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: factory(%s): %v\n", provider, err)
		return
	}

	msgs := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: plain},
	}
	resp, err := client.Chat(ctx, msgs, llm.ChatOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: Chat %s: %v\n", n.ID, err)
		return
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return
	}
	if err := s.nodes.SetSummary(ctx, n.ID, summary, capturedVersion); err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: SetSummary %s: %v\n", n.ID, err)
	}
}

// summarizeContainer rolls a container up from its children's Label+summary.
// Stale children are summarized first via a depth-first recursion.
func (s *Summarizer) summarizeContainer(ctx context.Context, n node.Node, depth int) {
	capturedVersion := n.ContentVersion
	children, err := s.nodes.ListChildren(ctx, n.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: ListChildren %s: %v\n", n.ID, err)
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

	provider := s.src.Provider()
	client, err := s.factory(provider, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: factory(%s): %v\n", provider, err)
		return
	}
	msgs := []llm.ChatMessage{
		{Role: "system", Content: containerSystemPrompt},
		{Role: "user", Content: input},
	}
	resp, err := client.Chat(ctx, msgs, llm.ChatOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: Chat (container) %s: %v\n", n.ID, err)
		return
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return
	}
	if err := s.nodes.SetSummary(ctx, n.ID, summary, capturedVersion); err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: SetSummary (container) %s: %v\n", n.ID, err)
	}
}

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
