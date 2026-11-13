package mcphost

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Prompts are the work's entry points: "start this piece of work now", one
// user message each. The agent panel's chips and an external client's slash
// commands (/linetta:draft_current_scene in Claude Code) are both read from
// the definitions below, so the two cannot drift apart (#133).
//
// A prompt is not a skill. A skill says how a kind of work is done, and the
// writer or the agent edits it; a prompt only says what to start. A prompt may
// name a tool or a skill but never carries the method itself, and never the
// story brief or the manuscript: the agent reads those through
// linetta_get_story_context like any other turn.

// PromptSlotsMeta is the _meta key on a listed prompt naming where the panel
// shows it: "start" before the first turn, "followup" once there is writing to
// check, "longform" under the long-form tools button. An external client
// ignores it.
const PromptSlotsMeta = "linetta/slots"

type promptDef struct {
	name  string
	slots []string
	// tools are what the prompt asks the agent to use. A server without all
	// of them does not list the prompt: an entry point whose first step is
	// refused is worse than none.
	tools []string
	title map[string]string
	text  map[string]string
}

var promptDefs = []promptDef{
	{
		name:  "draft_current_scene",
		slots: []string{"start"},
		tools: []string{"linetta_write_scene"},
		title: map[string]string{"ko": "현재 씬 초고", "en": "Draft the current scene", "ja": "現在のシーンの下書き"},
		text:  map[string]string{"ko": "지금 열려 있는 씬의 초고를 써줘.", "en": "Write a draft of the scene that's currently open.", "ja": "今開いているシーンの下書きを書いて。"},
	},
	{
		name:  "check_continuity",
		slots: []string{"start", "followup"},
		tools: []string{"linetta_report_continuity"},
		title: map[string]string{"ko": "연속성 점검", "en": "Check continuity", "ja": "連続性チェック"},
		text:  map[string]string{"ko": "지금 씬을 Story World와 앞 회차에 대조해 연속성을 점검하고 결과를 기록해 줘. 원고는 고치지 마.", "en": "Check this scene's continuity against Story World and earlier chapters, and record what you find. Do not change the manuscript.", "ja": "今のシーンをStory Worldと前の話と照らし合わせて連続性を確認し、結果を記録して。原稿は直さないで。"},
	},
	{
		name:  "fill_story_world",
		slots: []string{"start"},
		tools: []string{"linetta_propose_story_ops"},
		title: map[string]string{"ko": "Story World 채우기", "en": "Fill Story World", "ja": "Story Worldを満たす"},
		text:  map[string]string{"ko": "가져온 원고를 회차 순서대로 읽으면서 인물·장소·관계·팩트 후보를 제안함에 올려줘. 진행률을 확인해서 아직 다루지 않은 회차부터 시작하고, 도구 호출 한도에 가까워지면 어디까지 했는지 알려줘.", "en": "Read the imported manuscript in episode order and propose character, place, relationship and fact candidates to the suggestions inbox. Check the progress first and start from the first episode not yet covered; if you are close to your tool-call budget, say how far you got.", "ja": "取り込んだ原稿を話数の順に読みながら、人物・場所・関係・ファクトの候補を提案箱に提案して。進捗を確認してまだ扱っていない話数から始め、ツール呼び出しの上限に近づいたらどこまで進んだか伝えて。"},
	},
	{
		name:  "suggest_next_scene",
		slots: []string{"start"},
		tools: []string{"linetta_get_story_context"},
		title: map[string]string{"ko": "다음 씬 제안", "en": "Suggest the next scene", "ja": "次のシーンを提案"},
		text:  map[string]string{"ko": "다음 씬으로 무엇을 쓰면 좋을지 제안해줘.", "en": "Suggest what the next scene should be.", "ja": "次のシーンに何を書けばいいか提案して。"},
	},
	{
		name:  "forecast_next_episode",
		slots: []string{"start", "followup"},
		tools: []string{"linetta_save_forecast"},
		title: map[string]string{"ko": "다음 전개 갈래", "en": "Next development branches", "ja": "次の展開の分岐"},
		text:  map[string]string{"ko": "다음 회차 전개 갈래 3개를 만들어 비교할 수 있게 저장해 줘.", "en": "Draft 3 possible branches for the next episode and save them so I can compare them.", "ja": "次の話の展開の分岐を3つ作って、比較できるように保存して。"},
	},
	{
		name:  "review_episode",
		slots: []string{"followup"},
		tools: []string{"linetta_report_continuity"},
		title: map[string]string{"ko": "이 화 검수", "en": "Review this episode", "ja": "この話を検収"},
		text:  map[string]string{"ko": "지금 화를 이 화만 읽은 독자의 눈으로 검수하고, 앞 회차를 읽은 독자의 눈으로 다시 읽어 줘. 연속성, 인물 동기, 가독성, 훅과 전환과 엔딩을 보고 찾은 것을 기록해 줘. 원고는 고치지 마.", "en": "Review the current episode as a reader who saw only this one, then as one who read everything before it. Check continuity, motives, readability, and the hook, turn and ending, and record what you find. Don't change the manuscript.", "ja": "今の話を、この話だけを読んだ読者の目で検収し、前の話まで読んだ読者の目で読み直してください。連続性、人物の動機、読みやすさ、導入と転換と結びを確認し、見つけたことを記録してください。原稿は変えないでください。"},
	},
	{
		name:  "review_part",
		slots: []string{"followup"},
		tools: []string{"linetta_list_ledger"},
		title: map[string]string{"ko": "부 전체 비평", "en": "Critique this part", "ja": "部全体を批評"},
		text:  map[string]string{"ko": "지금 부를 처음부터 끝까지 읽고 작품 원장과 부 목표에 비춰 비평해 줘. 회수되지 않은 약속, 긴장이 처지는 구간, 반복되는 패턴을 근거와 함께 알려 줘. 원고는 고치지 마.", "en": "Read this part from start to finish and critique it against the ledger and the part's goal. Point out unpaid promises, sagging stretches and repeated patterns, with evidence. Don't change the manuscript.", "ja": "この部を最初から最後まで読み、作品台帳と部の目標に照らして批評してください。未回収の約束、緊張がたるむ箇所、繰り返しのパターンを根拠とともに示してください。原稿は変えないでください。"},
	},
	{
		name:  "close_arc",
		slots: []string{"longform"},
		tools: []string{"linetta_edit_ledger", "linetta_set_part_arc"},
		title: map[string]string{"ko": "아크 결산", "en": "Close the arc", "ja": "アークの決算"},
		text:  map[string]string{"ko": "지금 아크를 결산해 줘. 회수된 복선을 닫고, 남은 복선은 넘길지 버릴지 나에게 물어 줘. 끝난 사실의 유효 범위를 닫고, 아크 요약과 다음 아크 계획을 갱신한 뒤, 무엇을 바꿨는지 알려 줘. 원고는 고치지 마.", "en": "Close the current arc. Mark the promises it paid, and ask me whether each remaining one carries over or drops. Close the span of facts that have ended, update the arc summary and the next arc's plan, and tell me what changed. Don't change the prose.", "ja": "今のアークを決算してください。回収した伏線を閉じ、残りは持ち越すか捨てるか私に聞いてください。終わった事実の範囲を閉じ、アークの要約と次のアークの計画を更新して、何を変えたか教えてください。原稿は直さないでください。"},
	},
	{
		name:  "rebuild_record",
		slots: []string{"longform"},
		tools: []string{"linetta_edit_ledger"},
		title: map[string]string{"ko": "정본 재구축", "en": "Rebuild the record", "ja": "正本の再構築"},
		text:  map[string]string{"ko": "현재 원고와 고정한 규칙을 기준으로 시놉시스, 부 요약, 회차 계획, 원장을 대조해서 낡은 내용을 고쳐 줘. 옛 원장 항목에는 주어와 속성을 붙이고, 바뀐 사실은 대체로 남겨 줘. 원고는 고치지 말고, 바꾼 기록 목록을 끝에 알려 줘. 큰 변경이면 먼저 작품 복제를 권해 줘.", "en": "Check the synopsis, part summaries, episode plans and ledger against the current manuscript and the pinned rules, and fix what is out of date. Give old ledger entries a subject and attribute, and keep changed facts as replacements. Don't change the prose, and list what you changed at the end. If the change is large, suggest duplicating the work first.", "ja": "現在の原稿と固定した規則を基準に、シノプシス、部の要約、話の計画、台帳を照らし合わせ、古い内容を直してください。古い台帳の項目には主語と属性を付け、変わった事実は置き換えとして残してください。原稿は直さず、変えた記録の一覧を最後に教えてください。大きな変更なら先に作品の複製を勧めてください。"},
	},
	{
		name:  "find_impact",
		slots: []string{"longform"},
		tools: []string{"linetta_find_impact", "linetta_report_continuity"},
		title: map[string]string{"ko": "최근 결정의 영향 찾기", "en": "Find what a decision affects", "ja": "最近の決定の影響を探す"},
		text:  map[string]string{"ko": "방금 바꾼 결정이나 사실이 앞 회차와 어긋나는 곳을 linetta_find_impact로 찾아서 읽고, 어긋나는 문장을 연속성 기록에 남겨 줘. 원고는 고치지 마.", "en": "Use linetta_find_impact to find where the decision or fact just changed disagrees with earlier episodes, read them, and record the lines in continuity checks. Don't change the prose.", "ja": "変えたばかりの決定や事実が前の話と食い違う箇所を linetta_find_impact で探して読み、食い違う文を連続性の記録に残してください。原稿は直さないでください。"},
	},
}

