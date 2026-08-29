package rpc

import "encoding/json"

// Reason codes name a failure the UI is expected to explain in the reader's
// own language. The Message on a MethodError stays English for logs and for
// clients that do not recognise the code; the code is what gets translated.
//
// Engine-side translation was the other option and is the wrong one here: the
// desktop app already carries a ko/en/ja catalogue, and Korean strings baked
// into handlers leak Korean to English and Japanese readers.
const (
	ReasonAIDataSharingConsentRequired = "ai_data_sharing_consent_required"
	ReasonMCPPortInUse                 = "mcp_port_in_use"
	ReasonMCPConsentRequired           = "mcp_consent_required"

	// The "not found" family. These are the failures a writer actually meets:
	// a scene deleted in another window, a character an agent removed, a
	// thread that went with the work it belonged to. The rest of the engine's
	// InvalidParams messages ("id required") only fire on a malformed call and
	// stay English for logs.
	ReasonNodeNotFound         = "node_not_found"
	ReasonProjectNotFound      = "project_not_found"
	ReasonEntityNotFound       = "entity_not_found"
	ReasonThreadNotFound       = "thread_not_found"
	ReasonBeatNotFound         = "beat_not_found"
	ReasonRelationshipNotFound = "relationship_not_found"
	ReasonNoteNotFound         = "note_not_found"
	ReasonFactCardNotFound     = "fact_card_not_found"
)

// NotFound builds the error for a record the caller asked for and the engine
// could not find. Message stays English for logs; reason is what gets
// translated (see apps/desktop/src/lib/rpcMessage.ts).
func NotFound(reason, message string) *MethodError {
	return &MethodError{Code: CodeInvalidParams, Message: message, Data: ReasonData(reason)}
}

// ReasonData builds the MethodError Data payload carrying a reason code.
func ReasonData(reason string) json.RawMessage {
	encoded, err := json.Marshal(struct {
		Reason string `json:"reason"`
	}{Reason: reason})
	if err != nil {
		return nil
	}
	return encoded
}
