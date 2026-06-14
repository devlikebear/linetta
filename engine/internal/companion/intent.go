package companion

import "strings"

type RequestIntent struct {
	Kind         string `json:"kind,omitempty"`
	TargetNodeID string `json:"target_node_id,omitempty"`
	ApplyPolicy  string `json:"apply_policy,omitempty"`
}

type companionIntentKind string

const (
	companionIntentChat            companionIntentKind = "chat"
	companionIntentGenericMutation companionIntentKind = "generic_mutation"
	companionIntentSceneWrite      companionIntentKind = "scene_write"
	companionIntentSceneRewrite    companionIntentKind = "scene_rewrite"
)

type companionIntent struct {
	Kind                companionIntentKind
	AllowEmptySceneText bool
	TargetNodeID        string
}

func (i companionIntent) RequiresApplyOps() bool {
	return i.Kind == companionIntentGenericMutation || i.RequiresSceneText()
}

func (i companionIntent) RequiresSceneText() bool {
	return i.Kind == companionIntentSceneWrite || i.Kind == companionIntentSceneRewrite
}

func classifyCompanionIntent(text string) companionIntent {
	return resolveCompanionIntent(text, RequestIntent{})
}

func resolveCompanionIntent(text string, request RequestIntent) companionIntent {
	s := strings.ToLower(strings.TrimSpace(text))
	if kind := normalizeRequestIntentKind(request.Kind); kind != "" {
		return companionIntent{
			Kind:                kind,
			AllowEmptySceneText: containsAny(s, companionSceneEmptyTerms),
			TargetNodeID:        strings.TrimSpace(request.TargetNodeID),
		}
	}
	if s == "" {
		return companionIntent{Kind: companionIntentChat}
	}
	if containsAny(s, companionEducationalTerms) || containsAny(s, companionResearchTerms) {
		return companionIntent{Kind: companionIntentChat}
	}
	if containsAny(s, companionDiscussionTerms) && !containsAny(s, companionDirectApplyTerms) {
		return companionIntent{Kind: companionIntentChat}
	}
	if isSceneTextTarget(s) && containsAny(s, companionMutationTerms) {
		kind := companionIntentSceneWrite
		if containsAny(s, companionSceneRewriteTerms) {
			kind = companionIntentSceneRewrite
		}
		return companionIntent{
			Kind:                kind,
			AllowEmptySceneText: containsAny(s, companionSceneEmptyTerms),
			TargetNodeID:        strings.TrimSpace(request.TargetNodeID),
		}
	}
	if containsAny(s, companionStructureTerms) && containsAny(s, companionMutationTerms) {
		return companionIntent{Kind: companionIntentGenericMutation}
	}
	return companionIntent{Kind: companionIntentChat}
}

func normalizeRequestIntentKind(kind string) companionIntentKind {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case string(companionIntentSceneWrite):
		return companionIntentSceneWrite
	case string(companionIntentSceneRewrite), "scene_proofread":
		return companionIntentSceneRewrite
	case string(companionIntentGenericMutation), "outline_mutation":
		return companionIntentGenericMutation
	case string(companionIntentChat):
		return companionIntentChat
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isSceneTextTarget(s string) bool {
	if containsAny(s, companionSceneBodyTerms) {
		return true
	}
	if !containsAny(s, []string{"씬", "장면"}) {
		return false
	}
	if containsAny(s, companionNonBodySceneTerms) {
		return false
	}
	return containsAny(s, companionSceneWriteTerms) || containsAny(s, companionSceneRewriteTerms)
}

var companionSceneBodyTerms = []string{
	"현재 씬", "현재 장면", "씬 본문", "장면 본문", "현재 본문", "현재 원고",
	"본문", "원고", "문장", "다음 씬", "다음 장면",
}

var companionSceneWriteTerms = []string{
	"작성", "써", "쓰", "이어", "계속", "채워", "완성",
}

var companionSceneRewriteTerms = []string{
	"수정", "바꿔", "변경", "반영", "다듬", "고쳐", "재작성", "교정", "퇴고",
}

var companionSceneEmptyTerms = []string{
	"비워", "초기화", "비우", "삭제", "지워",
}

var companionNonBodySceneTerms = []string{
	"비트", "플롯", "아웃라인", "목차", "구조", "나눠", "나누", "분할", "쪼개",
}
