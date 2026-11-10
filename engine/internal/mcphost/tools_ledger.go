package mcphost

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/ledger"
)

type listLedgerInput struct {
	ProjectID string   `json:"project_id"`
	Kinds     []string `json:"kinds,omitempty"`
	Status    string   `json:"status,omitempty"`
	Query     string   `json:"query,omitempty" jsonschema:"only entries whose subject or body contains this"`
	Cursor    string   `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
}

func (in listLedgerInput) scope() (string, string) { return in.ProjectID, "" }

type listLedgerOutput struct {
	Entries    []ledger.Entry `json:"entries"`
	Total      int            `json:"total"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type editLedgerInput struct {
	ProjectID     string   `json:"project_id"`
	ID            string   `json:"id,omitempty" jsonschema:"entry to change; omit to add one"`
	Delete        bool     `json:"delete,omitempty"`
	Kind          string   `json:"kind,omitempty" jsonschema:"promise, gag, question: owed to the reader. decision, number, timeline, knowledge: settled"`
	Subject       *string  `json:"subject,omitempty"`
	Body          *string  `json:"body,omitempty"`
	Status        *string  `json:"status,omitempty" jsonschema:"open, paid or dropped"`
	PlantedNodeID *string  `json:"planted_node_id,omitempty"`
	PlantedQuote  *string  `json:"planted_quote,omitempty" jsonschema:"exact text from that scene"`
	DueNodeID     *string  `json:"due_node_id,omitempty" jsonschema:"scene or part it must pay off by"`
	PaidNodeID    *string  `json:"paid_node_id,omitempty"`
	PaidQuote     *string  `json:"paid_quote,omitempty" jsonschema:"exact text from that scene"`
	Attribute     *string  `json:"attribute,omitempty" jsonschema:"makes a settled entry a fact: 위치, 소지금, 경지, 부상, 소속, 호칭, 나이, 작중 날짜, 경과 or 앎. Use planted_node_id as where it starts"`
	Strength      *string  `json:"strength,omitempty" jsonschema:"hard for what the narrator states, soft for what a character says or a rumour"`
	ValueNum      *float64 `json:"value_num,omitempty" jsonschema:"the number a fact holds: 경과 and 작중 날짜 in days, 나이 in years"`
	ValidToNodeID *string  `json:"valid_to_node_id,omitempty" jsonschema:"scene it stops being true at"`
	Supersedes    string   `json:"supersedes,omitempty" jsonschema:"id of the fact this one replaces"`
	Reach         *string  `json:"reach,omitempty" jsonschema:"how far a promise runs: arc (default), volume or work"`
	Answer        *string  `json:"answer,omitempty" jsonschema:"question only: the answer only the writer knows; keep it out of the prose until revealed"`
	About         *string  `json:"about,omitempty" jsonschema:"앎: question id; value_num 1 knows, 0 not. 경과: id of the timeline event counted from"`
}

func (in editLedgerInput) scope() (string, string) { return in.ProjectID, "" }

type editLedgerOutput struct {
	Entry   *ledger.Entry `json:"entry,omitempty"`
	Deleted bool          `json:"deleted,omitempty"`
	// Conflicts are recorded facts this one disagrees with. It is saved anyway.
	// Replace one with supersedes if it changed, or report it if it is a mistake.
	Conflicts []ledger.Conflict `json:"conflicts,omitempty"`
	// Previous is the last number on record, for a fact with a value.
	Previous *ledger.Previous `json:"previous,omitempty"`
}

type setPartArcInput struct {
	NodeID  string `json:"node_id" jsonschema:"a part (container)"`
	Role    string `json:"role,omitempty" jsonschema:"its job in the whole work, such as 기, 승, 전 or 결"`
	Goal    string `json:"goal,omitempty" jsonschema:"what must be true when it ends"`
	Tension int    `json:"tension,omitempty" jsonschema:"target tension, 1 to 10"`
}

func (in setPartArcInput) scope() (string, string) { return "", in.NodeID }

type findImpactInput struct {
	EntryID string `json:"entry_id" jsonschema:"the ledger entry that changed"`
}

func (in findImpactInput) scope() (string, string) { return "", "" }

type findImpactOutput struct {
	Scenes  []ledger.Candidate `json:"scenes"`
	Omitted int                `json:"omitted,omitempty"`
}

func (d ToolDeps) registerLedgerReadTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_find_impact",
		Description: "List the written scenes a ledger entry may touch, in story order, with why each is " +
			"listed: to read after a decision or fact changes. It ranks by text; reading decides.",
	}, record(d, "linetta_find_impact", d.findImpact))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "linetta_list_ledger",
		Description: "List the work's ledger, newest first: what the story owes the reader and what is settled. Follow next_cursor for the rest.",
	}, record(d, "linetta_list_ledger", d.listLedger))
}

