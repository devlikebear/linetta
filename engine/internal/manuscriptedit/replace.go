package manuscriptedit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
)

var (
	ErrInvalidRequest = errors.New("invalid replace request")
	ErrInvalidPlan    = errors.New("invalid replace plan")
)

const (
	FailureVersionMismatch = "version_mismatch"
	FailureSnapshot        = "snapshot_failed"
	FailureUpdate          = "update_failed"
	FailureNotFound        = "candidate_not_found"
)

type ReplacePlanRequest struct {
	ProjectID     string   `json:"project_id"`
	Query         string   `json:"query"`
	Replacement   string   `json:"replacement"`
	NodeIDs       []string `json:"node_ids,omitempty"`
	MatchCase     bool     `json:"match_case,omitempty"`
	WholeWord     bool     `json:"whole_word,omitempty"`
	MaxCandidates int      `json:"max_candidates,omitempty"`
}

type ReplaceCandidate struct {
	ID             string `json:"id"`
	NodeID         string `json:"node_id"`
	Breadcrumb     string `json:"breadcrumb"`
	Before         string `json:"before"`
	After          string `json:"after"`
	Occurrences    int    `json:"occurrences"`
	Selected       bool   `json:"selected"`
	PreviewVersion int    `json:"preview_version"`
}

type ReplacePlan struct {
	ID            string             `json:"id,omitempty"`
	ProjectID     string             `json:"project_id"`
	Query         string             `json:"query"`
	Replacement   string             `json:"replacement"`
	Candidates    []ReplaceCandidate `json:"candidates"`
	GeneratedAt   int64              `json:"generated_at,omitempty"`
	MatchCase     bool               `json:"match_case,omitempty"`
	WholeWord     bool               `json:"whole_word,omitempty"`
	MaxCandidates int                `json:"max_candidates,omitempty"`
}

type ApplyFailure struct {
	CandidateID string `json:"candidate_id"`
	NodeID      string `json:"node_id,omitempty"`
	Breadcrumb  string `json:"breadcrumb,omitempty"`
	Reason      string `json:"reason"`
	Message     string `json:"message"`
}

type ApplyReplaceResult struct {
	Applied        int            `json:"applied"`
	Skipped        int            `json:"skipped"`
	Failures       []ApplyFailure `json:"failures"`
	ChangedNodeIDs []string       `json:"changed_node_ids"`
	// SnapshotIDs maps each changed node to the snapshot taken just before
	// its edit, so a caller (an MCP agent especially, #73) can restore any
	// one scene without hunting through version history.
	SnapshotIDs map[string]string `json:"snapshot_ids,omitempty"`
}

type nodeRepo interface {
	Get(ctx context.Context, id string) (node.Node, error)
	ListByProject(ctx context.Context, projectID string) ([]node.Node, error)
	UpdateContent(ctx context.Context, id string, doc string, now int64) error
	UpdateContentIfVersion(ctx context.Context, id string, doc string, expectedVersion int, now int64) error
}

type snapshotRepo interface {
	Create(ctx context.Context, nodeID, doc, reason string, now int64) (snapshot.Snapshot, error)
}

type Service struct {
	nodes nodeRepo
	snaps snapshotRepo
}

func NewService(nodes nodeRepo, snaps snapshotRepo) *Service {
	return &Service{nodes: nodes, snaps: snaps}
}

