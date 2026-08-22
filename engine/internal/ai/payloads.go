package ai

// DeltaPayload is the body of an "ai.delta" notification.
type DeltaPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}

// DonePayload is the body of an "ai.done" notification.
type DonePayload struct {
	RunID    string `json:"run_id"`
	FullText string `json:"full_text"`
}

// ErrorPayload is the body of an "ai.error" notification.
type ErrorPayload struct {
	RunID   string `json:"run_id"`
	Message string `json:"message"`
}

// CancelledPayload is the body of an "ai.cancelled" notification.
type CancelledPayload struct {
	RunID string `json:"run_id"`
}

// ResetPayload is the body of an "ai.reset" notification. Sent when the
// streaming text needs to be REPLACED (not appended) — used when the upstream
// provider's transparent retry produces deltas that diverge from earlier ones
// and we need to reconcile the frontend's view to the deduplicated buffer.
type ResetPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}
