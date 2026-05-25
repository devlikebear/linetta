package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/internal/llm"
	"github.com/devlikebear/linetta/internal/memory"
	"github.com/devlikebear/linetta/internal/work"
)

// extractedProposal mirrors the JSON shape the Canon Keeper extractor LLM
// returns for each new-canon proposal.
type extractedProposal struct {
	Kind       string  `json:"kind"`
	Title      string  `json:"title"`
	Body       string  `json:"body"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

// extractedIssue mirrors the JSON shape for each continuity-issue flag.
type extractedIssue struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

type reviewExtraction struct {
	Proposals []extractedProposal `json:"proposals"`
	Issues    []extractedIssue    `json:"issues"`
}

// extractReviewWithLLM asks the LLM to scan the edited draft + canon review
// against the existing Canon and produce structured proposals/issues. Returns
// (props, issues, true) on success; (_, _, false) if the LLM is unavailable
// or returns malformed output. Caller falls back to the deterministic stub.
func extractReviewWithLLM(
	ctx context.Context,
	provider llm.Provider,
	workItem work.Work,
	episode work.Episode,
	blueprint work.EpisodeBlueprint,
	canonItems []memory.Item,
	canonReview string,
	editedDraft string,
) ([]extractedProposal, []extractedIssue, bool) {
	if provider == nil {
		return nil, nil, false
	}
	if strings.TrimSpace(editedDraft) == "" && strings.TrimSpace(canonReview) == "" {
		return nil, nil, false
	}

	sys := `You are the "Canon Keeper" extractor. Compare an edited episode draft against the writer's existing Canon memory.
Return ONLY JSON in this exact shape:
{
  "proposals": [
    {
      "kind": "character|world_fact|timeline_event|plot_thread|style_rule|source",
      "title": "short name of the entity or rule",
      "body": "the canonical text the user can accept as-is — full sentence(s)",
      "reason": "why this should be added to Canon",
      "confidence": 0.0
    }
  ],
  "issues": [
    {
      "severity": "info|warning|blocker",
      "title": "concise issue label",
      "body": "what contradicts and which canon item it conflicts with"
    }
  ]
}

Rules:
- Propose ONLY entities that are *not already in the provided Canon* (match by title, case-insensitive).
- Flag issues when the draft contradicts an existing Canon item — cite the item by title in body.
- Be selective: prefer 1-5 proposals and 0-3 issues. Quality over quantity.
- If nothing meaningful is found, return {"proposals": [], "issues": []}.
- Respond in the language of the input.`

	var canon strings.Builder
	if len(canonItems) == 0 {
		canon.WriteString("  (none)\n")
	} else {
		for _, it := range canonItems {
			body := strings.TrimSpace(it.Body)
			if len(body) > 160 {
				body = body[:160] + "…"
			}
			fmt.Fprintf(&canon, "  - [%s] %s — %s\n", it.Kind, it.Title, body)
		}
	}

	user := fmt.Sprintf(`Work: %s (%s)
Episode: %s
Blueprint premise: %s

Canon items currently on record:
%s
Canon Keeper agent's plain-text review:
%s

Edited draft:
%s
`,
		workItem.Title, workItem.Genre,
		episode.Title,
		blueprint.Premise,
		canon.String(),
		safeOr(canonReview, "(none)"),
		safeOr(editedDraft, "(none)"),
	)

	var out reviewExtraction
	if err := provider.ChatJSON(ctx, []llm.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: user},
	}, 0.3, &out); err != nil {
		return nil, nil, false
	}
	// Drop anything that already matches a canon item by case-insensitive title.
	out.Proposals = dedupeAgainstCanon(out.Proposals, canonItems)
	return out.Proposals, out.Issues, true
}

func dedupeAgainstCanon(proposals []extractedProposal, canonItems []memory.Item) []extractedProposal {
	existing := make(map[string]struct{}, len(canonItems))
	for _, it := range canonItems {
		existing[strings.ToLower(strings.TrimSpace(it.Title))] = struct{}{}
	}
	out := proposals[:0]
	for _, p := range proposals {
		key := strings.ToLower(strings.TrimSpace(p.Title))
		if key == "" {
			continue
		}
		if _, dup := existing[key]; dup {
			continue
		}
		out = append(out, p)
	}
	return out
}

func mapProposalKind(s string) memory.Kind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "character":
		return memory.KindCharacter
	case "world_fact", "world", "world-fact":
		return memory.KindWorldFact
	case "timeline_event", "timeline":
		return memory.KindTimelineEvent
	case "plot_thread", "plot-thread", "plot":
		return memory.KindPlotThread
	case "style_rule", "style":
		return memory.KindStyleRule
	case "source":
		return memory.KindSource
	default:
		return memory.KindPlotThread
	}
}

func mapIssueSeverity(s string) memory.IssueSeverity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "info":
		return memory.IssueInfo
	case "blocker":
		return memory.IssueBlocker
	default:
		return memory.IssueWarning
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func clamp01(v, fallback float64) float64 {
	if v <= 0 || v > 1 {
		return fallback
	}
	return v
}