// PromptNames lists every prompt, in the order the panel shows them.
func PromptNames() []string {
	out := make([]string, len(promptDefs))
	for i, p := range promptDefs {
		out[i] = p.name
	}
	return out
}

// promptLanguage falls back to Korean, the app's default language.
func promptLanguage(lang string) string {
	switch lang {
	case "en", "ja":
		return lang
	}
	return "ko"
}

var promptTarget = map[string][2]string{
	"ko": {"대상 작품 ID: %s", "대상 씬 ID: %s"},
	"en": {"Work id: %s", "Scene id: %s"},
	"ja": {"対象の作品ID: %s", "対象のシーンID: %s"},
}

// PromptText is the message a prompt hands back. With no arguments it is the
// sentence alone, word for word what the panel's chip puts in the composer;
// a client that names the work or the scene gets them on lines after it, since
// it has no open scene for "this scene" to mean.
func PromptText(name, lang, projectID, nodeID string) (string, bool) {
	i := slices.IndexFunc(promptDefs, func(p promptDef) bool { return p.name == name })
	if i < 0 {
		return "", false
	}
	lang = promptLanguage(lang)
	lines := []string{promptDefs[i].text[lang]}
	target := promptTarget[lang]
	if id := strings.TrimSpace(projectID); id != "" {
		lines = append(lines, fmt.Sprintf(target[0], id))
	}
	if id := strings.TrimSpace(nodeID); id != "" {
		lines = append(lines, fmt.Sprintf(target[1], id))
	}
	return strings.Join(lines, "\n\n"), true
}