func (s *Service) PlanReplace(ctx context.Context, req ReplacePlanRequest) (ReplacePlan, error) {
	projectID := strings.TrimSpace(req.ProjectID)
	// Not trimmed: a space at the end of a query is part of what is replaced,
	// and trimming only the query would leave the replacement's own space
	// doubled against the one still in the scene.
	query := req.Query
	if s == nil || s.nodes == nil || projectID == "" || strings.TrimSpace(query) == "" || strings.TrimSpace(req.Replacement) == "" {
		return ReplacePlan{}, ErrInvalidRequest
	}
	if req.WholeWord {
		return ReplacePlan{}, fmt.Errorf("%w: whole_word is not supported yet", ErrInvalidRequest)
	}

	nodes, err := s.candidateNodes(ctx, projectID, req.NodeIDs)
	if err != nil {
		return ReplacePlan{}, err
	}
	byID := map[string]node.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}

	plan := ReplacePlan{
		ID:            planID(query, req.Replacement),
		ProjectID:     projectID,
		Query:         query,
		Replacement:   req.Replacement,
		MatchCase:     req.MatchCase,
		WholeWord:     req.WholeWord,
		MaxCandidates: req.MaxCandidates,
		Candidates:    []ReplaceCandidate{},
	}
	for _, n := range nodes {
		if n.Kind != node.KindLeaf || n.ContentDoc == nil {
			continue
		}
		result, err := replaceDocText(*n.ContentDoc, query, req.Replacement, req.MatchCase)
		if err != nil || result.Occurrences == 0 {
			continue
		}
		c := ReplaceCandidate{
			ID:             candidateID(n.ID, n.ContentVersion),
			NodeID:         n.ID,
			Breadcrumb:     node.BreadcrumbLabel(byID, n),
			Before:         result.BeforePlain,
			After:          result.AfterPlain,
			Occurrences:    result.Occurrences,
			Selected:       true,
			PreviewVersion: n.ContentVersion,
		}
		plan.Candidates = append(plan.Candidates, c)
		if req.MaxCandidates > 0 && len(plan.Candidates) >= req.MaxCandidates {
			break
		}
	}
	return plan, nil
}

func (s *Service) ApplyReplace(ctx context.Context, plan ReplacePlan, candidateIDs []string, now int64) (ApplyReplaceResult, error) {
	if s == nil || s.nodes == nil || s.snaps == nil || strings.TrimSpace(plan.ProjectID) == "" ||
		strings.TrimSpace(plan.Query) == "" || strings.TrimSpace(plan.Replacement) == "" {
		return ApplyReplaceResult{}, ErrInvalidPlan
	}
	selected := map[string]bool{}
	for _, id := range candidateIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			selected[id] = true
		}
	}
	if len(selected) == 0 {
		return ApplyReplaceResult{Skipped: len(plan.Candidates), Failures: []ApplyFailure{}, ChangedNodeIDs: []string{}}, nil
	}

	result := ApplyReplaceResult{Failures: []ApplyFailure{}, ChangedNodeIDs: []string{}}
	for _, c := range plan.Candidates {
		if !selected[c.ID] {
			result.Skipped++
			continue
		}
		n, err := s.nodes.Get(ctx, c.NodeID)
		if err != nil {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureNotFound,
				Message:     err.Error(),
			})
			continue
		}
		if n.ProjectID != plan.ProjectID {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureNotFound,
				Message:     "node does not belong to replace plan project",
			})
			continue
		}
		if n.ContentVersion != c.PreviewVersion {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureVersionMismatch,
				Message:     "scene changed after preview; refresh preview",
			})
			continue
		}
		if n.ContentDoc == nil {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureNotFound,
				Message:     "scene has no content",
			})
			continue
		}
		replaced, err := replaceDocText(*n.ContentDoc, plan.Query, plan.Replacement, plan.MatchCase)
		if err != nil || replaced.Occurrences == 0 {
			msg := "candidate no longer matches"
			if err != nil {
				msg = err.Error()
			}
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureNotFound,
				Message:     msg,
			})
			continue
		}
		snap, err := s.snaps.Create(ctx, n.ID, *n.ContentDoc, snapshot.ReasonManual, now)
		if err != nil {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureSnapshot,
				Message:     err.Error(),
			})
			continue
		}
		if err := s.nodes.UpdateContentIfVersion(ctx, n.ID, replaced.Doc, c.PreviewVersion, now); err != nil {
			reason := FailureUpdate
			message := err.Error()
			if errors.Is(err, node.ErrContentConflict) {
				// A save landed between our version check above and this
				// write (#107); report it the same way as the read-time
				// mismatch so callers see one consistent conflict signal.
				reason = FailureVersionMismatch
				message = "scene changed after preview; refresh preview"
			}
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      reason,
				Message:     message,
			})
			continue
		}
		result.Applied++
		result.ChangedNodeIDs = append(result.ChangedNodeIDs, n.ID)
		if result.SnapshotIDs == nil {
			result.SnapshotIDs = map[string]string{}
		}
		result.SnapshotIDs[n.ID] = snap.ID
	}
	return result, nil
}

func (s *Service) candidateNodes(ctx context.Context, projectID string, nodeIDs []string) ([]node.Node, error) {
	if len(nodeIDs) == 0 {
		return s.nodes.ListByProject(ctx, projectID)
	}
	out := make([]node.Node, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		n, err := s.nodes.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if n.ProjectID == projectID {
			out = append(out, n)
		}
	}
	return out, nil
}

