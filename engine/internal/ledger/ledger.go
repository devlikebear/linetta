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
	ErrNotFound        = errors.New("ledger: entry not found")
	ErrInvalidKind     = errors.New("ledger: kind must be promise, gag, question, decision, number, timeline, knowledge or cut_plan")
	ErrInvalidStatus   = errors.New("ledger: status must be open, paid or dropped")
	ErrEmptyBody       = errors.New("ledger: body is required")
	ErrWrongWork       = errors.New("ledger: that scene belongs to another work")
	ErrQuoteNotFound   = errors.New("ledger: the quote is not in that scene; copy it exactly from linetta_read_scene")
	ErrPaidNeedsNode   = errors.New("ledger: mark an entry paid with the scene that pays it")
	ErrNotAPart        = errors.New("ledger: an arc belongs to a part, not a scene")
	ErrTension         = errors.New("ledger: tension must be 0 to 10")
	ErrInvalidReach    = errors.New("ledger: reach must be arc, volume or work")
	ErrFactOnly        = errors.New("ledger: only a decision, number, timeline or knowledge entry can carry an attribute, a span or a value")
	ErrFactNoSubject   = errors.New("ledger: a fact with an attribute needs a subject")
	ErrInvalidStrength = errors.New("ledger: strength must be hard or soft")
	ErrSupersedePinned = errors.New("ledger: the writer pinned that entry; ask them to change it")
	ErrNotPinnable     = errors.New("ledger: only an open question, decision, number, timeline or knowledge entry can be pinned")
	// The truth behind a secret, what a person's knowing is about, and the
	// event an elapsed time is counted from.
	ErrAnswerQuestionOnly = errors.New("ledger: only a question carries an answer")
	ErrAboutWrongFact     = errors.New("ledger: about belongs to a knowledge fact with the attribute 앎 (a question's id) or a timeline fact with the attribute 경과 (an event's id)")
	ErrAboutNotQuestion   = errors.New("ledger: about must be the id of a question in this work")
	ErrAboutNotEvent      = errors.New("ledger: about must be the id of a timeline entry in this work that is an event, not another 경과")
	ErrAboutNeedsValue    = errors.New("ledger: a knows fact about a question needs value_num 1 (knows) or 0 (does not know)")
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
	// Pinned is set by the writer alone: a pinned entry reaches every brief and
	// no agent can change it. It is not in Patch for that reason.
	Pinned bool `json:"pinned,omitempty"`
	// Attribute says what about Subject a settled entry fixes, and makes it a
	// fact with a span: true from PlantedNodeID until ValidToNodeID. An entry
	// without one stays a free note.
	Attribute     string `json:"attribute,omitempty"`
	ValidToNodeID string `json:"valid_to_node_id,omitempty"`
	// SupersededBy is the entry that replaced this one. It is kept, not deleted.
	SupersededBy string `json:"superseded_by,omitempty"`
	// Strength is StrengthHard for what the narrator stated, StrengthSoft for
	// what a character said or a rumour carried.
	Strength string `json:"strength,omitempty"`
	// ValueNum is the number a fact holds, when it holds one.
	ValueNum *float64 `json:"value_num,omitempty"`
	// Reach is how far a promise runs: ReachArc, ReachVolume or ReachWork. The
	// brief stops showing one past its reach unless it has come due.
	Reach string `json:"reach,omitempty"`
	// Answer is the truth behind a question, which only the writer knows until
	// the scene that pays the question tells the reader.
	Answer string `json:"answer,omitempty"`
	// About is the question a knows fact is about, its ValueNum then saying
	// which way: KnowsYes or KnowsNo. On an elapsed fact it is the timeline
	// event the time is counted from, its ValueNum the days since.
	About     string `json:"about,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Reaches. An arc is the default: most of what a story owes, it pays soon.
const (
	ReachArc    = "arc"
	ReachVolume = "volume"
	ReachWork   = "work"
)

// Strengths. Hard is the default.
const (
	StrengthHard = "hard"
	StrengthSoft = "soft"
)

// AttributeDate is the attribute whose value only moves forward in story order.
const AttributeDate = "작중 날짜"

// AttributeKnows piles up: a person comes to know one thing after another, and
// the second does not replace the first.
const AttributeKnows = "앎"

// AttributeElapsed is how long it has been, at its scene, since the event its
// About names. Its ValueNum is in days. An event is a timeline entry with no
// elapsed attribute of its own; its ValueNum, when known, is its story date.
const AttributeElapsed = "경과"

// AttributeAge with a ValueNum is a person's age in years: time since their
// birth, compared like an elapsed fact with the person as the event.
const AttributeAge = "나이"

// The two values a knows fact about a question can hold.
const (
	KnowsNo  = 0.0
	KnowsYes = 1.0
)

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
	Attribute     *string `json:"attribute,omitempty"`
	ValidToNodeID *string `json:"valid_to_node_id,omitempty"`
	Strength      *string `json:"strength,omitempty"`
	Reach         *string `json:"reach,omitempty"`
	Answer        *string `json:"answer,omitempty"`
	About         *string `json:"about,omitempty"`
	// ValueNum nil leaves the number as it is. There is no clearing one: a fact
	// whose value stops being a number is a new fact.
	ValueNum *float64 `json:"value_num,omitempty"`
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