func (d ToolDeps) registerLedgerWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_edit_ledger",
		Description: "Add, change or delete a ledger entry. Record foreshadowing and running gags when " +
			"you plant them, the writer's decisions, numbers, dates and who knows what. Mark a promise " +
			"paid with the scene and an exact quote. A fact (subject + attribute) is compared with the " +
			"ledger: conflicts are returned, not refused.",
	}, record(d, "linetta_edit_ledger", d.editLedger))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "linetta_set_part_arc",
		Description: "Set a part's job in the whole work: role, what must be true by its end, target tension.",
	}, record(d, "linetta_set_part_arc", d.setPartArc))
}

func (d ToolDeps) listLedger(ctx context.Context, _ *mcp.CallToolRequest, in listLedgerInput) (*mcp.CallToolResult, listLedgerOutput, error) {
	if d.Ledger == nil {
		return toolErr("the ledger is not available in this build"), listLedgerOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, listLedgerOutput{}, nil
	}
	entries, err := d.Ledger.List(ctx, ledger.Filter{ProjectID: p.ID, Kinds: in.Kinds, Status: in.Status})
	if err != nil {
		return toolErr("could not read the ledger: %v", err), listLedgerOutput{}, nil
	}
	// Newest first: the end of the list is what a page cuts, and the latest
	// decisions are the ones a turn is likeliest to need.
	rows := make([]ledger.Entry, 0, len(entries))
	query := strings.TrimSpace(in.Query)
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if query == "" || strings.Contains(e.Subject, query) || strings.Contains(e.Body, query) {
			rows = append(rows, e)
		}
	}
	page, next, total := pageRows(rows, in.Cursor, pageMaxRunes)
	return nil, listLedgerOutput{Entries: page, Total: total, NextCursor: next}, nil
}

func (d ToolDeps) findImpact(ctx context.Context, _ *mcp.CallToolRequest, in findImpactInput) (*mcp.CallToolResult, findImpactOutput, error) {
	if d.Ledger == nil {
		return toolErr("the ledger is not available in this build"), findImpactOutput{}, nil
	}
	scenes, omitted, err := d.Ledger.Impact(ctx, strings.TrimSpace(in.EntryID))
	if err != nil {
		return ledgerToolErr(err), findImpactOutput{}, nil
	}
	return nil, findImpactOutput{Scenes: scenes, Omitted: omitted}, nil
}

