package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/pkg/llm"
)

// SettlementLedger is what the agent needs of the settlement record. An
// interface so this package reads no store; ledger.Settlements satisfies it.
type SettlementLedger interface {
	// Version is a scene's content version now.
	Version(ctx context.Context, nodeID string) (int, error)
	// Record keeps how the pass over one scene at version ended.
	Record(ctx context.Context, projectID, nodeID string, version int, outcome string, now int64) error
	// Pending lists, in story order, the scenes before beforeNodeID still
	// waiting for a settlement. Empty while settlement is off.
	Pending(ctx context.Context, projectID, beforeNodeID string) ([]string, error)
}

// settle runs the settlement pass over scenes and records, scene by scene, how
// it ended against the version each scene had when the pass began: prose the
// writer changes while the pass runs is a new version, still waiting.
func (s *Service) settle(ctx context.Context, st loopState, name string, scenes []string) string {
	versions := map[string]int{}
	if s.deps.Settlements != nil {
		for _, id := range scenes {
			v, err := s.deps.Settlements.Version(ctx, id)
			if err != nil {
				logf("%s after turn %s: version of %s: %v", name, st.runID, id, err)
				continue
			}
			versions[id] = v
		}
	}
	outcome := s.runPass(ctx, st, name, []llm.ChatMessage{
		{Role: "system", Content: settlementSystemPrompt},
		{Role: "user", Content: settlementRequest(st.req.ProjectID, scenes)},
	}, settlementTools, settlementCallsPerScene*len(scenes))
	if s.deps.Settlements == nil {
		return outcome
	}
	now := s.now()
	for _, id := range scenes {
		v, ok := versions[id]
		if !ok {
			continue
		}
		// Recorded even when the pass was cut off: the record is what says it
		// was, and a cancelled context must not lose it.
		if err := s.deps.Settlements.Record(context.WithoutCancel(ctx), st.req.ProjectID, id, v, outcome, now); err != nil {
			logf("%s after turn %s: recording %s: %v", name, st.runID, id, err)
		}
	}
	return outcome
}

// A turn that writes after scenes the ledger has not caught up with settles
// them first. Refusing the write instead would be a refusal the agent cannot
// act on: settlement is a pass, not a tool it can call. So the turn's first
// prose write — once both gates let it through — catches up, oldest first and
// within a short budget, and goes ahead whatever happens; the scenes still
// waiting are named in a line on that write's result. A turn that writes no
// prose — a question, a world edit — never waits on a settlement.
const (
	catchUpMaxScenes = 3
	catchUpTimeout   = 2 * time.Minute
	// catchUpNoticeMax caps how many waiting scenes the notice names.
	catchUpNoticeMax = 10
)

// catchUpOnce lets one prose write per turn run the catch-up. The mutex is
// for the CLI bridge, whose calls arrive on HTTP handlers.
type catchUpOnce struct {
	mu   sync.Mutex
	done bool
}

// first reports whether this is the turn's first call to ask, and marks it.
func (c *catchUpOnce) first() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return false
	}
	c.done = true
	return true
}

// catchUp settles what is waiting before target, the scene the write is
// about, and returns the notice for what is still waiting, or "". A writer
// who has settlement off pays for no call they did not switch on.
func (s *Service) catchUp(ctx context.Context, st loopState, target string) string {
	target = strings.TrimSpace(target)
	if target == "" || s.deps.Settlements == nil || s.deps.SettlementEnabled == nil || !s.deps.SettlementEnabled() ||
		!offers(st.schemas, "linetta_edit_ledger") {
		return ""
	}
	pending, err := s.deps.Settlements.Pending(ctx, st.req.ProjectID, target)
	if err != nil {
		logf("settlement catch-up for turn %s: %v", st.runID, err)
		return ""
	}
	if len(pending) == 0 {
		return ""
	}
	batch := pending[:min(len(pending), catchUpMaxScenes)]
	passCtx, cancel := context.WithTimeout(ctx, catchUpTimeout)
	started := time.Now()
	outcome := s.settle(passCtx, st, "settlement catch-up", batch)
	cancel()
	left, err := s.deps.Settlements.Pending(context.WithoutCancel(ctx), st.req.ProjectID, target)
	if err != nil {
		logf("settlement catch-up for turn %s: %v", st.runID, err)
		left = pending
	}
	s.recordGate(ctx, GateEvent{
		Kind: SettlementCatchUp, ProjectID: st.projectID, NodeID: batch[0], RunID: st.runID, OK: outcome == passDone,
		Detail: fmt.Sprintf("outcome=%s scenes=%d left=%d ms=%d", outcome, len(batch), len(left), time.Since(started).Milliseconds()),
	})
	return s.pendingNotice(ctx, left)
}

// pendingNotice names the scenes still waiting, by the labels the writer
// knows them by, the way the brief names the sections it left out.
func (s *Service) pendingNotice(ctx context.Context, ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	labels := make([]string, 0, min(len(ids), catchUpNoticeMax))
	for _, id := range ids[:min(len(ids), catchUpNoticeMax)] {
		label := ""
		if s.deps.Scope != nil {
			label = s.deps.Scope.NodeLabel(ctx, id)
		}
		if label == "" {
			label = id
		}
		labels = append(labels, label)
	}
	more := ""
	if n := len(ids) - len(labels); n > 0 {
		more = fmt.Sprintf(" (+%d more)", n)
	}
	return "[system notice] Settlement pending: " + strings.Join(labels, ", ") + more +
		". The ledger may not yet hold what these scenes changed; where it matters, check their prose rather than trusting the ledger."
}

