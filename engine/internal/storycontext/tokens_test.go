package storycontext

import "testing"

func TestEstimateTokens(t *testing.T) {
	if got := EstimateChars("가나다abc"); got != 6 {
		t.Fatalf("EstimateChars = %d, want 6", got)
	}
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("EstimateTokens empty = %d, want 0", got)
	}
	if got := EstimateTokens("가나다abc"); got != 2 {
		t.Fatalf("EstimateTokens = %d, want 2", got)
	}
	if got := EstimateTokens("abcdefg"); got != 3 {
		t.Fatalf("EstimateTokens rounds up = %d, want 3", got)
	}
}
