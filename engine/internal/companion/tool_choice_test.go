package companion

import (
	"context"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

func TestCompanionToolChoiceForUserText_ForcesApplyOpsForDirectMutations(t *testing.T) {
	for _, text := range []string{
		"스토리라인에 복수극 라인을 추가해줘",
		"캐릭터 관계를 더 긴장감 있게 수정해줘",
		"비트 하나 만들어서 현재 씬에 붙여줘",
		"장소 설정을 정리해서 저장해줘",
		"소설 개요 작성해줘",
		"시놉시스를 새로 써줘",
		"챕터별 아웃라인을 잡아줘",
	} {
		tool := companionForcedToolForUserText(text)
		if tool != applyOpsToolName {
			t.Fatalf("companionForcedToolForUserText(%q) = %q, want %q", text, tool, applyOpsToolName)
		}
	}
}

func TestFirstTurnToolChoiceClientOnlyForcesFirstChat(t *testing.T) {
	base := &recordingToolChoiceClient{}
	client := newFirstTurnToolChoiceClient(base, applyOpsToolName)
	tools := []llm.ToolSchema{
		{Type: "function", Function: llm.ToolFunctionSchema{Name: applyOpsToolName}},
		{Type: "function", Function: llm.ToolFunctionSchema{Name: "web_search"}},
	}

	for i := 0; i < 2; i++ {
		if _, err := client.Chat(context.Background(), nil, llm.ChatOptions{Tools: tools}); err != nil {
			t.Fatalf("Chat: %v", err)
		}
	}

	if len(base.choices) != 2 {
		t.Fatalf("choices = %+v", base.choices)
	}
	if base.choices[0] == nil || base.choices[0].Mode != llm.ToolChoiceModeRequired {
		t.Fatalf("first choice = %+v, want required", base.choices[0])
	}
	if got := toolSchemaNames(base.tools[0]); len(got) != 1 || got[0] != applyOpsToolName {
		t.Fatalf("first tools = %+v, want only %s", got, applyOpsToolName)
	}
	if base.choices[1] != nil {
		t.Fatalf("second choice = %+v, want nil auto", base.choices[1])
	}
	if got := toolSchemaNames(base.tools[1]); len(got) != 2 {
		t.Fatalf("second tools = %+v, want original tools", got)
	}
}

type recordingToolChoiceClient struct {
	choices []*llm.ToolChoice
	tools   [][]llm.ToolSchema
}

func (c *recordingToolChoiceClient) Ask(context.Context, string) (string, error) {
	return "", nil
}

func (c *recordingToolChoiceClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.choices = append(c.choices, opts.ToolChoice)
	c.tools = append(c.tools, opts.Tools)
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}, nil
}

func toolSchemaNames(tools []llm.ToolSchema) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Function.Name)
	}
	return names
}

func TestCompanionToolChoiceForUserText_LeavesDiscussionOnAuto(t *testing.T) {
	for _, text := range []string{
		"이 캐릭터 관계 어때?",
		"플롯 아이디어 세 개만 추천해줘",
		"이 장면의 분위기를 설명해줘",
		"최신 자료를 검색해서 장소 설정에 반영해줘",
		"소설 개요 작성법 알려줘",
		"시놉시스는 어떻게 쓰면 돼?",
	} {
		if tool := companionForcedToolForUserText(text); tool != "" {
			t.Fatalf("companionForcedToolForUserText(%q) = %q, want empty auto", text, tool)
		}
	}
}
