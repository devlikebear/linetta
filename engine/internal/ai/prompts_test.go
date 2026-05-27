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

func TestBuildSystem_myToneEmphasizesStyleNotes(t *testing.T) {
	c := Context{
		StyleNotes: "단문 위주, 한자어 자제.",
		Options:    Options{Tone: TonePresetMy},
		UserPrompt: "재작성",
	}
	sys := BuildMessages(c)[0].Content
	if !strings.Contains(sys, "단문 위주, 한자어 자제.") {
		t.Errorf("my tone should inject style_notes: %q", sys)
	}
}

func TestBuildSystem_coolToneAppendsPhrase(t *testing.T) {
	c := Context{Options: Options{Tone: TonePresetCool}, UserPrompt: "확장"}
	sys := BuildMessages(c)[0].Content
	if !strings.Contains(sys, "차갑고 거리감") {
		t.Errorf("cool tone phrase missing: %q", sys)
	}
}

func TestBuildSystem_emptyToneNoExtra(t *testing.T) {
	c := Context{Options: Options{Tone: ""}, UserPrompt: "재작성"}
	sys := BuildMessages(c)[0].Content
	if strings.Contains(sys, "톤으로 유지하라") || strings.Contains(sys, "스타일 노트") {
		t.Errorf("empty tone leaked fragment: %q", sys)
	}
}

func TestBuildSystem_shortFormStillWorks(t *testing.T) {
	c := Context{Options: Options{Tone: TonePresetSensory, ShortForm: true}, UserPrompt: "확장"}
	sys := BuildMessages(c)[0].Content
	if !strings.Contains(sys, "한 문단 이내") {
		t.Errorf("short_form clause dropped: %q", sys)
	}
	if !strings.Contains(sys, "감각적 톤") {
		t.Errorf("tone fragment missing alongside short_form: %q", sys)
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
		Options:    Options{Tone: TonePresetMy, ShortForm: true},
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

func TestBuildUser_includesActiveThreads(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		ActiveThreads: []ActiveThread{
			{
				Name:    "잃어버린 시간",
				Color:   "#c08a3e",
				Summary: "여름 한 철의 기억",
				RecentBeats: []BeatBrief{
					{Label: "사진을 찍는 손", Ordinal: 3},
					{Label: "사라진 자전거", Ordinal: 4},
				},
			},
		},
		UserPrompt: "확장",
	}
	msgs := BuildMessages(c)
	usr := msgs[1].Content
	if !strings.Contains(usr, "## 활성 스토리라인") {
		t.Errorf("missing header: %q", usr)
	}
	if !strings.Contains(usr, "잃어버린 시간") || !strings.Contains(usr, "여름 한 철의 기억") {
		t.Errorf("thread metadata missing: %q", usr)
	}
	if !strings.Contains(usr, "#3 사진을 찍는 손") || !strings.Contains(usr, "#4 사라진 자전거") {
		t.Errorf("beats missing: %q", usr)
	}
}

func TestBuildUser_omitsActiveThreadsHeaderWhenEmpty(t *testing.T) {
	c := Context{SceneLabel: "씬 1", SceneText: "본문", UserPrompt: "재작성"}
	usr := BuildMessages(c)[1].Content
	if strings.Contains(usr, "활성 스토리라인") {
		t.Errorf("header should not appear when empty: %q", usr)
	}
}

func TestBuildUser_includesNotesSection(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Notes: []NoteBrief{
			{Anchor: 5, Body: "여기 톤을 더 차갑게"},
			{Anchor: 22, Body: "@해진의 대사로 받기"},
		},
		UserPrompt: "확장",
	}
	msgs := BuildMessages(c)
	user := msgs[1].Content
	if !strings.Contains(user, "## 작가 주석") {
		t.Fatalf("missing 작가 주석 header: %q", user)
	}
	if !strings.Contains(user, "- 여기 톤을 더 차갑게") || !strings.Contains(user, "- @해진의 대사로 받기") {
		t.Errorf("missing note bodies: %q", user)
	}
	if strings.Contains(user, "anchor") {
		t.Errorf("anchor key leaked: %q", user)
	}
}
