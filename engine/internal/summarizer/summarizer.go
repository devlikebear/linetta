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
const systemPrompt = "다음 한국어 본문을 3~5문장으로 요약하라. 등장인물·장소·핵심 사건은 반드시 보존하라. 새 정보 추가 금지."

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

func (s *Summarizer) summarizeOne(ctx context.Context, nodeID string) {
	n, err := s.nodes.Get(ctx, nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: get %s: %v\n", nodeID, err)
		return
	}
	if n.Kind != "leaf" || n.ContentDoc == nil {
		return
	}
	if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
		return
	}

	capturedVersion := n.ContentVersion
	plain := strings.TrimSpace(docToPlainText(*n.ContentDoc))
	if plain == "" {
		return
	}

	if runeLen(plain) < minRunesForLLM {
		if err := s.nodes.SetSummary(ctx, nodeID, plain, capturedVersion); err != nil {
			fmt.Fprintf(os.Stderr, "summarizer: SetSummary (short) %s: %v\n", nodeID, err)
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
		fmt.Fprintf(os.Stderr, "summarizer: Chat %s: %v\n", nodeID, err)
		return
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return
	}
	if err := s.nodes.SetSummary(ctx, nodeID, summary, capturedVersion); err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: SetSummary %s: %v\n", nodeID, err)
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
