package server

import (
	"os"
	"testing"
)

// TestMain ensures unit tests never shell out to the user's codex CLI or
// hit OpenAI. The run-episode test path goes through the agent runner which
// would otherwise resolve a real LLM provider from ~/.linetta/tessera.yaml.
func TestMain(m *testing.M) {
	os.Setenv("LINETTA_DISABLE_LLM", "1")
	os.Exit(m.Run())
}
