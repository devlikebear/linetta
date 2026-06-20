package snapshot

import "testing"

func TestValidReason_acceptsCompanionBeforeAndRejectsAIReplace(t *testing.T) {
	if !ValidReason("companion-before") {
		t.Fatal("companion-before should be a valid snapshot reason")
	}
	if ValidReason("ai-replace") {
		t.Fatal("ai-replace should no longer be a valid snapshot reason")
	}
}
