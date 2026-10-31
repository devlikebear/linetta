// Package ledger is a work's account of itself, kept apart from its prose: what
// the story has promised and not yet paid, what has been settled, and what each
// part is for. The brief reads it so a long work's memory does not depend on
// summaries of the nearest scenes.
package ledger

import "errors"

// Kinds of entry. The first three are owed to the reader and end paid or
// dropped; the rest are settled matters, in force until dropped. A cut plan is
// what a deleted episode was meant to do, kept so a restructure loses nothing
// silently.
const (
	KindPromise   = "promise"
	KindGag       = "gag"
	KindQuestion  = "question"
	KindDecision  = "decision"
	KindNumber    = "number"
	KindTimeline  = "timeline"
	KindKnowledge = "knowledge"
	KindCutPlan   = "cut_plan"
)

// Statuses.
const (
	StatusOpen    = "open"
	StatusPaid    = "paid"
	StatusDropped = "dropped"
)

// Sources.
const (
	SourceWriter   = "writer"
	SourceAgent    = "agent"
	SourceExternal = "external"
)

var (
	ErrNotFound      = errors.New("ledger: entry not found")
	ErrInvalidKind   = errors.New("ledger: kind must be promise, gag, question, decision, number, timeline, knowledge or cut_plan")
	ErrInvalidStatus = errors.New("ledger: status must be open, paid or dropped")
	ErrEmptyBody     = errors.New("ledger: body is required")
	ErrWrongWork     = errors.New("ledger: that scene belongs to another work")
	ErrQuoteNotFound = errors.New("ledger: the quote is not in that scene; copy it exactly from linetta_read_scene")
	ErrPaidNeedsNode = errors.New("ledger: mark an entry paid with the scene that pays it")
	ErrNotAPart      = errors.New("ledger: an arc belongs to a part, not a scene")
	ErrTension       = errors.New("ledger: tension must be 0 to 10")
)

// ValidKind reports whether v is one of the kinds above.
func ValidKind(v string) bool {
	switch v {
	case KindPromise, KindGag, KindQuestion, KindDecision, KindNumber, KindTimeline, KindKnowledge, KindCutPlan:
		return true
	}
	return false
}

// Tracked reports whether a kind is something the story owes, as against
// something it has settled.
func Tracked(kind string) bool {
	return kind == KindPromise || kind == KindGag || kind == KindQuestion
}

// Entry is one line of the ledger.
type Entry struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	Kind          string `json:"kind"`
	Subject       string `json:"subject"`
	Body          string `json:"body"`
	Status        string `json:"status"`
	PlantedNodeID string `json:"planted_node_id,omitempty"`
	PlantedQuote  string `json:"planted_quote,omitempty"`
	DueNodeID     string `json:"due_node_id,omitempty"`
	PaidNodeID    string `json:"paid_node_id,omitempty"`
	PaidQuote     string `json:"paid_quote,omitempty"`
	Source        string `json:"source"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// Patch is a partial update. Nil leaves a field as it is; an empty string clears
// an optional field.
type Patch struct {
	Subject       *string `json:"subject,omitempty"`
	Body          *string `json:"body,omitempty"`
	Status        *string `json:"status,omitempty"`
	PlantedNodeID *string `json:"planted_node_id,omitempty"`
	PlantedQuote  *string `json:"planted_quote,omitempty"`
	DueNodeID     *string `json:"due_node_id,omitempty"`
	PaidNodeID    *string `json:"paid_node_id,omitempty"`
	PaidQuote     *string `json:"paid_quote,omitempty"`
}

// Arc is one part's job in the whole work.
type Arc struct {
	NodeID    string `json:"node_id"`
	ProjectID string `json:"project_id"`
	Role      string `json:"role"`
	Goal      string `json:"goal"`
	Tension   int    `json:"tension"`
	UpdatedAt int64  `json:"updated_at"`
}