func candidateID(nodeID string, contentVersion int) string {
	return nodeID + ":" + strconv.Itoa(contentVersion)
}

func planID(query string, replacement string) string {
	return "replace:" + strconv.QuoteToASCII(query) + "->" + strconv.QuoteToASCII(replacement)
}

type docReplaceResult struct {
	Doc         string
	BeforePlain string
	AfterPlain  string
	Occurrences int
}

func replaceDocText(raw string, query string, replacement string, matchCase bool) (docReplaceResult, error) {
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return docReplaceResult{}, err
	}
	beforePlain := plainText(doc)
	occurrences := replaceTextNodes(doc, query, replacement, matchCase)
	if occurrences == 0 {
		return docReplaceResult{Doc: raw, BeforePlain: beforePlain, AfterPlain: beforePlain}, nil
	}
	afterPlain := plainText(doc)
	encoded, err := json.Marshal(doc)
	if err != nil {
		return docReplaceResult{}, err
	}
	return docReplaceResult{
		Doc:         string(encoded),
		BeforePlain: beforePlain,
		AfterPlain:  afterPlain,
		Occurrences: occurrences,
	}, nil
}

// replaceTextNodes replaces query throughout the block tree under v. A match
// may cross formatting, linked names and paragraph breaks, because that is how
// the text reads; see replaceInBlocks for what it will not touch.
func replaceTextNodes(v any, query string, replacement string, matchCase bool) int {
	t, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	content, ok := t["content"].([]any)
	if !ok {
		return 0
	}
	next, count := replaceInBlocks(content, query, replacement, matchCase)
	t["content"] = next
	return count
}

// isTextBlock reports whether a block holds inline content directly. Runs of
// these are matched as one stretch of text.
func isTextBlock(block map[string]any) bool {
	kind, _ := block["type"].(string)
	return kind == "paragraph" || kind == "heading"
}

// replaceInBlocks handles one list of sibling blocks: each run of adjacent text
// blocks as a unit, and anything else (a quote, a list) by descending into it.
// A block that is neither, such as an illustration, ends the run, so a match
// never swallows a picture.
func replaceInBlocks(blocks []any, query, replacement string, matchCase bool) ([]any, int) {
	out := make([]any, 0, len(blocks))
	count := 0
	var run []map[string]any
	flush := func() {
		replaced, n := replaceInRun(run, query, replacement, matchCase)
		for _, block := range replaced {
			out = append(out, block)
		}
		count += n
		run = nil
	}
	for _, child := range blocks {
		block, ok := child.(map[string]any)
		if ok && isTextBlock(block) {
			run = append(run, block)
			continue
		}
		flush()
		if ok {
			count += replaceTextNodes(block, query, replacement, matchCase)
		}
		out = append(out, child)
	}
	flush()
	return out, count
}

// runText is a run of text blocks read as one string. A mention reads as its
// label and a paragraph break as a newline. Anything else inline, a hard break
// say, reads as a rune no query contains.
type runText struct {
	runes []rune
	// block and offset say where each rune came from: which block of the run,
	// and how many runes into that block's inline content.
	block, offset []int
	atoms         []atom
}

// atom is a mention's span in runText.
type atom struct {
	from, to int
	node     map[string]any
}

const unmatchableRune = '\uFFFF'

func readRun(run []map[string]any) runText {
	var rt runText
	add := func(r rune, block, offset int) {
		rt.runes = append(rt.runes, r)
		rt.block = append(rt.block, block)
		rt.offset = append(rt.offset, offset)
	}
	for i, block := range run {
		content, _ := block["content"].([]any)
		if len(content) == 0 {
			continue
		}
		if len(rt.runes) > 0 {
			add('\n', i, 0)
		}
		offset := 0
		for _, child := range content {
			inline, _ := child.(map[string]any)
			text, isMention := inlineText(inline)
			from := len(rt.runes)
			for _, r := range []rune(text) {
				add(r, i, offset)
				offset++
			}
			if isMention {
				rt.atoms = append(rt.atoms, atom{from: from, to: len(rt.runes), node: inline})
			}
		}
	}
	return rt
}

// inlineText is what an inline node reads as, and whether it is a mention.
func inlineText(inline map[string]any) (string, bool) {
	switch inline["type"] {
	case "text":
		text, _ := inline["text"].(string)
		return text, false
	case "mention":
		attrs, _ := inline["attrs"].(map[string]any)
		label, _ := attrs["label"].(string)
		return label, true
	default:
		return string(unmatchableRune), false
	}
}

