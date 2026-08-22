package companion

import "testing"

func TestResolveCompanionIntentPromotesSceneWriteFollowups(t *testing.T) {
	previous := []conversationMessage{{
		Role: "assistant",
		Content: "원하시면 제가 바로 현재 씬 본문을 완성해서 적용할 수 있습니다. " +
			"가능한 작업은 현재 씬 본문 작성, 현재 씬 확장, 연결부 작성입니다.",
	}}

	for _, text := range []string{"1번", "적용해줘", "좋아 바로 적용", "본문 작성"} {
		got := resolveCompanionIntentWithConversation(text, RequestIntent{}, previous)
		if got.Kind != companionIntentSceneWrite {
			t.Fatalf("resolveCompanionIntentWithConversation(%q) = %q, want %q", text, got.Kind, companionIntentSceneWrite)
		}
	}
}

func TestResolveCompanionIntentPromotesNumberedSceneChoiceApply(t *testing.T) {
	previous := []conversationMessage{{
		Role: "assistant",
		Content: "## 1권 / 1화 씬에 이어질 문장 제안\n\n" +
			"**제안 1 (추천):** 어둠이 짙게 깔린 밤, 낡은 창고의 삐걱이는 문틈으로 스며든 공기가 그의 뺨을 스쳤다.\n\n" +
			"**제안 2:** 싸늘한 밤공기를 가르며 낡은 창고 문이 삐걱였다.\n\n" +
			"어떤 제안이 마음에 드시나요?",
	}}

	got := resolveCompanionIntentWithConversation("1안으로 적용", RequestIntent{}, previous)
	if got.Kind != companionIntentSceneWrite {
		t.Fatalf("numbered scene choice apply = %q, want %q", got.Kind, companionIntentSceneWrite)
	}
}

func TestResolveCompanionIntentDoesNotPromoteUnrelatedFollowups(t *testing.T) {
	got := resolveCompanionIntentWithConversation("1번", RequestIntent{}, []conversationMessage{{
		Role:    "assistant",
		Content: "아이디어를 세 가지로 정리했습니다. 1번은 자료 조사, 2번은 설정 검토입니다.",
	}})
	if got.Kind != companionIntentChat {
		t.Fatalf("unrelated numeric followup = %q, want chat", got.Kind)
	}

	got = resolveCompanionIntentWithConversation("적용해줘", RequestIntent{Kind: "chat"}, []conversationMessage{{
		Role:    "assistant",
		Content: "현재 씬 본문을 완성해서 적용할 수 있습니다.",
	}})
	if got.Kind != companionIntentChat {
		t.Fatalf("explicit chat intent should not be promoted: %q", got.Kind)
	}
}

func TestClassifyCompanionIntentDetectsSceneTextRequests(t *testing.T) {
	for _, tc := range []struct {
		text string
		kind companionIntentKind
	}{
		{"아니 1장 1씬 작성해달라고", companionIntentSceneWrite},
		{"1부 1장 씬 1 본문 작성해줘", companionIntentSceneWrite},
		{"현재 씬 본문 써줘", companionIntentSceneWrite},
		{"다음 씬 작성하자", companionIntentSceneWrite},
		{"이 문장 다듬어줘", companionIntentSceneRewrite},
		{"현재 원고를 감정선 중심으로 재작성해줘", companionIntentSceneRewrite},
		{"씬 작성법 알려줘", companionIntentChat},
		{"이 설정 어때?", companionIntentChat},
		{"이 씬을 더 구체화해서 비트로 확장해줘", companionIntentGenericMutation},
	} {
		got := classifyCompanionIntent(tc.text)
		if got.Kind != tc.kind {
			t.Fatalf("classifyCompanionIntent(%q) = %q, want %q", tc.text, got.Kind, tc.kind)
		}
	}
}

func TestCompanionToolChoiceForUserTextUsesSceneIntent(t *testing.T) {
	for _, text := range []string{
		"아니 1장 1씬 작성해달라고",
		"다음 씬 작성하자",
		"현재 씬 본문 써줘",
	} {
		if got := companionForcedToolForUserText(text); got != applyOpsToolName {
			t.Fatalf("companionForcedToolForUserText(%q) = %q, want %q", text, got, applyOpsToolName)
		}
	}
}

func TestResolveCompanionIntentUsesExplicitRequestIntent(t *testing.T) {
	got := resolveCompanionIntent("아이디어만 말해줘", RequestIntent{Kind: "scene_write", TargetNodeID: "node-1"})
	if got.Kind != companionIntentSceneWrite || got.TargetNodeID != "node-1" {
		t.Fatalf("explicit intent not applied: %+v", got)
	}

	got = resolveCompanionIntent("현재 씬을 비워줘", RequestIntent{Kind: "scene_rewrite"})
	if got.Kind != companionIntentSceneRewrite || !got.AllowEmptySceneText {
		t.Fatalf("explicit intent should preserve clear-text allowance: %+v", got)
	}
}

// The reported case: a diagnosis-only request must never be treated as a
// mutation turn, so the companion cannot silently update work memory.
func TestResolveCompanionIntentDetectsReadOnlyRequests(t *testing.T) {
	for _, text := range []string{
		"현재 1화 초안을 수정하지 말고 먼저 진단해줘. 문체와 개연성을 평가하고 수정 제안만 제시해줘.",
		"본문은 바꾸지 말고 개연성만 짚어줘",
		"이 씬 고치지 말고 검토만 해줘",
		"작품 기억에 저장하지 말고 의견만 말해줘",
		"Review only: do not change the scene, just tell me what is weak.",
		"シーン本文は修正しないで、診断だけしてください。",
		"아직 적용하지 말고 진행해도 될지 알려줘",
	} {
		if got := classifyCompanionIntent(text); !got.IsReadOnly() {
			t.Fatalf("classifyCompanionIntent(%q).Kind = %q, want read_only", text, got.Kind)
		}
	}
}

// An explicit read-only phrase in the writer's own words outranks a caller
// supplied intent kind: refusing to mutate is recoverable, mutating is not.
func TestResolveCompanionIntentReadOnlyOverridesRequestKind(t *testing.T) {
	got := resolveCompanionIntent("본문을 수정하지 말고 진단만 해줘", RequestIntent{Kind: "scene_write", TargetNodeID: "node-1"})
	if !got.IsReadOnly() {
		t.Fatalf("read-only text with scene_write request = %q, want read_only", got.Kind)
	}
	if got.TargetNodeID != "node-1" {
		t.Fatalf("read-only intent should keep the target node: %+v", got)
	}
	if got.RequiresApplyOps() || got.RequiresSceneText() {
		t.Fatalf("read-only intent must not require apply ops: %+v", got)
	}
	if companionForcedToolForIntent(got) != "" {
		t.Fatalf("read-only intent must not force the apply tool")
	}
}

// Ordinary revise/apply requests must keep working.
func TestResolveCompanionIntentKeepsMutationRequests(t *testing.T) {
	for _, tc := range []struct {
		text string
		kind companionIntentKind
	}{
		{"현재 씬 본문 다듬어줘", companionIntentSceneRewrite},
		{"현재 씬 본문 써줘", companionIntentSceneWrite},
		{"이 씬을 더 구체화해서 비트로 확장해줘", companionIntentGenericMutation},
	} {
		if got := classifyCompanionIntent(tc.text); got.Kind != tc.kind {
			t.Fatalf("classifyCompanionIntent(%q) = %q, want %q", tc.text, got.Kind, tc.kind)
		}
	}
}
