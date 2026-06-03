package companion

import (
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// PromptData is everything buildContext needs, gathered by the service.
type PromptData struct {
	Outline       string
	Spine         plot.Spine
	HasSpine      bool
	Threads       []thread.Thread
	Entities      []entity.Entity
	Relationships []relationship.Relationship
	Memories      []string
}

// buildSystem returns the companion persona + tool/proposal rules.
func buildSystem() string {
	var b strings.Builder
	b.WriteString("당신은 한국어 소설 작가의 집필 동료입니다. 작가와 자연스럽게 대화하며 플롯·인물·전개를 함께 구상합니다.\n\n")
	b.WriteString("도구가 제공되면 적극적으로 사용하세요: web_search는 최신 자료나 장르 레퍼런스를 찾고, web_fetch는 특정 URL 내용을 확인하며, linetta_apply_ops는 작품의 플롯·스토리라인·비트·캐릭터·관계·장소·요약·기억을 직접 갱신합니다.\n")
	b.WriteString("작가가 아이디어를 승인했거나 작품/소설 개요·시놉시스·아웃라인·스토리라인·비트·캐릭터·관계·장소·요약·기억의 작성/수정/추가/생성/반영/저장을 명확히 요청하면 설명으로 끝내지 말고 반드시 linetta_apply_ops를 호출하세요. 적용 후에는 무엇을 바꿨는지 짧게 말하고, 불확실한 변경은 먼저 질문하세요.\n\n")
	b.WriteString("도구가 없거나 작가가 검토용 제안을 원할 때만, 구체적인 플롯 변경(스토리라인 생성/수정, 비트 추가/수정/삭제, 작품 개요/시놉시스/아웃라인 설정)을 다음 형식의 펜스드 블록 **정확히 하나**로 제안하세요. 단순 대화·질문 응답이면 블록을 넣지 마세요.\n\n")
	b.WriteString("```linetta-proposal\n")
	b.WriteString(`{"summary":"<한 줄 요약>","ops":[ ... ]}` + "\n")
	b.WriteString("```\n\n")
	b.WriteString("op 종류: create_thread{ref?,name,color?,summary?}, update_thread{thread_id,name?,color?,summary?}, add_beat{thread_id|thread_ref,node_id?|node_ref?,label,description?,intensity?}, update_beat{beat_id,label?,description?,intensity?}, delete_beat{beat_id}, set_outline{outline}, remember{text,category?}, create_entity{ref?,kind,name,role?,summary?}, update_entity{entity_id,name?,role?,summary?}, create_relationship{from|from_ref,to|to_ref,label,inverse_label?,notes?}, create_scene{ref?,label,title?,after_node_id?}.\n")
	b.WriteString("id 규칙(중요): 컨텍스트의 '씬'·'스토리라인'·'등장 인물·장소·관계' 목록에 실제로 주어진 id만 사용하세요. id를 절대 지어내지 마세요.\n")
	b.WriteString("- add_beat.node_id는 위 '씬' 목록의 node_id 중 하나입니다. 생략하면 현재 씬에 붙습니다.\n")
	b.WriteString("- 비트는 반드시 스토리라인에 속합니다. 기존 스토리라인이 있으면 그 thread_id를, 새 줄거리면 같은 제안에 create_thread(ref 포함)를 먼저 넣고 add_beat.thread_ref로 그 ref를 참조하세요. thread_id를 추측하지 마세요.\n")
	b.WriteString("- 캐릭터·장소(엔티티)와 관계도 같은 규칙입니다. 기존 엔티티는 '등장 인물·장소·관계' 목록의 id로 참조하고, 새 엔티티는 create_entity(ref 포함) 후 create_relationship에서 from_ref/to_ref로 그 ref를 참조하세요.\n")
	b.WriteString("- create_entity.kind는 반드시 character|place|item|concept 중 하나입니다(생략 시 character로 간주). 캐릭터/인물은 character, 장소는 place.\n")
	b.WriteString("- 새 씬은 create_scene(ref 포함) 후 add_beat.node_ref로 그 씬에 비트를 붙입니다(node_id 생략 시 현재 씬). 관계를 양방향으로 만들려면 create_relationship에 inverse_label을 주세요.\n")
	b.WriteString("예시:\n")
	b.WriteString("```\n")
	b.WriteString(`{"summary":"복수극 라인 추가","ops":[{"op":"create_thread","ref":"t1","name":"복수극"},{"op":"add_beat","thread_ref":"t1","label":"결심","description":"주인공이 복수를 다짐한다"}]}` + "\n")
	b.WriteString("```\n")
	b.WriteString("도구 적용과 제안 블록의 op 스키마는 동일합니다. linetta_apply_ops 입력은 summary와 ops_json입니다. ops_json에는 위 op 배열을 JSON 문자열로 넣으세요. linetta_apply_ops를 사용할 때도 위 id/ref 규칙을 지키고, id를 지어내지 마세요.\n")
	b.WriteString("이전 대화에서 알게 된 작품 설정·작가 취향은 아래 '기억'에 주어집니다. 기억할 가치가 있는 새 사실(작가 취향, 세계관 규칙 등)은 작가 의도가 명확하면 linetta_apply_ops의 remember op로 저장하고, 아니면 remember op로 제안하세요.\n\n")
	b.WriteString("작가에게 여러 후보 중 하나를 고르게 할 때(제목·이름·전개 방향·톤 등)는 본문에 목록으로 나열하지 말고, 아래 형식의 펜스드 블록 **정확히 하나**로 제시하세요. 그러면 작가가 버튼으로 바로 고를 수 있습니다.\n")
	b.WriteString("```linetta-choices\n")
	b.WriteString(`{"prompt":"<무엇을 고르는지 한 줄>","options":["후보1","후보2","후보3"],"allow_custom":true}` + "\n")
	b.WriteString("```\n")
	b.WriteString("- options는 2개 이상이며, 작가가 그 텍스트를 그대로 답장으로 보냅니다. 짧고 명확하게 쓰세요.\n")
	b.WriteString("- allow_custom이 true면 '직접 입력' 버튼이 함께 표시됩니다(작가가 직접 답을 적을 수 있을 때 true).\n")
	b.WriteString("- 블록 앞 본문에는 맥락/이유를 짧게 적되, 후보 목록 자체는 본문에 중복하지 마세요. linetta-choices와 linetta-proposal은 같은 턴에 함께 쓰지 마세요(선택지는 작가에게 되묻는 차례입니다). 단순 대화·설명에는 이 블록을 넣지 마세요.\n")
	return b.String()
}

// buildContext renders the project state as a single user-role message body.
func buildContext(d PromptData) string {
	var b strings.Builder
	if s := strings.TrimSpace(d.Outline); s != "" {
		b.WriteString("## 작품 개요\n")
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	if len(d.Memories) > 0 {
		b.WriteString("## 기억\n")
		for _, m := range d.Memories {
			b.WriteString("- " + m + "\n")
		}
		b.WriteString("\n")
	}
	if d.HasSpine && hasSpineBeats(d.Spine) {
		b.WriteString("## 플롯\n")
		writeScene(&b, "[이전 씬]", d.Spine.Prev)
		writeSceneVal(&b, "[현재 씬]", d.Spine.Current)
		writeScene(&b, "[다음 씬]", d.Spine.Next)
		b.WriteString("\n")
	}
	if d.HasSpine {
		b.WriteString("## 씬 (비트를 붙일 수 있는 실제 씬 — node_id)\n")
		if d.Spine.Prev != nil {
			b.WriteString(fmt.Sprintf("- [%s] %s (이전 씬)\n", d.Spine.Prev.NodeID, d.Spine.Prev.Label))
		}
		b.WriteString(fmt.Sprintf("- [%s] %s (현재 씬)\n", d.Spine.Current.NodeID, d.Spine.Current.Label))
		if d.Spine.Next != nil {
			b.WriteString(fmt.Sprintf("- [%s] %s (다음 씬)\n", d.Spine.Next.NodeID, d.Spine.Next.Label))
		}
		b.WriteString("\n")
	}
	if len(d.Threads) > 0 {
		b.WriteString("## 스토리라인\n")
		for _, t := range d.Threads {
			line := fmt.Sprintf("- [%s] %s", t.ID, t.Name)
			if t.Summary != "" {
				line += " — " + t.Summary
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	if len(d.Entities) > 0 || len(d.Relationships) > 0 {
		b.WriteString("## 등장 인물·장소·관계\n")
		nameByID := map[string]string{}
		for _, e := range d.Entities {
			nameByID[e.ID] = e.Name
			line := fmt.Sprintf("- [%s] (%s) %s", e.ID, kindLabel(e.Kind), e.Name)
			if e.Role != "" {
				line += " / " + e.Role
			}
			if e.Summary != "" {
				line += ": " + e.Summary
			}
			b.WriteString(line + "\n")
		}
		seen := map[string]bool{}
		for _, r := range d.Relationships {
			if r.PairID != nil && *r.PairID != "" {
				if seen[*r.PairID] {
					continue
				}
				seen[*r.PairID] = true
			}
			from, to := nameByID[r.FromID], nameByID[r.ToID]
			if from == "" || to == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s ↔ %s: %s\n", from, to, r.Label))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func hasSpineBeats(s plot.Spine) bool {
	if len(s.Current.Beats) > 0 {
		return true
	}
	if s.Prev != nil && len(s.Prev.Beats) > 0 {
		return true
	}
	if s.Next != nil && len(s.Next.Beats) > 0 {
		return true
	}
	return false
}

func writeScene(b *strings.Builder, tag string, s *plot.SceneBeats) {
	if s == nil || len(s.Beats) == 0 {
		return
	}
	writeSceneVal(b, tag, *s)
}

func writeSceneVal(b *strings.Builder, tag string, s plot.SceneBeats) {
	if len(s.Beats) == 0 {
		return
	}
	b.WriteString(tag + "\n")
	for _, bt := range s.Beats {
		line := fmt.Sprintf("  · [%s] %s", bt.ThreadName, bt.Label)
		if strings.TrimSpace(bt.Description) != "" {
			line += " — " + bt.Description
		}
		b.WriteString(line + "\n")
	}
}

func kindLabel(k string) string {
	switch k {
	case "character":
		return "인물"
	case "place":
		return "장소"
	case "item":
		return "물건"
	case "concept":
		return "개념"
	}
	return k
}
