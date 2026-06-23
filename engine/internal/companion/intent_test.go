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