// now is the app's clock, or the wall clock when none is wired.
func (s *Service) now() int64 {
	if s.deps.Clock != nil {
		return s.deps.Clock()
	}
	return time.Now().UnixMilli()
}

// A settlement may read each scene and the ledger, and write what changed. Each
// fact is one call, so it gets a larger budget than the review. Reading the
// open questions for leaks (Phase 7) adds a call a scene; the budget stays
// until Phase 6's measurement of real passes says it runs short.
const settlementCallsPerScene = 14

// settlementTools can record in the ledger and report what does not fit. It has
// no way to touch the prose or the story world: what a scene established is the
// only thing it is here to write down.
var settlementTools = map[string]bool{
	"linetta_read_scene":        true,
	"linetta_list_ledger":       true,
	"linetta_edit_ledger":       true,
	"linetta_report_continuity": true,
}

// settlementSystemPrompt asks for changes only. A fact that the story leans on
// later is worth a row; the prose is the record of everything else, and a ledger
// that copies it grows with the book and stops being readable.
const settlementSystemPrompt = `You are settling a finished chapter of fiction into the work's ledger, for its author. You did not write it. Your reply goes nowhere: what you find reaches the author only through the ledger and linetta_report_continuity.

For each scene you are given:
1. Read it with linetta_read_scene numbered=true.
2. Look up what the ledger already holds with linetta_list_ledger, using query for the subject.
3. For each thing this scene changed that later scenes must honour, call linetta_edit_ledger with kind (number, timeline, knowledge or decision), subject, attribute, body, planted_node_id and planted_quote copied exactly from the scene. Use the attributes 위치, 소지금, 경지, 부상, 소속, 호칭, 작중 날짜, 앎. Set value_num when the value is a number, and strength soft when it comes from a character's mouth or a rumour rather than the narrator. When a character learns the answer to an open question, or is told not to know it, record 앎 with about set to the question's id and value_num 1 (knows) or 0 (does not).
4. When a value replaces one already recorded, pass its id as supersedes.
5. When the result lists conflicts and the scene is not plainly changing the value, the prose and the record disagree: call linetta_report_continuity with the quote, and leave the fact as it is.
6. Read the open questions and their answers with linetta_list_ledger kind=question. Call linetta_report_continuity with the quote where the narrator states an answer the story has not paid yet, with a note starting "비밀 누설:" (a character's guess or a rumour is not one), and where a character acts on an answer they do not know at this point, with a note starting "앎 위반:". Even when this scene is the first to tell an answer, do not pay the question: when the reader learns it is the author's call.

Record only what changed and will matter: knowledge only about open questions. Do not retell the plot, and do not record what is already there. Nothing to record is a good result.`

func settlementRequest(projectID string, scenes []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Work: %s\nSettle these scenes, in order:\n", projectID)
	for _, id := range scenes {
		b.WriteString("- " + id + "\n")
	}
	return b.String()
}

// impactCallsPerEntry covers finding the scenes, reading the twelve it may list,
// and a few reports. impactMaxEntries keeps a turn that settled many things from
// sending the book back.
const (
	impactCallsPerEntry = 18
	impactMaxEntries    = 3
)

// impactTools can look and report, and nothing else: a change's effect on the
// prose is the writer's to act on.
var impactTools = map[string]bool{
	"linetta_list_ledger":       true,
	"linetta_find_impact":       true,
	"linetta_read_scene":        true,
	"linetta_search_manuscript": true,
	"linetta_report_continuity": true,
}

const impactSystemPrompt = `A decision or a fact in the ledger of a work of fiction has just changed. You are finding where the prose already written no longer agrees with it, for its author. You did not write it. Your reply goes nowhere: what you find reaches the author only through linetta_report_continuity.

For each entry you are given:
1. Read it with linetta_list_ledger, using query for its subject, and take what it now says.
2. Call linetta_find_impact with its id for the scenes to read.
3. Read each with linetta_read_scene numbered=true, looking only for statements this entry makes false. A candidate is a scene worth reading, not a scene that is wrong.
4. Call linetta_report_continuity once for each scene that disagrees, with source quotes copied exactly and a note starting "영향 검사:" that names the entry. An empty list is a good result: skip scenes that agree.

Report what cannot both be true, not what could be better. Do not suggest prose longer than a sentence.`

func impactRequest(projectID string, entries []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Work: %s\nThese ledger entries changed:\n", projectID)
	for _, id := range entries {
		b.WriteString("- " + id + "\n")
	}
	return b.String()
}

// sampleTools read and report: two scenes, side by side, and no brief. What the
// agent believes about the book is what this pass exists to do without.
var sampleTools = map[string]bool{
	"linetta_read_scene":        true,
	"linetta_report_continuity": true,
}

const sampleSystemPrompt = `You are comparing scenes of a work of fiction, for its author. You know nothing of how it came to be written and nothing about it but what you read here. Your reply goes nowhere: what you find reaches the author only through linetta_report_continuity.

For each pair you are given, read both scenes with linetta_read_scene numbered=true, the later one being the scene the finding belongs to. Look for two statements that cannot both be true: a fact, a number, a date, an object, who knows what, what someone can do. A character's lie or a rumour that later proves wrong is not a contradiction. A fixed rule of the work counts as a statement.

Call linetta_report_continuity for the later scene only when you find one, with kind contradiction, source quotes copied exactly from both scenes, and a note starting "표본 대조:". Finding nothing is the usual result, and a good one. Taste, pacing and style are not findings.`
