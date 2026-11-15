package mcphost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/continuity"
	"github.com/devlikebear/linetta/engine/internal/evidence"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/style"
)

// ---------- linetta_cite_sources ----------
//
// An answer about the manuscript names where it came from (#385). The agent
// passes the passages it relied on; each one is checked the way a continuity
// citation is (evidence.VerifyQuote) and only then handed back, numbered, for
// the reply to point at as [1], [2]. A quote the manuscript does not hold is
// refused, so a source the panel draws is one the writer can open and find.

// MaxCitations bounds one call. An answer leaning on more passages than this
// is a summary of the work, not a cited answer.
const MaxCitations = 6

type citationInput struct {
	NodeID        string `json:"node_id" jsonschema:"the scene"`
	ParagraphFrom int    `json:"paragraph_from"`
	ParagraphTo   int    `json:"paragraph_to,omitempty" jsonschema:"defaults to paragraph_from"`
	Quote         string `json:"quote" jsonschema:"copied exactly from those paragraphs"`
}

type citeSourcesInput struct {
	ProjectID string          `json:"project_id"`
	Citations []citationInput `json:"citations" jsonschema:"at most 6, in the order the reply cites them"`
}

func (in citeSourcesInput) scope() (string, string) { return in.ProjectID, "" }

// CitedSource is one checked citation. The agent passes it on to the panel
// unchanged (agent.citation mirrors these tags), so a source drawn under a
// reply carries exactly what this tool verified.
type CitedSource struct {
	// N is the number the reply cites it by: [1] is N=1.
	N      int    `json:"n"`
	NodeID string `json:"node_id"`
	// Breadcrumb is the scene's outline path in stored labels, e.g.
	// "1부 / 12화 / 씬 3"; the app translates each label for display.
	Breadcrumb    string `json:"breadcrumb"`
	SceneTitle    string `json:"scene_title,omitempty"`
	ParagraphFrom int    `json:"paragraph_from"`
	ParagraphTo   int    `json:"paragraph_to"`
	Quote         string `json:"quote"`
}

type citeSourcesOutput struct {
	Citations []CitedSource `json:"citations"`
}

func (d ToolDeps) registerCiteTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_cite_sources",
		Description: "Cite the passages an answer about the manuscript rests on: scene, paragraphs as " +
			"linetta_read_scene numbered=true numbers them, and a quote, checked against the text. Returns " +
			"them numbered; cite them in your reply as [1], [2].",
	}, record(d, "linetta_cite_sources", d.citeSources))
}

func (d ToolDeps) citeSources(ctx context.Context, _ *mcp.CallToolRequest, in citeSourcesInput) (*mcp.CallToolResult, citeSourcesOutput, error) {
	project, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, citeSourcesOutput{}, nil
	}
	switch n := len(in.Citations); {
	case n == 0:
		return toolErr("citations is empty; pass at least one passage, or answer without citing if the " +
			"manuscript does not say"), citeSourcesOutput{}, nil
	case n > MaxCitations:
		return toolErr("%d citations; at most %d per call — keep the passages the answer most depends on",
			n, MaxCitations), citeSourcesOutput{}, nil
	}

	flat, err := d.Nodes.ListByProject(ctx, project.ID)
	if err != nil {
		return nil, citeSourcesOutput{}, err
	}
	byID := make(map[string]node.Node, len(flat))
	for _, n := range flat {
		byID[n.ID] = n
	}

	var problems []string
	out := citeSourcesOutput{Citations: make([]CitedSource, 0, len(in.Citations))}
	for i, c := range in.Citations {
		num := i + 1
		src, problem := citeOne(byID, project.ID, num, c)
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		out.Citations = append(out.Citations, src)
	}
	if len(problems) > 0 {
		// Every problem at once, as linetta_report_continuity refuses, so the
		// next call fixes them all.
		return toolErr("%s", strings.Join(problems, "\n")), citeSourcesOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: citedText(out.Citations)}}}, out, nil
}