func (d ToolDeps) editLedger(ctx context.Context, _ *mcp.CallToolRequest, in editLedgerInput) (*mcp.CallToolResult, editLedgerOutput, error) {
	if d.Ledger == nil {
		return toolErr("the ledger is not available in this build"), editLedgerOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, editLedgerOutput{}, nil
	}
	id := strings.TrimSpace(in.ID)
	if id != "" {
		cur, err := d.Ledger.Get(ctx, id)
		if err != nil || cur.ProjectID != p.ID {
			return toolErr("no ledger entry %q in this work; call linetta_list_ledger", id), editLedgerOutput{}, nil
		}
		// Write-side only: a pin is the writer's, and the agent reads what it
		// holds in every brief. Asking is the way to change one.
		if cur.Pinned {
			return toolErr("%q is pinned by the writer, so it cannot be changed or deleted here. "+
				"Ask the writer to unpin it in the ledger, or to change it themselves.", id), editLedgerOutput{}, nil
		}
	}
	var out editLedgerOutput
	switch {
	case in.Delete:
		if id == "" {
			return toolErr("delete needs id"), editLedgerOutput{}, nil
		}
		if err := d.Ledger.Delete(ctx, id); err != nil {
			return ledgerToolErr(err), editLedgerOutput{}, nil
		}
		out.Deleted = true
	case id != "":
		e, err := d.Ledger.Update(ctx, d.now(), id, ledger.Patch{
			Subject: in.Subject, Body: in.Body, Status: in.Status,
			PlantedNodeID: in.PlantedNodeID, PlantedQuote: in.PlantedQuote, DueNodeID: in.DueNodeID,
			PaidNodeID: in.PaidNodeID, PaidQuote: in.PaidQuote,
			Attribute: in.Attribute, Strength: in.Strength, ValueNum: in.ValueNum, ValidToNodeID: in.ValidToNodeID,
			Reach: in.Reach, Answer: in.Answer, About: in.About,
		})
		if err != nil {
			return ledgerToolErr(err), editLedgerOutput{}, nil
		}
		out.Entry = &e
	default:
		if in.Supersedes != "" {
			old, err := d.Ledger.Get(ctx, in.Supersedes)
			if err != nil || old.ProjectID != p.ID {
				return toolErr("no ledger entry %q in this work; call linetta_list_ledger", in.Supersedes), editLedgerOutput{}, nil
			}
			if old.Pinned {
				return ledgerToolErr(ledger.ErrSupersedePinned), editLedgerOutput{}, nil
			}
		}
		e, err := d.Ledger.Add(ctx, d.now(), ledger.Entry{
			ProjectID: p.ID, Kind: in.Kind, Subject: deref(in.Subject), Body: deref(in.Body), Status: deref(in.Status),
			PlantedNodeID: deref(in.PlantedNodeID), PlantedQuote: deref(in.PlantedQuote), DueNodeID: deref(in.DueNodeID),
			PaidNodeID: deref(in.PaidNodeID), PaidQuote: deref(in.PaidQuote), Source: d.ledgerSource(),
			Attribute: deref(in.Attribute), Strength: deref(in.Strength), ValueNum: in.ValueNum, ValidToNodeID: deref(in.ValidToNodeID),
			Reach: deref(in.Reach), Answer: deref(in.Answer), About: deref(in.About),
		})
		if err != nil {
			return ledgerToolErr(err), editLedgerOutput{}, nil
		}
		out.Entry = &e
		if in.Supersedes != "" {
			if err := d.Ledger.Supersede(ctx, d.now(), in.Supersedes, e); err != nil {
				return ledgerToolErr(err), editLedgerOutput{}, nil
			}
		}
	}
	if out.Entry != nil {
		conflicts, previous, err := d.Ledger.Conflicts(ctx, *out.Entry)
		if err != nil {
			return toolErr("saved, but could not compare it with the ledger: %v", err), editLedgerOutput{}, nil
		}
		out.Conflicts, out.Previous = conflicts, previous
	}
	d.notifyChanged(p.ID, "linetta_edit_ledger", nil, "")
	return nil, out, nil
}

func (d ToolDeps) setPartArc(ctx context.Context, _ *mcp.CallToolRequest, in setPartArcInput) (*mcp.CallToolResult, ledger.Arc, error) {
	if d.Ledger == nil {
		return toolErr("the ledger is not available in this build"), ledger.Arc{}, nil
	}
	n, errResult := d.requireNode(ctx, in.NodeID)
	if errResult != nil {
		return errResult, ledger.Arc{}, nil
	}
	a, err := d.Ledger.SetArc(ctx, d.now(), ledger.Arc{NodeID: n.ID, Role: in.Role, Goal: in.Goal, Tension: in.Tension})
	if err != nil {
		return ledgerToolErr(err), ledger.Arc{}, nil
	}
	d.notifyChanged(n.ProjectID, "linetta_set_part_arc", nil, "")
	return nil, a, nil
}

func (d ToolDeps) ledgerSource() string {
	if d.Source == SourceAgent {
		return ledger.SourceAgent
	}
	return ledger.SourceExternal
}

func ledgerToolErr(err error) *mcp.CallToolResult {
	if errors.Is(err, ledger.ErrNotFound) {
		return toolErr("no such ledger entry; call linetta_list_ledger")
	}
	return toolErr("%s", strings.TrimPrefix(err.Error(), "ledger: "))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
