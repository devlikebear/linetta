package rpc

import (
	"encoding/json"
	"errors"
)

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

	// The built-in agent's provider layer (#90). The first two are states the
	// settings pane fixes; the last three are what the provider said, reduced
	// to something the reader can act on. The provider's raw body stays in
	// the English Message for logs and never becomes UI text.
	ReasonProviderNotConfigured   = "provider_not_configured"
	ReasonProviderConsentRequired = "provider_consent_required"
	ReasonProviderAuthFailed      = "provider_auth_failed"
	ReasonProviderRateLimited     = "provider_rate_limited"
	ReasonProviderUnreachable     = "provider_unreachable"
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

// ReasonError carries a reason code out of a package that does not build
// MethodErrors itself (internal/provider must not know about JSON-RPC).
// Handlers turn it into one with MethodErrorFrom.
type ReasonError struct {
	Reason string
	Err    error
}

func (e *ReasonError) Error() string {
	if e.Err == nil {
		return e.Reason
	}
	return e.Reason + ": " + e.Err.Error()
}

func (e *ReasonError) Unwrap() error { return e.Err }

// MethodErrorFrom maps any error to a MethodError: a ReasonError anywhere in
// the chain becomes an InvalidParams error carrying its reason; anything else
// is an internal error with the message alone.
func MethodErrorFrom(err error) *MethodError {
	var re *ReasonError
	if errors.As(err, &re) {
		return &MethodError{Code: CodeInvalidParams, Message: err.Error(), Data: ReasonData(re.Reason)}
	}
	return &MethodError{Code: CodeInternalError, Message: err.Error()}
}
