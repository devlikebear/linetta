package companion

import (
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func TestBuildSystem_HasProposalRules(t *testing.T) {
	s := buildSystem("")
	for _, want := range []string{"집필 동료", "linetta-proposal", "create_thread", "add_beat", "linetta_apply_ops"} {
		if !strings.Contains(s, want) {
			t.Fatalf("system missing %q", want)
		}
	}
}

func TestBuildSystem_EnglishLanguage(t *testing.T) {
	s := buildSystem("en")
	for _, want := range []string{"writing companion for a fiction writer", "linetta-proposal", "create_thread", "add_beat", "linetta_apply_ops"} {
		if !strings.Contains(s, want) {
			t.Fatalf("english system missing %q", want)
		}
	}
	if strings.Contains(s, "집필 동료") {
		t.Fatal("english system prompt still contains Korean persona line")
	}
}

func TestBuildContext_EnglishLabels(t *testing.T) {
	d := PromptData{
		Outline:  "A detective hunts a vanished message.",
		Memories: []string{"The writer prefers short sentences."},
	}
	out := buildContext(d, "en")
	for _, want := range []string{"## Synopsis", "## Memories"} {
		if !strings.Contains(out, want) {
			t.Fatalf("english context missing %q", want)
		}
	}
	if strings.Contains(out, "## 작품 개요") || strings.Contains(out, "## 기억") {
		t.Fatalf("english context still contains Korean headers: %q", out)
	}
}

func TestBuildSystem_JapaneseLanguage(t *testing.T) {
	s := buildSystem("ja")
	for _, want := range []string{"執筆コンパニオン", "常に日本語で答えて", "linetta-proposal", "create_thread", "add_beat", "linetta_apply_ops"} {
		if !strings.Contains(s, want) {
			t.Fatalf("japanese system missing %q", want)
		}
	}
	if strings.Contains(s, "집필 동료") {
		t.Fatal("japanese system prompt still contains Korean persona line")
	}
}

func TestBuildContext_JapaneseLabels(t *testing.T) {
	d := PromptData{
		Outline:  "探偵が消えたメッセージを追う。",
		Memories: []string{"作家は短い文を好む。"},
	}
	out := buildContext(d, "ja")
	for _, want := range []string{"## 作品概要", "## 記憶"} {
		if !strings.Contains(out, want) {
			t.Fatalf("japanese context missing %q", want)
		}
	}
	if strings.Contains(out, "## 작품 개요") || strings.Contains(out, "## 기억") {
		t.Fatalf("japanese context still contains Korean headers: %q", out)
	}
}

func TestBuildSystem_MentionsWebTools(t *testing.T) {
	s := buildSystem("")
	for _, want := range []string{"web_search", "web_fetch"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing %q", want)
		}
	}
}

func TestBuildSystem_MentionsManuscriptSearchTool(t *testing.T) {
	s := buildSystem("")
	for _, want := range []string{"search_manuscript", "get_scene_text", "본문 전체", "node_id"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing manuscript search guidance %q", want)
		}
	}
}

func TestBuildSystem_MentionsProofreadRules(t *testing.T) {
	s := buildSystem("")
	for _, want := range []string{"맞춤법", "고유명사", "대사 톤", "변경 목록"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing proofread rule %q", want)
		}
	}
}

func TestBuildContext_RendersSections(t *testing.T) {
	d := PromptData{
		Outline:  "전체 개요",
		HasSpine: true,
		Spine: plot.Spine{
			Current: plot.SceneBeats{NodeID: "n1", Beats: []plot.Beat{{ThreadName: "메인", Label: "발단", Description: "주인공 등장"}}},
		},
		Threads: []thread.Thread{{ID: "th1", Name: "메인플롯", Summary: "중심 줄기"}},
	}
	out := buildContext(d, "")
	for _, want := range []string{"## 작품 개요", "전체 개요", "## 플롯", "[현재 씬]", "메인", "## 스토리라인", "[th1] 메인플롯"} {
		if !strings.Contains(out, want) {
			t.Fatalf("context missing %q in:\n%s", want, out)
		}
	}
}

func TestBuildContext_EmptyIsBlank(t *testing.T) {
	if out := buildContext(PromptData{}, ""); out != "" {
		t.Fatalf("empty data should yield empty context, got %q", out)
	}
}

func TestBuildContext_RendersMemories(t *testing.T) {
	out := buildContext(PromptData{Memories: []string{"작가는 단문을 선호"}}, "")
	if !strings.Contains(out, "## 기억") || !strings.Contains(out, "작가는 단문을 선호") {
		t.Fatalf("memories not rendered:\n%s", out)
	}
}

