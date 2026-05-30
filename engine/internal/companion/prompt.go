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

// buildSystem returns the companion persona + proposal-format rules.
func buildSystem() string {
	var b strings.Builder
	b.WriteString("당신은 한국어 소설 작가의 집필 동료입니다. 작가와 자연스럽게 대화하며 플롯·인물·전개를 함께 구상합니다.\n\n")
	b.WriteString("구체적인 플롯 변경(스토리라인 생성/수정, 비트 추가/수정/삭제, 작품 개요 설정)을 제안할 때만, 응답에 다음 형식의 펜스드 블록을 **정확히 하나** 포함하세요. 단순 대화·질문 응답이면 블록을 넣지 마세요.\n\n")
	b.WriteString("```linetta-proposal\n")
	b.WriteString(`{"summary":"<한 줄 요약>","ops":[ ... ]}` + "\n")
	b.WriteString("```\n\n")
	b.WriteString("op 종류: create_thread{ref?,name,color?,summary?}, update_thread{thread_id,name?,color?,summary?}, add_beat{thread_id|thread_ref,node_id,label,description?,intensity?}, update_beat{beat_id,label?,description?,intensity?}, delete_beat{beat_id}, set_outline{outline}, remember{text,category?}.\n")
	b.WriteString("기존 스토리라인·씬은 아래 컨텍스트에 주어진 id로 참조하세요. 같은 제안에서 새로 만든 스토리라인은 create_thread.ref 핸들을 add_beat.thread_ref로 참조하세요.\n")
	b.WriteString("당신은 변경을 직접 적용하지 않습니다 — 작가가 제안을 검토 후 적용합니다.\n")
	b.WriteString("이전 대화에서 알게 된 작품 설정·작가 취향은 아래 '기억'에 주어집니다. 기억할 가치가 있는 새 사실(작가 취향, 세계관 규칙 등)은 remember op로 제안하세요(작가가 승인하면 저장됩니다).\n")
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
			line := fmt.Sprintf("- [%s] %s", e.ID, e.Name)
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
