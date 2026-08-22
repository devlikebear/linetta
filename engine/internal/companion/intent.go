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
	companionIntentReadOnly        companionIntentKind = "read_only"
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

// IsReadOnly reports whether the writer explicitly asked for a diagnosis,
// review, or critique without changing the work. Read-only turns must never
// mutate project state: the companion answers in prose and, when a change is
// worth making, offers it as a proposal block the writer applies.
func (i companionIntent) IsReadOnly() bool {
	return i.Kind == companionIntentReadOnly
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
	// The read-only check runs before the caller-supplied kind: an explicit
	// "do not change it, just diagnose" in the writer's own words outranks an
	// inferred or stale request kind, and refusing to mutate is recoverable
	// while an unwanted mutation is not.
	if isReadOnlyRequest(s) {
		return companionIntent{
			Kind:         companionIntentReadOnly,
			TargetNodeID: strings.TrimSpace(request.TargetNodeID),
		}
	}
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
	case string(companionIntentReadOnly), "readonly", "diagnose", "review":
		return companionIntentReadOnly
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

// isReadOnlyRequest matches turns where the writer either forbids changes
// outright ("수정하지 말고") or asks for diagnosis/review only ("제안만").
func isReadOnlyRequest(s string) bool {
	if s == "" {
		return false
	}
	return containsAny(s, companionReadOnlyGuardTerms) || containsAny(s, companionReadOnlyOnlyTerms)
}

func isSceneTextTarget(s string) bool {
	if containsAny(s, companionSceneBodyTerms) {
		return true
	}
	if !containsAny(s, []string{"씬", "장면", "scene", "シーン", "場面"}) {
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
	if isASCIIDigits(s) {
		return true
	}
	for _, marker := range []string{"번째", "번", "안", "째"} {
		before, after, ok := strings.Cut(s, marker)
		if !ok || !isASCIIDigits(before) {
			continue
		}
		if after == "" {
			return true
		}
		if strings.HasPrefix(after, "으로") {
			after = strings.TrimPrefix(after, "으로")
		}
		if after == "" || containsAny(after, companionSceneFollowupTerms) || containsAny(after, companionDirectApplyTerms) {
			return true
		}
	}
	return false
}

func isASCIIDigits(s string) bool {
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
	// English equivalents (input is lower-cased before matching).
	"current scene", "this scene", "scene text", "the scene",
	"manuscript", "prose", "the text", "next scene", "draft",
	// Japanese equivalents.
	"現在のシーン", "シーン本文", "場面の本文", "本文", "原稿", "文章",
	"次のシーン", "現在の場面",
}

var companionSceneWriteTerms = []string{
	"작성", "써", "쓰", "이어", "계속", "채워", "완성",
	// English equivalents.
	"write", "continue", "keep going", "finish", "fill", "complete", "draft",
	// Japanese equivalents.
	"書いて", "執筆", "続き", "続けて", "埋めて", "完成", "仕上げ",
}

var companionSceneRewriteTerms = []string{
	"수정", "바꿔", "변경", "반영", "다듬", "고쳐", "재작성", "교정", "퇴고",
	// English equivalents.
	"rewrite", "revise", "edit", "fix", "polish", "proofread", "refine",
	"improve", "tighten",
	// Japanese equivalents.
	"修正", "書き直", "変更", "反映", "推敲", "校正", "直して", "リライト",
}

var companionSceneFollowupTerms = []string{
	"적용", "진행", "바로", "좋아", "그걸로", "그대로", "작성해", "써줘", "써 줘",
	"완성해", "본문 작성", "현재 씬 작성", "첫번째", "첫 번째", "1번",
	// English equivalents.
	"apply", "go ahead", "do it", "sounds good", "that one", "option 1",
	"the first one", "proceed", "yes please",
	// Japanese equivalents.
	"適用", "進めて", "それで", "そのまま", "お願いします", "一番目", "1番",
}

var companionSceneOfferTargetTerms = []string{
	"현재 씬 본문", "현재 장면 본문", "씬 본문", "장면 본문", "현재 원고",
	"본문 작성", "현재 씬", "현재 장면", "이어질 문장", "문장 제안",
	// English equivalents.
	"current scene", "scene text", "the scene", "next sentences",
	"continuation",
	// Japanese equivalents.
	"現在のシーン本文", "シーン本文", "続きの文", "文の提案",
}

var companionSceneOfferActionTerms = []string{
	"작성", "써드", "써 드", "새로 씁", "완성", "적용", "이어", "확장",
	"연결부", "문체", "톤 맞춰",
	// English equivalents.
	"write", "draft", "continue", "expand", "extend", "apply",
	"match the tone",
	// Japanese equivalents.
	"書き", "適用", "続け", "拡張", "文体", "トーンに合わせ",
}

var companionSceneEmptyTerms = []string{
	"비워", "초기화", "비우", "삭제", "지워",
	// English equivalents.
	"clear", "empty", "erase", "reset", "wipe", "delete",
	// Japanese equivalents.
	"空にして", "初期化", "削除", "消して", "クリア",
}

var companionNonBodySceneTerms = []string{
	"비트", "플롯", "아웃라인", "목차", "구조", "나눠", "나누", "분할", "쪼개",
	// English equivalents.
	"beat", "plot", "outline", "structure", "split", "divide",
	"table of contents",
	// Japanese equivalents.
	"ビート", "プロット", "アウトライン", "目次", "構成", "分割", "分けて",
}

// companionReadOnlyGuardTerms are explicit "do not change it" markers. They
// stay tied to mutation verbs so that an ordinary negation ("아직 하지 마세요")
// does not disable applying.
var companionReadOnlyGuardTerms = []string{
	"수정하지 마", "수정하지마", "수정하지 말", "수정하지말",
	"고치지 마", "고치지마", "고치지 말", "고치지말",
	"바꾸지 마", "바꾸지마", "바꾸지 말", "바꾸지말",
	"변경하지 마", "변경하지마", "변경하지 말", "변경하지말",
	"적용하지 마", "적용하지마", "적용하지 말", "적용하지말",
	"저장하지 마", "저장하지마", "저장하지 말", "저장하지말",
	"반영하지 마", "반영하지마", "반영하지 말", "반영하지말",
	"덮어쓰지 마", "덮어쓰지마", "건드리지 마", "건드리지마",
	"읽기 전용", "읽기전용",
	// English equivalents (input is lower-cased before matching).
	"don't modify", "do not modify", "don't change", "do not change",
	"don't edit", "do not edit", "don't rewrite", "do not rewrite",
	"don't apply", "do not apply", "don't save", "do not save",
	"without changing", "without modifying", "read-only", "read only",
	// Japanese equivalents.
	"修正しないで", "変更しないで", "書き換えないで", "適用しないで",
	"保存しないで", "触らないで", "読み取り専用",
}

// companionReadOnlyOnlyTerms are "diagnosis/review only" markers: the writer
// wants an assessment, not an edit.
var companionReadOnlyOnlyTerms = []string{
	"진단만", "평가만", "분석만", "검토만", "리뷰만", "비평만",
	"제안만", "의견만", "피드백만", "지적만", "설명만",
	// English equivalents.
	"diagnose only", "diagnosis only", "review only", "analysis only",
	"analyze only", "feedback only", "suggestions only", "critique only",
	"just diagnose", "just review", "just analyze", "just critique",
	// Japanese equivalents.
	"診断だけ", "評価だけ", "分析だけ", "レビューだけ", "提案だけ",
	"フィードバックだけ", "指摘だけ",
}
