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
)

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
