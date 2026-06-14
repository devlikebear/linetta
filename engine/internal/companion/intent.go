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

type conversationMessage struct {
	Role    string
	Content string
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

func resolveCompanionIntentWithConversation(text string, request RequestIntent, history []conversationMessage) companionIntent {
	if normalizeRequestIntentKind(request.Kind) != "" {
		return resolveCompanionIntent(text, request)
	}
	intent := resolveCompanionIntent(text, request)
	if intent.Kind != companionIntentChat {
		return intent
	}
	if isSceneWriteFollowup(text) && recentAssistantOfferedSceneWrite(history) {
		return companionIntent{
			Kind:                companionIntentSceneWrite,
			AllowEmptySceneText: containsAny(strings.ToLower(strings.TrimSpace(text)), companionSceneEmptyTerms),
			TargetNodeID:        strings.TrimSpace(request.TargetNodeID),
		}
	}
	return intent
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

func isSceneWriteFollowup(text string) bool {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return false
	}
	if isNumberedChoiceFollowup(s) {
		return true
	}
	if len([]rune(s)) > 48 {
		return false
	}
	if isSceneTextTarget(s) && containsAny(s, companionMutationTerms) {
		return true
	}
	return containsAny(s, companionSceneFollowupTerms)
}

func isNumberedChoiceFollowup(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, ".。．!！?？")
	s = strings.ReplaceAll(s, " ", "")
	for _, suffix := range []string{"번", "안", "번째", "째"} {
		s = strings.TrimSuffix(s, suffix)
	}
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func recentAssistantOfferedSceneWrite(history []conversationMessage) bool {
	checked := 0
	for i := len(history) - 1; i >= 0 && checked < 4; i-- {
		msg := history[i]
		if msg.Role != "assistant" {
			continue
		}
		checked++
		s := strings.ToLower(strings.TrimSpace(msg.Content))
		if s == "" {
			continue
		}
		if containsAny(s, companionSceneOfferTargetTerms) && containsAny(s, companionSceneOfferActionTerms) {
			return true
		}
	}
	return false
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

var companionSceneFollowupTerms = []string{
	"적용", "진행", "바로", "좋아", "그걸로", "그대로", "작성해", "써줘", "써 줘",
	"완성해", "본문 작성", "현재 씬 작성", "첫번째", "첫 번째", "1번",
}

var companionSceneOfferTargetTerms = []string{
	"현재 씬 본문", "현재 장면 본문", "씬 본문", "장면 본문", "현재 원고",
	"본문 작성", "현재 씬", "현재 장면",
}

var companionSceneOfferActionTerms = []string{
	"작성", "써드", "써 드", "새로 씁", "완성", "적용", "이어", "확장",
	"연결부", "문체", "톤 맞춰",
}

var companionSceneEmptyTerms = []string{
	"비워", "초기화", "비우", "삭제", "지워",
}

var companionNonBodySceneTerms = []string{
	"비트", "플롯", "아웃라인", "목차", "구조", "나눠", "나누", "분할", "쪼개",
}