// normalizeForRun brings text copied from a scene to the form runText reads:
// read_scene writes a mention as @label, and paragraph breaks may be one
// newline or a blank line.
func normalizeForRun(text string, atoms []atom) string {
	for _, a := range atoms {
		attrs, _ := a.node["attrs"].(map[string]any)
		if label, _ := attrs["label"].(string); label != "" {
			text = strings.ReplaceAll(text, "@"+label, label)
		}
	}
	return paragraphBreak.ReplaceAllString(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

var paragraphBreak = regexp.MustCompile(`[ \t]*\n\s*`)

func replaceInRun(run []map[string]any, query, replacement string, matchCase bool) ([]map[string]any, int) {
	if len(run) == 0 {
		return run, 0
	}
	rt := readRun(run)
	needle := []rune(normalizeForRun(query, rt.atoms))
	if len(needle) == 0 {
		return run, 0
	}
	matches := findMatches(rt, needle, matchCase)
	replacement = normalizeForRun(replacement, rt.atoms)
	// Last to first: what a replacement changes lies at or after its own start,
	// so the positions of the matches still to come stay true.
	for i := len(matches) - 1; i >= 0; i-- {
		run = applyMatch(run, rt, matches[i], replacement)
	}
	return run, len(matches)
}

type span struct{ from, to int }

// findMatches returns non-overlapping matches, first to last. Linked names stay
// whole: a match may contain a mention, but one that is only a mention, or that
// cuts one in two, is left alone. That is what lets a rename sweep the prose
// without unlinking the name it is renaming.
func findMatches(rt runText, needle []rune, matchCase bool) []span {
	var out []span
	for at := 0; at+len(needle) <= len(rt.runes); {
		if !runesMatch(rt.runes[at:at+len(needle)], needle, matchCase) {
			at++
			continue
		}
		m := span{at, at + len(needle)}
		if !keepsAtomsWhole(rt, m) {
			at++
			continue
		}
		out = append(out, m)
		at = m.to
	}
	return out
}

func runesMatch(text, needle []rune, matchCase bool) bool {
	for i, r := range needle {
		if text[i] == r || (!matchCase && unicode.ToLower(text[i]) == unicode.ToLower(r)) {
			continue
		}
		return false
	}
	return true
}

func keepsAtomsWhole(rt runText, m span) bool {
	for _, a := range rt.atoms {
		if a.to <= m.from || a.from >= m.to {
			continue
		}
		if a.from < m.from || a.to > m.to {
			return false
		}
		if a.from == m.from && a.to == m.to {
			return false
		}
	}
	return true
}

// applyMatch rewrites the blocks a match covers. The first keeps what came
// before the match, the last keeps what came after, the ones between go, and
// each newline in the replacement starts a paragraph.
func applyMatch(run []map[string]any, rt runText, m span, replacement string) []map[string]any {
	first, last := rt.block[m.from], rt.block[m.to-1]
	firstContent, _ := run[first]["content"].([]any)
	lastContent, _ := run[last]["content"].([]any)

	before, matched := splitInline(firstContent, rt.offset[m.from])
	_, after := splitInline(lastContent, rt.offset[m.to-1]+1)

	var relink []map[string]any
	for _, a := range rt.atoms {
		if a.from >= m.from && a.to <= m.to {
			relink = append(relink, a.node)
		}
	}
	marks := firstMarks(matched)
	lines := strings.Split(replacement, "\n")

	out := make([]map[string]any, 0, len(run)+len(lines))
	out = append(out, run[:first]...)
	if len(lines) == 1 {
		out = append(out, withContent(run[first], concatInline(before, inlineFor(lines[0], relink, marks), after)))
	} else {
		out = append(out, withContent(run[first], concatInline(before, inlineFor(lines[0], relink, marks))))
		for _, line := range lines[1 : len(lines)-1] {
			out = append(out, paragraphOf(inlineFor(line, relink, marks)))
		}
		out = append(out, paragraphOf(concatInline(inlineFor(lines[len(lines)-1], relink, marks), after)))
	}
	return append(out, run[last+1:]...)
}

// splitInline cuts inline content at a rune offset, copying a text node the cut
// falls inside. Offsets never fall inside a mention; findMatches saw to that.
func splitInline(content []any, offset int) (left, right []any) {
	for _, child := range content {
		inline, _ := child.(map[string]any)
		text, _ := inlineText(inline)
		runes := []rune(text)
		switch {
		case offset <= 0:
			right = append(right, child)
		case offset >= len(runes):
			left = append(left, child)
		default:
			left = append(left, textLike(inline, string(runes[:offset])))
			right = append(right, textLike(inline, string(runes[offset:])))
		}
		offset -= len(runes)
	}
	return left, right
}

func textLike(inline map[string]any, text string) map[string]any {
	out := map[string]any{"type": "text", "text": text}
	if marks, ok := inline["marks"]; ok {
		out["marks"] = marks
	}
	return out
}

// firstMarks is the formatting of the text a replacement takes over.
func firstMarks(content []any) any {
	for _, child := range content {
		inline, _ := child.(map[string]any)
		if inline["type"] == "text" {
			return inline["marks"]
		}
	}
	return nil
}

// inlineFor turns replacement text into inline nodes. A linked name the match
// removed goes back as the same mention wherever the replacement still says it,
// so rewording around a name does not unlink it.
func inlineFor(line string, relink []map[string]any, marks any) []any {
	var out []any
	flush := func(text string) {
		if text == "" {
			return
		}
		node := map[string]any{"type": "text", "text": text}
		if marks != nil {
			node["marks"] = marks
		}
		out = append(out, node)
	}
	cursor := 0
	for cursor < len(line) {
		at, atom := nextLabel(line[cursor:], relink)
		if atom == nil {
			break
		}
		flush(line[cursor : cursor+at])
		out = append(out, atom)
		attrs, _ := atom["attrs"].(map[string]any)
		label, _ := attrs["label"].(string)
		cursor += at + len(label)
	}
	flush(line[cursor:])
	return out
}

// nextLabel finds the earliest label in text, the longest where two start
// together.
func nextLabel(text string, relink []map[string]any) (int, map[string]any) {
	best, bestLen := -1, 0
	var found map[string]any
	for _, node := range relink {
		attrs, _ := node["attrs"].(map[string]any)
		label, _ := attrs["label"].(string)
		if label == "" {
			continue
		}
		at := strings.Index(text, label)
		if at < 0 {
			continue
		}
		if best < 0 || at < best || (at == best && len(label) > bestLen) {
			best, bestLen, found = at, len(label), node
		}
	}
	return best, found
}

// concatInline joins inline runs, folding neighbouring text with the same
// formatting back into one node.
func concatInline(parts ...[]any) []any {
	var out []any
	for _, part := range parts {
		for _, child := range part {
			inline, _ := child.(map[string]any)
			if len(out) > 0 && inline["type"] == "text" {
				prev, _ := out[len(out)-1].(map[string]any)
				if prev["type"] == "text" && reflect.DeepEqual(prev["marks"], inline["marks"]) {
					merged := textLike(prev, prev["text"].(string)+inline["text"].(string))
					out[len(out)-1] = merged
					continue
				}
			}
			out = append(out, child)
		}
	}
	return out
}

func withContent(block map[string]any, content []any) map[string]any {
	out := make(map[string]any, len(block))
	for k, v := range block {
		out[k] = v
	}
	if len(content) == 0 {
		delete(out, "content")
	} else {
		out["content"] = content
	}
	return out
}

func paragraphOf(content []any) map[string]any {
	return withContent(map[string]any{"type": "paragraph"}, content)
}

func plainText(v any) string {
	var sb strings.Builder
	writePlain(v, &sb)
	return sb.String()
}

func writePlain(v any, sb *strings.Builder) {
	switch t := v.(type) {
	case map[string]any:
		kind, _ := t["type"].(string)
		if kind == "mention" {
			attrs, _ := t["attrs"].(map[string]any)
			if label, _ := attrs["label"].(string); label != "" {
				sb.WriteString("@")
				sb.WriteString(label)
			}
			return
		}
		if kind == "text" {
			if text, ok := t["text"].(string); ok {
				sb.WriteString(text)
			}
			return
		}
		if content, ok := t["content"].([]any); ok {
			for _, child := range content {
				writePlain(child, sb)
			}
		}
		if kind == "paragraph" || kind == "heading" || kind == "blockquote" {
			sb.WriteString("\n")
		}
	case []any:
		for _, child := range t {
			writePlain(child, sb)
		}
	}
}
