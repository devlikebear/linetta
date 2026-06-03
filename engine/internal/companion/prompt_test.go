package companion

import (
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func TestBuildSystem_HasProposalRules(t *testing.T) {
	s := buildSystem()
	for _, want := range []string{"집필 동료", "linetta-proposal", "create_thread", "add_beat", "linetta_apply_ops"} {
		if !strings.Contains(s, want) {
			t.Fatalf("system missing %q", want)
		}
	}
}

func TestBuildSystem_MentionsWebTools(t *testing.T) {
	s := buildSystem()
	for _, want := range []string{"web_search", "web_fetch"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing %q", want)
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
	out := buildContext(d)
	for _, want := range []string{"## 작품 개요", "전체 개요", "## 플롯", "[현재 씬]", "메인", "## 스토리라인", "[th1] 메인플롯"} {
		if !strings.Contains(out, want) {
			t.Fatalf("context missing %q in:\n%s", want, out)
		}
	}
}

func TestBuildContext_EmptyIsBlank(t *testing.T) {
	if out := buildContext(PromptData{}); out != "" {
		t.Fatalf("empty data should yield empty context, got %q", out)
	}
}

func TestBuildContext_RendersMemories(t *testing.T) {
	out := buildContext(PromptData{Memories: []string{"작가는 단문을 선호"}})
	if !strings.Contains(out, "## 기억") || !strings.Contains(out, "작가는 단문을 선호") {
		t.Fatalf("memories not rendered:\n%s", out)
	}
}

func TestBuildSystem_MentionsRemember(t *testing.T) {
	s := buildSystem()
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
	out := buildContext(d)
	if !strings.Contains(out, "## 씬") || !strings.Contains(out, "node-123") {
		t.Fatalf("scene ids not exposed:\n%s", out)
	}
}

func TestBuildSystem_ForbidsInventingIDs(t *testing.T) {
	s := buildSystem()
	if !strings.Contains(s, "지어내지 마") || !strings.Contains(s, "node_id") {
		t.Fatal("system prompt missing id-discipline guidance")
	}
}

func TestBuildContext_EntityShowsKind(t *testing.T) {
	d := PromptData{Entities: []entity.Entity{{ID: "e1", Kind: "character", Name: "하나", Role: "주인공"}}}
	out := buildContext(d)
	if !strings.Contains(out, "[e1] (인물) 하나") {
		t.Fatalf("entity kind not rendered:\n%s", out)
	}
}

func TestBuildSystem_MentionsEntityOps(t *testing.T) {
	s := buildSystem()
	for _, want := range []string{"create_entity", "create_relationship", "from_ref"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing %q", want)
		}
	}
}

func TestBuildSystem_MentionsSceneAndPair(t *testing.T) {
	s := buildSystem()
	for _, want := range []string{"create_scene", "create_outline_node", "parent_node_ref", "node_ref", "inverse_label"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing %q", want)
		}
	}
}