// RegisterPrompts adds the prompts whose tools a server of this mode and tool
// budget serves, worded in lang (the app's UI language).
func RegisterPrompts(s *mcp.Server, lang, mode string, groups ToolGroups) {
	served := ToolNames(mode, groups)
	lang = promptLanguage(lang)
	for _, p := range promptDefs {
		if !servesAll(served, p.tools) {
			continue
		}
		s.AddPrompt(&mcp.Prompt{
			Name:        p.name,
			Title:       p.title[lang],
			Description: p.text[lang],
			Arguments: []*mcp.PromptArgument{
				{Name: "project_id", Description: "The work to act on. Leave out in the Linetta app, where the open work is meant."},
				{Name: "node_id", Description: "The scene to act on. Leave out in the Linetta app, where the open scene is meant."},
			},
			Meta: mcp.Meta{PromptSlotsMeta: slices.Clone(p.slots)},
		}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			args := req.Params.Arguments
			text, _ := PromptText(p.name, lang, args["project_id"], args["node_id"])
			return &mcp.GetPromptResult{
				Description: p.title[lang],
				Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}},
			}, nil
		})
	}
}

func servesAll(served, need []string) bool {
	for _, n := range need {
		if !slices.Contains(served, n) {
			return false
		}
	}
	return true
}

// PanelPrompt is one chip in the agent panel.
type PanelPrompt struct {
	Name  string   `json:"name"`
	Title string   `json:"title"`
	Text  string   `json:"text"`
	Slots []string `json:"slots"`
}

// PanelPrompts is what the agent panel shows: it asks an in-memory server for
// prompts/list and prompts/get, the same two calls an external client makes,
// so a chip's sentence is the slash command's message by construction.
func PanelPrompts(ctx context.Context, lang, mode string, groups ToolGroups) ([]PanelPrompt, error) {
	srv := mcp.NewServer(&mcp.Implementation{Name: ServerName, Title: "Linetta", Version: ServerVersion}, nil)
	RegisterPrompts(srv, lang, mode, groups)
	clientT, serverT := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		return nil, fmt.Errorf("mcphost: connect prompt server: %w", err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "linetta-agent-panel", Version: ServerVersion}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		return nil, fmt.Errorf("mcphost: connect prompt client: %w", err)
	}
	defer cs.Close()

	out := []PanelPrompt{}
	for p, err := range cs.Prompts(ctx, nil) {
		if err != nil {
			return nil, err
		}
		got, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: p.Name})
		if err != nil {
			return nil, err
		}
		text := ""
		if len(got.Messages) > 0 {
			if c, ok := got.Messages[0].Content.(*mcp.TextContent); ok {
				text = c.Text
			}
		}
		out = append(out, PanelPrompt{Name: p.Name, Title: p.Title, Text: text, Slots: metaSlots(p.Meta)})
	}
	// prompts/list orders by name; the panel shows them in definition order.
	order := PromptNames()
	slices.SortStableFunc(out, func(a, b PanelPrompt) int {
		return slices.Index(order, a.Name) - slices.Index(order, b.Name)
	})
	return out, nil
}

func metaSlots(meta mcp.Meta) []string {
	raw, _ := meta[PromptSlotsMeta].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		if s, ok := meta[PromptSlotsMeta].([]string); ok {
			return s
		}
	}
	return out
}
