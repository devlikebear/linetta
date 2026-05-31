// Package snapshot persists node_snapshots — point-in-time copies of a leaf
// node's content_doc, tagged with a reason (manual, autosave, ai-replace).
package snapshot

// Snapshot mirrors the node_snapshots row.
type Snapshot struct {
	ID         string `json:"id"`
	NodeID     string `json:"node_id"`
	ContentDoc string `json:"content_doc"`
	Reason     string `json:"reason"`
	CreatedAt  int64  `json:"created_at"`
}

// Reasons.
const (
	ReasonManual    = "manual"
	ReasonAutosave  = "autosave"
	ReasonAIReplace = "ai-replace"
)

func ValidReason(reason string) bool {
	return reason == ReasonManual || reason == ReasonAutosave || reason == ReasonAIReplace
}
