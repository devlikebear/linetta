// Package snapshot persists node_snapshots — point-in-time copies of a leaf
// node's content_doc, tagged with a reason (manual, autosave, companion-before).
package snapshot

// Snapshot mirrors the node_snapshots row.
type Snapshot struct {
	ID         string `json:"id"`
	NodeID     string `json:"node_id"`
	ContentDoc string `json:"content_doc"`
	Reason     string `json:"reason"`
	CreatedAt  int64  `json:"created_at"`
}

// CompareSide is a plaintext view of one snapshot in a two-version comparison.
type CompareSide struct {
	ID        string `json:"id"`
	Reason    string `json:"reason"`
	CreatedAt int64  `json:"created_at"`
	Plaintext string `json:"plaintext"`
}

// CompareResult returns the two selected snapshots in caller-specified order.
type CompareResult struct {
	Left  CompareSide `json:"left"`
	Right CompareSide `json:"right"`
}

// Reasons.
const (
	ReasonManual          = "manual"
	ReasonAutosave        = "autosave"
	ReasonCompanionBefore = "companion-before"
)

func ValidReason(reason string) bool {
	return reason == ReasonManual || reason == ReasonAutosave || reason == ReasonCompanionBefore
}