func TestBuildContext_RendersSceneExcerpts(t *testing.T) {
	out := buildContext(PromptData{
		SceneExcerpts: []SceneExcerpt{{
			NodeID: "n1",
			Label:  "1부 / 1장 / 씬 1",
			Text:   "해진은 민호를 믿지 못한 채 항구를 떠났다.",
		}},
	}, "")
	for _, want := range []string{"## 작성된 본문 발췌", "[n1] 1부 / 1장 / 씬 1", "해진은 민호를 믿지 못한 채"} {
		if !strings.Contains(out, want) {
			t.Fatalf("context missing %q in:\n%s", want, out)
		}
	}
}

func TestBuildSystem_MentionsRemember(t *testing.T) {
	s := buildSystem("")
	if !strings.Contains(s, "remember") || !strings.Contains(s, "기억") {
		t.Fatal("buildSystem missing remember/memory guidance")
	}
}

func TestBuildContext_ExposesSceneIDs(t *testing.T) {
	d := PromptData{
		HasSpine: true,
		Spine: plot.Spine{
			Current: plot.SceneBeats{NodeID: "node-123", Label: "1부 / 1장 / 씬1"},
		},
	}
	out := buildContext(d, "")
	if !strings.Contains(out, "## 씬") || !strings.Contains(out, "node-123") {
		t.Fatalf("scene ids not exposed:\n%s", out)
	}
}

func TestBuildContext_RendersOutlineTreeIDs(t *testing.T) {
	out := buildContext(PromptData{
		OutlineNodes: []OutlineNode{
			{ID: "part-1", ParentID: "", Kind: "container", Label: "1부", Title: "경계의 틈", Depth: 0},
			{ID: "chapter-1", ParentID: "part-1", Kind: "container", Label: "1장", Title: "불안한 아침", Depth: 1},
			{ID: "scene-1", ParentID: "chapter-1", Kind: "leaf", Label: "씬 1", Title: "조각난 자아", Depth: 2},
		},
	}, "")
	for _, want := range []string{"## 아웃라인 트리", "[part-1]", "1부", "[chapter-1]", "1장", "[scene-1]", "씬 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("outline tree context missing %q in:\n%s", want, out)
		}
	}
}

func TestBuildContext_RendersOutlineStructurePreset(t *testing.T) {
	out := buildContext(PromptData{
		OutlineStructure: "웹소설: 권 > 화 > 씬 (예: 1권 > 1화 > 씬 1)",
	}, "")
	for _, want := range []string{"## 아웃라인 구조 프리셋", "웹소설: 권 > 화 > 씬", "계층과 라벨 예시"} {
		if !strings.Contains(out, want) {
			t.Fatalf("outline structure context missing %q in:\n%s", want, out)
		}
	}
}

func TestBuildSystem_ForbidsInventingIDs(t *testing.T) {
	s := buildSystem("")
	if !strings.Contains(s, "지어내지 마") || !strings.Contains(s, "node_id") {
		t.Fatal("system prompt missing id-discipline guidance")
	}
}

func TestBuildContext_EntityShowsKind(t *testing.T) {
	d := PromptData{Entities: []entity.Entity{{
		ID: "e1", Kind: "concept", Name: "빛의 맹약", Role: "마법",
		Attributes: map[string]string{"효과": "거짓을 드러낸다", "비용": "기억 한 조각"},
	}}}
	out := buildContext(d, "")
	for _, want := range []string{"## 세계관 요소·관계", "[e1] (개념) 빛의 맹약 / 마법", "비용:기억 한 조각", "효과:거짓을 드러낸다"} {
		if !strings.Contains(out, want) {
			t.Fatalf("entity context missing %q:\n%s", want, out)
		}
	}
}

func TestBuildSystem_MentionsEntityOps(t *testing.T) {
	s := buildSystem("")
	for _, want := range []string{"create_entity", "create_relationship", "from_ref", "attributes", "아이템", "스킬", "마법", "능력"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing %q", want)
		}
	}
}

func TestBuildSystem_MentionsSceneAndPair(t *testing.T) {
	s := buildSystem("")
	for _, want := range []string{"create_scene", "create_outline_node", "set_scene_text", "현재 씬", "parent_node_ref", "node_ref", "inverse_label"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing %q", want)
		}
	}
}

func TestBuildSystem_MentionsFactBookRules(t *testing.T) {
	s := buildSystem("")
	for _, want := range []string{"create_fact_card", "출처 URL", "status", "팩트 자료집"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing %q", want)
		}
	}
}

func TestBuildContext_RendersFactBook(t *testing.T) {
	out := buildContext(PromptData{
		Facts: []fact.Card{{
			ID:     "f1",
			Claim:  "런던 일반 경찰은 항상 총기를 휴대한다",
			Result: "일반 경찰은 통상 비무장이다.",
			Status: fact.StatusVerified,
			Sources: []fact.Source{{
				URL:   "https://www.met.police.uk/",
				Title: "Met Police",
			}},
		}},
	}, "")
	for _, want := range []string{"## 팩트 자료집", "[f1]", "verified", "https://www.met.police.uk/"} {
		if !strings.Contains(out, want) {
			t.Fatalf("context missing %q in:\n%s", want, out)
		}
	}
}
