package handlers

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// The UI translates on the reason code, not on the English sentence, so the
// code has to survive on the wire.
func TestProviderMethodError_tagsTheConsentFailureWithAReasonCode(t *testing.T) {
	err := providerMethodError(ai.ResolvedProvider{Provider: "openrouter"}, ai.ErrDataSharingConsentRequired)

	if err.Code != rpc.CodeInternalError {
		t.Errorf("Code = %d, want %d", err.Code, rpc.CodeInternalError)
	}
	var payload struct {
		Reason string `json:"reason"`
	}
	if e := json.Unmarshal(err.Data, &payload); e != nil {
		t.Fatalf("Data is not decodable json: %v (data=%s)", e, err.Data)
	}
	if payload.Reason != rpc.ReasonAIDataSharingConsentRequired {
		t.Errorf("reason = %q, want %q", payload.Reason, rpc.ReasonAIDataSharingConsentRequired)
	}
	if err.Message == "" {
		t.Error("Message is empty; it must stay readable for logs and older clients")
	}
}

func TestProviderMethodError_doesNotTagUnrelatedFailures(t *testing.T) {
	err := providerMethodError(ai.ResolvedProvider{Provider: "openrouter"}, errors.New("connection refused"))

	if err.Data != nil {
		t.Errorf("Data = %s, want nil for an untagged failure", err.Data)
	}
	if err.Message == "" {
		t.Error("Message is empty")
	}
}
