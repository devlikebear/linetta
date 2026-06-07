package ai

import (
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/plot"
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
	if !strings.Contains(sys, "500자 이내") {
		t.Errorf("short_form length cap missing: %q", sys)
	}
	if !strings.Contains(sys, "감각적 톤") {
		t.Errorf("tone fragment missing alongside short_form: %q", sys)
	}
}

func TestBuildMessages_contextSelectionOmitsDisabledSections(t *testing.T) {
	off := false
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "현재 씬 본문",
		Project: ProjectMeta{
			Genres:       []string{"미스터리"},
			LengthTarget: "novel",
			DefaultPOV:   "third_limited",
		},
		Outline: "작품 전체 개요",
		Hierarchical: HierarchicalContext{
			NearbyLeafSummaries: []SceneSummary{{Label: "씬 0", Body: "직전 씬"}},
		},
		RelatedScenes: []SceneSummary{{Label: "씬 9", Body: "관련 과거 씬"}},
		Entities:      []EntityBrief{{Name: "해진", Kind: "character", Summary: "사진작가"}},
		Relationships: []RelationBrief{{From: "해진", To: "도윤", Label: "동료"}},
		Plot: plot.Spine{
			Current: plot.SceneBeats{NodeID: "n1", Beats: []plot.Beat{{ThreadName: "첫 장면", Ordinal: 1, Label: "단서 발견", Description: "비밀 편지를 본다"}}},
		},
		Notes:      []NoteBrief{{Body: "작가 주석"}},
		StyleNotes: "내 문체",
		UserPrompt: "이어 써줘",
		Options: Options{Tone: TonePresetMy, Context: ContextSelection{
			CurrentScene:  &off,
			Overview:      &off,
			NearbyScenes:  &off,
			RelatedScenes: &off,
			Plot:          &off,
			Entities:      &off,
			Relationships: &off,
			Notes:         &off,
			ProjectMeta:   &off,
			StyleNotes:    &off,
		}},
	}

	msgs := BuildMessages(c)
	system := msgs[0].Content
	user := msgs[1].Content

	for _, gone := range []string{
		"내 문체",
		"작품 설정",
		"미스터리",
		"작품 전체 개요",
		"직전 씬",
		"관련 과거 씬",
		"현재 씬 본문",
		"@해진",
		"동료",
		"단서 발견",
		"작가 주석",
	} {
		if strings.Contains(system, gone) || strings.Contains(user, gone) {
			t.Fatalf("disabled context %q leaked into prompt\nsystem:\n%s\nuser:\n%s", gone, system, user)
		}
	}
	if !strings.Contains(user, "## 작가의 지시\n이어 써줘") {
		t.Fatalf("user instruction should remain in prompt:\n%s", user)
	}
}

func TestBuildMessagesRendersOutlineAndSynopsisSeparately(t *testing.T) {
	c := Context{
		SceneLabel: "씬",
		Project: ProjectMeta{
			Synopsis: "사건이 실제로 이렇게 전개된다.",
		},
		Outline:    "작가의 계획과 주제 메모.",
		UserPrompt: "이어 써줘",
	}
	user := BuildMessages(c)[1].Content
	for _, want := range []string{
		"## 작품 개요",
		"작가의 계획과 주제 메모.",
		"## 작품 시놉시스",
		"사건이 실제로 이렇게 전개된다.",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("prompt missing %q:\n%s", want, user)
		}
	}
}

func TestProjectMetaSelectionDoesNotDisableSynopsis(t *testing.T) {
	off := false
	c := Context{
		SceneLabel: "씬",
		Project: ProjectMeta{
			Genres:       []string{"미스터리"},
			LengthTarget: "novel",
			DefaultPOV:   "third_limited",
			Synopsis:     "사건 요약",
		},
		UserPrompt: "이어 써줘",
		Options: Options{Context: ContextSelection{
			ProjectMeta: &off,
		}},
	}
	user := BuildMessages(c)[1].Content
	if strings.Contains(user, "## 작품 설정") || strings.Contains(user, "미스터리") {
		t.Fatalf("project meta should be disabled:\n%s", user)
	}
	if !strings.Contains(user, "## 작품 시놉시스") || !strings.Contains(user, "사건 요약") {
		t.Fatalf("synopsis should remain enabled when project meta is disabled:\n%s", user)
	}
}

