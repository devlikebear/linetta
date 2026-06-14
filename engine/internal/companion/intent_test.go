package companion

import "testing"

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
