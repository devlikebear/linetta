package ai

import (
	"strings"
	"testing"
)

func TestPresetSeed(t *testing.T) {
	if PresetSeed(PresetRewrite) == "" {
		t.Error("rewrite seed should be non-empty")
	}
	if PresetSeed(PresetFree) != "" {
		t.Error("free preset should have no seed")
	}
}

func TestBuildMessages_shapesSystemAndUser(t *testing.T) {
	c := Context{
		ProjectID:   "p1",
		NodeID:      "n1",
		SceneLabel:  "씬 1",
		SceneText:   "파도 소리.",
		PrevSummary: "어제는 비가 왔다.",
		Entities: []EntityBrief{
			{Name: "해진", Kind: "character", Role: "POV", Summary: "사진작가"},
		},
		StyleNotes: "단문 위주",
		UserPrompt: "재작성",
		Options:    Options{TonePreset: true, ShortForm: true},
	}
	msgs := BuildMessages(c)
	if len(msgs) != 2 {
		t.Fatalf("len = %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Errorf("roles = %q, %q", msgs[0].Role, msgs[1].Role)
	}
	sys := msgs[0].Content
	if !strings.Contains(sys, "단문 위주") {
		t.Errorf("style notes not in system: %q", sys)
	}
	if !strings.Contains(sys, "한 문단 이내") {
		t.Errorf("short_form not in system: %q", sys)
	}
	usr := msgs[1].Content
	if !strings.Contains(usr, "씬 1") || !strings.Contains(usr, "파도 소리") {
		t.Errorf("scene missing from user: %q", usr)
	}
	if !strings.Contains(usr, "@해진") || !strings.Contains(usr, "POV") {
		t.Errorf("entities missing from user: %q", usr)
	}
}
