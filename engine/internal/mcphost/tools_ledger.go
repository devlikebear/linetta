//go:build !mobile

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
}

func (in listLedgerInput) scope() (string, string) { return in.ProjectID, "" }

type listLedgerOutput struct {
	Entries []ledger.Entry `json:"entries"`
}

type editLedgerInput struct {
	ProjectID     string  `json:"project_id"`
	ID            string  `json:"id,omitempty" jsonschema:"entry to change; omit to add one"`
	Delete        bool    `json:"delete,omitempty"`
	Kind          string  `json:"kind,omitempty" jsonschema:"promise, gag, question: owed to the reader. decision, number, timeline, knowledge: settled"`
	Subject       *string `json:"subject,omitempty"`
	Body          *string `json:"body,omitempty"`
	Status        *string `json:"status,omitempty" jsonschema:"open, paid or dropped"`
	PlantedNodeID *string `json:"planted_node_id,omitempty"`
	PlantedQuote  *string `json:"planted_quote,omitempty" jsonschema:"exact text from that scene"`
	DueNodeID     *string `json:"due_node_id,omitempty" jsonschema:"scene or part it must pay off by"`
	PaidNodeID    *string `json:"paid_node_id,omitempty"`
	PaidQuote     *string `json:"paid_quote,omitempty" jsonschema:"exact text from that scene"`
}

func (in editLedgerInput) scope() (string, string) { return in.ProjectID, "" }

type editLedgerOutput struct {
	Entry   *ledger.Entry `json:"entry,omitempty"`
	Deleted bool          `json:"deleted,omitempty"`
}

type setPartArcInput struct {
	NodeID  string `json:"node_id" jsonschema:"a part (container)"`
	Role    string `json:"role,omitempty" jsonschema:"its job in the whole work, such as 기, 승, 전 or 결"`
	Goal    string `json:"goal,omitempty" jsonschema:"what must be true when it ends"`
	Tension int    `json:"tension,omitempty" jsonschema:"target tension, 1 to 10"`
}

func (in setPartArcInput) scope() (string, string) { return "", in.NodeID }

func (d ToolDeps) registerLedgerReadTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "linetta_list_ledger",
		Description: "List the work's ledger: what the story owes the reader and what is settled.",
	}, record(d, "linetta_list_ledger", d.listLedger))
}

func (d ToolDeps) registerLedgerWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_edit_ledger",
		Description: "Add, change or delete a ledger entry. Record foreshadowing and running gags when " +
			"you plant them, the writer's decisions, numbers, dates and who knows what. Mark a promise " +
			"paid with the scene and an exact quote.",
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
	return nil, listLedgerOutput{Entries: entries}, nil
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
		})
		if err != nil {
			return ledgerToolErr(err), editLedgerOutput{}, nil
		}
		out.Entry = &e
	default:
		e, err := d.Ledger.Add(ctx, d.now(), ledger.Entry{
			ProjectID: p.ID, Kind: in.Kind, Subject: deref(in.Subject), Body: deref(in.Body), Status: deref(in.Status),
			PlantedNodeID: deref(in.PlantedNodeID), PlantedQuote: deref(in.PlantedQuote), DueNodeID: deref(in.DueNodeID),
			PaidNodeID: deref(in.PaidNodeID), PaidQuote: deref(in.PaidQuote), Source: d.ledgerSource(),
		})
		if err != nil {
			return ledgerToolErr(err), editLedgerOutput{}, nil
		}
		out.Entry = &e
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