// citeOne checks one citation, returning it numbered or the sentence that
// says how to fix it.
func citeOne(byID map[string]node.Node, projectID string, num int, c citationInput) (CitedSource, string) {
	nodeID := strings.TrimSpace(c.NodeID)
	if nodeID == "" {
		return CitedSource{}, fmt.Sprintf("citation %d: node_id is required", num)
	}
	n, ok := byID[nodeID]
	if !ok {
		return CitedSource{}, fmt.Sprintf("citation %d: scene %q is not in work %q; find it with linetta_get_outline "+
			"or linetta_search_manuscript", num, nodeID, projectID)
	}
	if n.Kind != node.KindLeaf {
		return CitedSource{}, fmt.Sprintf("citation %d: node %q is a container (%s), not a scene; cite the scene "+
			"the words are in", num, n.ID, n.Label)
	}
	quote := strings.TrimSpace(c.Quote)
	if quote == "" {
		return CitedSource{}, fmt.Sprintf("citation %d: quote is required; copy it exactly from the numbered text", num)
	}
	if runes := utf8.RuneCountInString(quote); runes > continuity.MaxQuoteRunes {
		return CitedSource{}, fmt.Sprintf("citation %d: quote is %d characters; the limit is %d — quote the words "+
			"that matter", num, runes, continuity.MaxQuoteRunes)
	}
	to := c.ParagraphTo
	if to == 0 {
		to = c.ParagraphFrom
	}
	paragraphs := []string{}
	if n.ContentDoc != nil {
		paragraphs = style.Paragraphs(*n.ContentDoc)
	}
	if err := evidence.VerifyQuote(paragraphs, c.ParagraphFrom, to, quote); err != nil {
		return CitedSource{}, citationProblem(num, n.ID, err)
	}
	return CitedSource{
		N:             num,
		NodeID:        n.ID,
		Breadcrumb:    node.BreadcrumbLabel(byID, n),
		SceneTitle:    n.Title,
		ParagraphFrom: c.ParagraphFrom,
		ParagraphTo:   to,
		Quote:         quote,
	}, ""
}

func citationProblem(num int, nodeID string, err error) string {
	var ee *evidence.Error
	if !errors.As(err, &ee) {
		return fmt.Sprintf("citation %d: %v", num, err)
	}
	switch ee.Kind {
	case evidence.FromTooSmall:
		return fmt.Sprintf("citation %d: paragraph_from %d must be at least 1", num, ee.From)
	case evidence.ToBeforeFrom:
		return fmt.Sprintf("citation %d: paragraph_to %d is before paragraph_from %d", num, ee.To, ee.From)
	case evidence.OutOfRange:
		return fmt.Sprintf("citation %d: paragraph %d is out of range; scene %q has %d paragraphs", num, ee.To, nodeID, ee.N)
	default:
		return fmt.Sprintf("citation %d: quote not found in %s of scene %q; read it with linetta_read_scene "+
			"numbered=true and copy the words exactly", num, paragraphsPhrase(ee.From, ee.To), nodeID)
	}
}

func paragraphsPhrase(from, to int) string {
	if from == to {
		return fmt.Sprintf("paragraph %d", from)
	}
	return fmt.Sprintf("paragraphs %d-%d", from, to)
}

// citedText is what the model, and an external client that has no panel to
// draw cards in, reads back: the verified list, numbered.
func citedText(sources []CitedSource) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d sources verified. Point at them in your reply as [n].\n", len(sources))
	for _, s := range sources {
		where := s.Breadcrumb
		if s.SceneTitle != "" {
			where += " (" + s.SceneTitle + ")"
		}
		fmt.Fprintf(&b, "[%d] %s, %s: %q\n", s.N, where, paragraphsPhrase(s.ParagraphFrom, s.ParagraphTo), s.Quote)
	}
	return strings.TrimRight(b.String(), "\n")
}