func TestPreviewFromContextRendersPlotSection(t *testing.T) {
	c := Context{
		Outline: "작품 전체 개요",
		Plot: plot.Spine{
			Current: plot.SceneBeats{
				NodeID: "n1",
				Label:  "현재",
				Beats:  []plot.Beat{{ThreadName: "첫 장면", Ordinal: 2, Label: "마지막 기회", Description: "주인공이 장소로 향한다"}},
			},
		},
	}

	preview := PreviewFromContext(c, DefaultContextSelection())
	var plotSection *PreviewSection
	for i := range preview.Sections {
		if preview.Sections[i].ID == ContextKeyPlot {
			plotSection = &preview.Sections[i]
			break
		}
	}
	if plotSection == nil {
		t.Fatal("missing plot preview section")
	}
	if !plotSection.Present || plotSection.Count != 1 {
		t.Fatalf("unexpected plot section metadata: %+v", *plotSection)
	}
	for _, want := range []string{"첫 장면", "마지막 기회", "주인공이 장소로 향한다"} {
		if !strings.Contains(plotSection.Preview, want) {
			t.Fatalf("plot preview missing %q: %+v", want, *plotSection)
		}
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

func TestBuildUserRendersPlotAndRelations(t *testing.T) {
	prev := plot.SceneBeats{NodeID: "p", Label: "1장 / 씬1", Beats: []plot.Beat{{ThreadName: "메인", Ordinal: 1, Label: "재회", Description: "항구에서"}}}
	c := Context{
		SceneLabel: "씬2", SceneText: "본문",
		Outline: "전체 개요",
		Plot: plot.Spine{
			Prev:    &prev,
			Current: plot.SceneBeats{NodeID: "c", Beats: []plot.Beat{{ThreadName: "메인", Ordinal: 2, Label: "발각", Description: "편지"}}},
		},
		Relationships: []RelationBrief{{From: "A", To: "B", Label: "라이벌", Bidirectional: true}},
		UserPrompt:    "확장해줘",
	}
	out := buildUser(c)
	for _, want := range []string{"## 작품 개요", "전체 개요", "## 플롯", "[이전 씬]", "[현재 씬]", "메인", "재회", "## 관계", "A ↔ B: 라이벌"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	for _, gone := range []string{"## 인근 줄거리", "## 같은 장 다른 씬", "## 활성 스토리라인"} {
		if strings.Contains(out, gone) {
			t.Fatalf("removed section %q still present", gone)
		}
	}
}

func TestOverviewDoesNotFallbackToDerivedSynopsis(t *testing.T) {
	c := Context{SceneLabel: "씬", UserPrompt: "x", Hierarchical: HierarchicalContext{ProjectSynopsis: "파생 시놉시스"}}
	out := buildUser(c)
	if strings.Contains(out, "파생 시놉시스") || strings.Contains(out, "## 작품 개요") || strings.Contains(out, "## 작품 시놉시스") {
		t.Fatalf("derived synopsis should not be injected unless stored on project:\n%s", out)
	}
}

func TestBuildUser_emitsHierarchicalSections(t *testing.T) {
	c := Context{
		SceneLabel: "씬 3",
		SceneText:  "씬 3 본문",
		Outline:    "작품 개요",
		Hierarchical: HierarchicalContext{
			ProjectSynopsis: "작품 시놉시스",
			NearbyLeafSummaries: []SceneSummary{
				{Label: "1부 / 1장 / 씬 1", Body: "씬 1 요약"},
				{Label: "1부 / 1장 / 씬 2", Body: "씬 2 요약"},
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
		"## 작품 개요",
		"## 직전·직후 씬 발췌",
		"## 관련 과거 씬",
		"## 현재 씬: 씬 3",
		"## 세계관 요소",
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
	if !strings.Contains(usr, "- [1부 / 1장 / 씬 1] 씬 1 요약") {
		t.Errorf("missing nearby bullet: %s", usr)
	}
	if !strings.Contains(usr, "- [1부 / 1장 / 씬 9] 관련 씬 요약") {
		t.Errorf("missing related-scene bullet: %s", usr)
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
		"## 작품 개요",
		"## 인근 줄거리",
		"## 같은 장 다른 씬",
		"## 직전·직후 씬 발췌",
		"## 관련 과거 씬",
		"## 플롯",
		"## 관계",
		"## 활성 스토리라인", // legacy header removed entirely
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

func TestBuildUser_projectMetaSection_fullyPopulated(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Project: ProjectMeta{
			Genres:       []string{"판타지", "미스터리"},
			LengthTarget: "novel",
			DefaultPOV:   "first",
		},
	}
	got := buildUser(c)
	want := "## 작품 설정\n장르: 판타지, 미스터리 · 분량: 장편 · 시점: 1인칭\n\n"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("expected prefix %q, got:\n%s", want, got)
	}
}

func TestBuildUser_projectMeta_canonicalSchemaValues(t *testing.T) {
	cases := []struct {
		name       string
		project    ProjectMeta
		wantSubstr string
	}{
		{"third_limited POV", ProjectMeta{DefaultPOV: "third_limited"}, "시점: 3인칭 제한"},
		{"flash length", ProjectMeta{LengthTarget: "flash"}, "분량: 플래시"},
		{"series length", ProjectMeta{LengthTarget: "series"}, "분량: 시리즈"},
		{"omniscient POV", ProjectMeta{DefaultPOV: "omniscient"}, "시점: 전지적"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Context{
				SceneLabel: "씬 1",
				SceneText:  "본문",
				Project:    tc.project,
			}
			got := buildUser(c)
			if !strings.Contains(got, tc.wantSubstr) {
				t.Fatalf("missing %q in:\n%s", tc.wantSubstr, got)
			}
		})
	}
}

func TestBuildUser_projectMetaSection_partial(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Project: ProjectMeta{
			LengthTarget: "short",
		},
	}
	got := buildUser(c)
	if !strings.Contains(got, "## 작품 설정") {
		t.Fatalf("missing header. got:\n%s", got)
	}
	if !strings.Contains(got, "분량: 단편") {
		t.Fatalf("missing length. got:\n%s", got)
	}
	if strings.Contains(got, "장르:") {
		t.Fatalf("genres should be omitted. got:\n%s", got)
	}
	if strings.Contains(got, "시점:") {
		t.Fatalf("POV should be omitted. got:\n%s", got)
	}
}

func TestBuildUser_projectMetaSection_allEmptyOmitsSection(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Project:    ProjectMeta{},
	}
	got := buildUser(c)
	if strings.Contains(got, "## 작품 설정") {
		t.Fatalf("section should be omitted entirely. got:\n%s", got)
	}
}

func TestBuildUser_projectMeta_unmappedValuesPassThrough(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Project: ProjectMeta{
			LengthTarget: "epic",
			DefaultPOV:   "second",
		},
	}
	got := buildUser(c)
	if !strings.Contains(got, "분량: epic") {
		t.Fatalf("unmapped length should pass through. got:\n%s", got)
	}
	if !strings.Contains(got, "시점: second") {
		t.Fatalf("unmapped POV should pass through. got:\n%s", got)
	}
}

func TestBuildUser_selectionTextSection_present(t *testing.T) {
	c := Context{
		SceneLabel:    "씬 1",
		SceneText:     "전체 본문 텍스트",
		SelectionText: "그녀는 천천히 고개를 들었다.",
	}
	got := buildUser(c)
	if !strings.Contains(got, "## 선택 영역") {
		t.Fatalf("missing header. got:\n%s", got)
	}
	if !strings.Contains(got, "그녀는 천천히 고개를 들었다.") {
		t.Fatalf("missing selection body. got:\n%s", got)
	}
}

func TestBuildUser_selectionTextSection_emptyOmitsSection(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
	}
	got := buildUser(c)
	if strings.Contains(got, "## 선택 영역") {
		t.Fatalf("section should be omitted. got:\n%s", got)
	}
}

func TestBuildUser_selectionTextSection_appearsAfterCurrentSceneBeforeInstruction(t *testing.T) {
	c := Context{
		SceneLabel:    "씬 1",
		SceneText:     "본문",
		SelectionText: "선택본",
		UserPrompt:    "지시문",
	}
	got := buildUser(c)
	sceneIdx := strings.Index(got, "## 현재 씬")
	selIdx := strings.Index(got, "## 선택 영역")
	instIdx := strings.Index(got, "## 작가의 지시")
	if sceneIdx == -1 || selIdx == -1 || instIdx == -1 {
		t.Fatalf("missing section. got:\n%s", got)
	}
	if !(sceneIdx < selIdx && selIdx < instIdx) {
		t.Fatalf("expected order: current scene < selection < instruction. got indices: scene=%d sel=%d inst=%d", sceneIdx, selIdx, instIdx)
	}
}
