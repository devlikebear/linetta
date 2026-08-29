package summarizer

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/opsstatus"
)

func TestLeadSummary_keepsShortTextWhole(t *testing.T) {
	got := leadSummary("비가 내렸다. 그는 우산을 펴지 않았다.")
	if got != "비가 내렸다. 그는 우산을 펴지 않았다." {
		t.Fatalf("lead = %q", got)
	}
}

func TestLeadSummary_cutsAtASentenceWhenOneIsInRange(t *testing.T) {
	// A sentence ends well inside the budget, so the lead should end there
	// rather than mid-word.
	first := strings.Repeat("가", 100) + "."
	text := first + strings.Repeat("나", 300)
	got := leadSummary(text)
	if got != first {
		t.Fatalf("lead did not cut at the sentence:\n got: %q", got)
	}
	if strings.HasSuffix(got, "…") {
		t.Fatalf("a clean sentence cut should not be marked truncated: %q", got)
	}
}

func TestLeadSummary_marksTruncationWhenNoSentenceFits(t *testing.T) {
	got := leadSummary(strings.Repeat("가", 500))
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated lead is not marked: %q", got)
	}
	if len([]rune(got)) > leadSummaryMaxRunes+1 {
		t.Fatalf("lead ran past the budget: %d runes", len([]rune(got)))
	}
}

func TestLeadSummary_collapsesWhitespaceSoTheBriefStaysOneLine(t *testing.T) {
	got := leadSummary("비가\n\n  내렸다.")
	if got != "비가 내렸다." {
		t.Fatalf("lead = %q", got)
	}
}

// The pivot removes the provider plumbing, so this is what every long scene
// gets from then on. A blank summary would cost an agent the brief it needs to
// write the next scene.
func TestSummarizer_withoutAProvider_writesTheSceneLead(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	body := "그는 문을 열었다. " + strings.Repeat("복도는 비어 있었다. ", 40)
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(body), 1100)

	sum := New(nodes)
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary != ""
	}, "lead lands without a provider")

	n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
	if !strings.HasPrefix(n.Summary, "그는 문을 열었다.") {
		t.Fatalf("summary is not the scene's opening: %q", n.Summary)
	}
	// Stamped against the current version, which is what stops the worker from
	// coming back and what lets an agent's real summary take precedence.
	if n.SummaryForVersion != n.ContentVersion {
		t.Fatalf("versions: summary_for=%d content=%d", n.SummaryForVersion, n.ContentVersion)
	}
}

// An agent that writes a real summary must not have it replaced by a lead on
// the next queue drain.
func TestSummarizer_leavesAnAuthoredSummaryAlone(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	nodeID := *p.LastOpenedNodeID
	_ = nodes.UpdateContent(ctx, nodeID, longDoc(strings.Repeat("가나다라마", 200)), 1100)

	n, _ := nodes.Get(ctx, nodeID)
	if err := nodes.SetSummary(ctx, nodeID, "에이전트가 쓴 요약.", n.ContentVersion); err != nil {
		t.Fatalf("SetSummary: %v", err)
	}

	sum := New(nodes)
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(nodeID)
	// Drain: enqueue a second node and wait for it, which means the first was
	// already processed by the single worker.
	waitFor(t, func() bool {
		got, _ := nodes.Get(ctx, nodeID)
		return got.Summary != ""
	}, "worker ran")

	got, _ := nodes.Get(ctx, nodeID)
	if got.Summary != "에이전트가 쓴 요약." {
		t.Fatalf("authored summary was replaced: %q", got.Summary)
	}
}

func TestSummarizer_withoutAProvider_reportsHealthy(t *testing.T) {
	st, nodes, p := newFixture(t)
	ctx := context.Background()
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(strings.Repeat("가나다라마", 200)), 1100)

	ops := opsstatus.NewRepo(st)
	sum := New(nodes).WithOpsStatus(ops, func() int64 { return 2000 })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary != ""
	}, "lead lands")

	// Falling back is normal operation, not a fault. A settings screen lit up
	// with a permanent summarizer error would be telling the writer that a
	// working feature is broken.
	waitFor(t, func() bool {
		statuses, err := ops.Get(context.Background())
		if err != nil {
			return false
		}
		for _, status := range statuses {
			if status.JobName == opsstatus.JobSummarizer {
				return status.LastOK
			}
		}
		return false
	}, "summarizer reports healthy after falling back")
}
