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

func TestBuildUser_emitsHierarchicalSections(t *testing.T) {
	c := Context{
		SceneLabel: "씬 3",
		SceneText:  "씬 3 본문",
		Hierarchical: HierarchicalContext{
			ProjectSynopsis: "작품 시놉시스",
			OtherPartSummaries: []PartSummary{
				{Label: "2부", Body: "2부 요약"},
			},
			OtherChapterSummaries: []ChapterSummary{
				{Label: "1부 / 2장", Body: "2장 요약"},
				{Label: "1부 / 3장", Body: "3장 요약"},
			},
			SameChapterSummaries: []SceneSummary{
				{Label: "1부 / 1장 / 씬 5", Body: "씬 5 요약"},
				{Label: "1부 / 1장 / 씬 6", Body: "씬 6 요약"},
			},
			NearbyLeafSummaries: []SceneSummary{
				{Label: "1부 / 1장 / 씬 1", Body: "씬 1 요약"},
				{Label: "1부 / 1장 / 씬 2", Body: "씬 2 요약"},
				{Label: "1부 / 1장 / 씬 4", Body: "씬 4 요약"},
			},
		},
		RelatedScenes: []SceneSummary{
			{Label: "1부 / 1장 / 씬 9", Body: "관련 씬 요약"},
		},
		Entities: []EntityBrief{
			{Name: "해진", Kind: "character"},
			{Name: "민호", Kind: "character", Recent: []string{"민호 dossier line"}},
		},
		UserPrompt: "확장",
	}
	usr := BuildMessages(c)[1].Content

	want := []string{
		"## 작품 전반",
		"## 인근 줄거리",
		"## 같은 장 다른 씬",
		"## 직전·직후 씬 발췌",
		"## 관련 과거 씬",
		"## 현재 씬: 씬 3",
		"## 등장 인물·장소",
		"## 작가의 지시",
	}
	last := -1
	for _, s := range want {
		idx := strings.Index(usr, s)
		if idx < 0 {
			t.Errorf("missing %q in prompt; got:\n%s", s, usr)
			continue
		}
		if idx < last {
			t.Errorf("section %q at %d came before previous (last=%d); got:\n%s", s, idx, last, usr)
		}
		last = idx
	}

	// Bullet formatting sanity checks.
	if !strings.Contains(usr, "- [2부] 2부 요약") {
		t.Errorf("missing other-part bullet: %s", usr)
	}
	if !strings.Contains(usr, "- [1부 / 2장] 2장 요약") {
		t.Errorf("missing other-chapter bullet: %s", usr)
	}
	if !strings.Contains(usr, "- [1부 / 1장 / 씬 5] 씬 5 요약") {
		t.Errorf("missing same-chapter bullet: %s", usr)
	}
	if !strings.Contains(usr, "- [1부 / 1장 / 씬 1] 씬 1 요약") {
		t.Errorf("missing nearby bullet: %s", usr)
	}
	if !strings.Contains(usr, "- [1부 / 1장 / 씬 9] 관련 씬 요약") {
		t.Errorf("missing related-scene bullet: %s", usr)
	}
	// Within 인근 줄거리, parts come before chapters per the spec.
	nearIdx := strings.Index(usr, "## 인근 줄거리")
	partIdx := strings.Index(usr, "- [2부] 2부 요약")
	chapIdx := strings.Index(usr, "- [1부 / 2장] 2장 요약")
	if nearIdx < 0 || partIdx < 0 || chapIdx < 0 || !(nearIdx < partIdx && partIdx < chapIdx) {
		t.Errorf("ordering within 인근 줄거리 wrong (parts-before-chapters): near=%d part=%d chap=%d", nearIdx, partIdx, chapIdx)
	}
}

func TestBuildUser_dossierIndentedUnderEntity(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Entities: []EntityBrief{
			{
				Name:   "해진",
				Kind:   "character",
				Recent: []string{"첫 등장", "두 번째"},
			},
		},
		UserPrompt: "확장",
	}
	usr := BuildMessages(c)[1].Content
	if !strings.Contains(usr, "\n  · 첫 등장\n") {
		t.Errorf("missing indented dossier line 1: %q", usr)
	}
	if !strings.Contains(usr, "\n  · 두 번째\n") {
		t.Errorf("missing indented dossier line 2: %q", usr)
	}
	// Dossier lines should appear after the entity bullet itself.
	entIdx := strings.Index(usr, "- @해진")
	dossIdx := strings.Index(usr, "  · 첫 등장")
	if entIdx < 0 || dossIdx < 0 || dossIdx < entIdx {
		t.Errorf("dossier order wrong: entIdx=%d dossIdx=%d", entIdx, dossIdx)
	}
}

func TestBuildUser_skipsEmptyHierarchicalSections(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		UserPrompt: "재작성",
	}
	usr := BuildMessages(c)[1].Content
	forbidden := []string{
		"## 작품 전반",
		"## 인근 줄거리",
		"## 같은 장 다른 씬",
		"## 직전·직후 씬 발췌",
		"## 관련 과거 씬",
		"## 직전 씬 발췌", // legacy header removed entirely
	}
	for _, s := range forbidden {
		if strings.Contains(usr, s) {
			t.Errorf("header %q should not appear when hierarchical is empty; got:\n%s", s, usr)
		}
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
