package settings

import (
	"context"
	"testing"
)

func TestAgentHistoryBudgetDefaultsToAutomaticAndSurvivesARestart(t *testing.T) {
	t.Setenv("LINETTA_HOME", t.TempDir())
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings store: %v", err)
	}
	if got := s.AgentHistoryBudget(); got != 0 {
		t.Fatalf("default = %d, want automatic", got)
	}
	want := 60000
	if _, err := s.Set(context.Background(), Patch{AgentHistoryBudget: &want}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	again, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings store: %v", err)
	}
	if got := again.AgentHistoryBudget(); got != want {
		t.Errorf("after a restart = %d, want %d", got, want)
	}
}

func TestAgentHistoryBudgetRefusesWhatIsOutOfRange(t *testing.T) {
	t.Setenv("LINETTA_HOME", t.TempDir())
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings store: %v", err)
	}
	for _, bad := range []int{-1, 1, MinAgentHistoryBudget - 1, MaxAgentHistoryBudget + 1} {
		if _, err := s.Set(context.Background(), Patch{AgentHistoryBudget: &bad}); err == nil {
			t.Errorf("%d was accepted", bad)
		}
	}
	zero := 0
	if _, err := s.Set(context.Background(), Patch{AgentHistoryBudget: &zero}); err != nil {
		t.Errorf("zero means automatic: %v", err)
	}
}
