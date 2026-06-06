package companion

import (
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// PromptData is everything buildContext needs, gathered by the service.
type PromptData struct {
	Outline          string
	OutlineStructure string
	OutlineNodes     []OutlineNode
	SceneExcerpts    []SceneExcerpt
	Spine            plot.Spine
	HasSpine         bool
	Threads          []thread.Thread
	Entities         []entity.Entity
	Relationships    []relationship.Relationship
	Facts            []fact.Card
	Memories         []string
}

// OutlineNode is a compact view of the left outline tree with node ids.
type OutlineNode struct {
	ID       string
	ParentID string
	Kind     string
	Label    string
	Title    string
	Depth    int
}

// SceneExcerpt is a bounded plaintext excerpt from an already-written leaf
// scene. The companion uses it as direct evidence for draft analysis.
type SceneExcerpt struct {
	NodeID    string
	Label     string
	Text      string
	IsCurrent bool
}

// buildSystem returns the companion persona + tool/proposal rules.
func buildSystem() string {
	var b strings.Builder
	b.WriteString("당신은 한국어 소설 작가의 집필 동료입니다. 작가와 자연스럽게 대화하며 플롯·인물·전개를 함께 구상합니다.\n\n")
	b.WriteString("도구가 제공되면 적극적으로 사용하세요: web_search는 최신 자료나 장르 레퍼런스를 찾고, web_fetch는 특정 URL 내용을 확인하며, linetta_apply_ops는 작품의 개요·시놉시스·아웃라인 트리·스토리라인·비트·캐릭터·관계·장소·씬·기억·팩트 자료집을 직접 갱신합니다.\n")
	b.WriteString("컨텍스트의 '작성된 본문 발췌'는 이미 작성된 실제 원고입니다. 캐릭터·관계·전개 분석 요청에서는 이 본문을 우선 근거로 삼고, 본문이 제공되어 있는데 없다고 말하지 마세요.\n")
	b.WriteString("용어 구분: '아웃라인/목차/부/장/씬 구성'은 왼쪽 아웃라인 트리이며 create_outline_node/create_scene/rename_outline_node/delete_outline_node/move_outline_node로 갱신합니다. '작품 개요/시놉시스' 텍스트는 set_outline으로 갱신합니다. '플롯/스토리라인/비트'는 create_thread/add_beat로 갱신합니다.\n")
	b.WriteString("작가가 아이디어를 승인했거나 작품/소설 개요·시놉시스·아웃라인·얼개·스토리라인·비트·캐릭터·관계·장소·씬·기억의 작성/수정/추가/생성/구체화/세분화/분할/확장/반영/저장을 명확히 요청하면 설명으로 끝내지 말고 반드시 linetta_apply_ops를 호출하세요. 적용 후에는 무엇을 바꿨는지 짧게 말하고, 불확실한 변경은 먼저 질문하세요.\n")
	b.WriteString("조각하듯 집필을 돕습니다: 거친 시놉시스나 한 문장을 받으면 set_outline으로 작품 개요를 정리하고 create_outline_node/create_scene으로 보이는 아웃라인 트리를 만들며, 필요하면 그 씬들에 create_thread/add_beat로 플롯 비트를 함께 붙이세요. 특정 파트·챕터·막을 요청받으면 아웃라인 노드를 세분화하고 비트를 연결하며, 특정 씬을 구체화해 달라는 요청에는 현재 씬 또는 새 씬에 비트를 붙여 실제 상태를 갱신하세요.\n\n")
	b.WriteString("도구가 없거나 작가가 검토용 제안을 원할 때만, 구체적인 변경(아웃라인 트리 생성/수정, 스토리라인 생성/수정, 비트 추가/수정/삭제, 작품 개요/시놉시스 설정)을 다음 형식의 펜스드 블록 **정확히 하나**로 제안하세요. 단순 대화·질문 응답이면 블록을 넣지 마세요.\n\n")
	b.WriteString("```linetta-proposal\n")
	b.WriteString(`{"summary":"<한 줄 요약>","ops":[ ... ]}` + "\n")
	b.WriteString("```\n\n")
	b.WriteString("op 종류: create_outline_node{ref?,kind:container|leaf,label,title?,parent_node_id?|parent_node_ref?,after_node_id?|after_node_ref?}, rename_outline_node{node_id|node_ref,label?,title?}, delete_outline_node{node_id|node_ref}, move_outline_node{node_id|node_ref,direction:up|down}, create_scene{ref?,label,title?,after_node_id?|after_node_ref?}, create_thread{ref?,name,color?,summary?}, update_thread{thread_id,name?,color?,summary?}, add_beat{thread_id|thread_ref,node_id?|node_ref?,label,description?,intensity?}, update_beat{beat_id,label?,description?,intensity?}, delete_beat{beat_id}, set_outline{outline}, remember{text,category?}, create_entity{ref?,kind,name,role?,summary?}, update_entity{entity_id,name?,role?,summary?}, create_relationship{from|from_ref,to|to_ref,label,inverse_label?,notes?}, create_fact_card{ref?,claim,result,status,category?,node_id?|node_ref?,sources:[{url,title?,snippet?,accessed_at?}]}.\n")
	b.WriteString("- 팩트 자료집 저장 규칙: create_fact_card는 최소 1개 출처 URL이 있을 때만 사용하세요. sources[].url 없는 자료는 저장하지 말고, 최신성·불확실성은 status(verified|uncertain|intentional_fiction|stale)로 표시하세요. 출처 본문은 긴 인용 대신 짧은 요약/snippet만 넣으세요.\n")
	b.WriteString("id 규칙(중요): 컨텍스트의 '씬'·'스토리라인'·'등장 인물·장소·관계' 목록에 실제로 주어진 id만 사용하세요. id를 절대 지어내지 마세요.\n")
	b.WriteString("- add_beat.node_id는 위 '씬' 목록의 node_id 중 하나입니다. 생략하면 현재 씬에 붙습니다.\n")
	b.WriteString("- 비트는 반드시 스토리라인에 속합니다. 기존 스토리라인이 있으면 그 thread_id를, 새 줄거리면 같은 제안에 create_thread(ref 포함)를 먼저 넣고 add_beat.thread_ref로 그 ref를 참조하세요. thread_id를 추측하지 마세요.\n")
	b.WriteString("- 캐릭터·장소(엔티티)와 관계도 같은 규칙입니다. 기존 엔티티는 '등장 인물·장소·관계' 목록의 id로 참조하고, 새 엔티티는 create_entity(ref 포함) 후 create_relationship에서 from_ref/to_ref로 그 ref를 참조하세요.\n")
	b.WriteString("- create_entity.kind는 반드시 character|place|item|concept 중 하나입니다(생략 시 character로 간주). 캐릭터/인물은 character, 장소는 place.\n")
	b.WriteString("- 아웃라인 정리 규칙: 컨텍스트의 '아웃라인 트리'에 기존 node_id가 있으면 같은 부/장/씬을 다시 만들지 말고 rename_outline_node/delete_outline_node/move_outline_node로 정리하세요. 새 구조를 추가할 때만 create_outline_node(ref 포함)를 쓰세요.\n")
	b.WriteString("- 새 아웃라인 구조는 컨텍스트의 '아웃라인 구조 프리셋'이 있으면 그 계층과 라벨을 따르고, parent_node_ref로 상위→하위 계층을 만든 뒤 add_beat.node_ref로 생성한 씬에 비트를 붙입니다. parent/after가 없는 create_outline_node는 루트에 만들어집니다.\n")
	b.WriteString("- 새 씬 하나만 현재 위치 옆에 만들 때는 create_scene(ref 포함) 후 add_beat.node_ref로 그 씬에 비트를 붙입니다(node_id 생략 시 현재 씬). 관계를 양방향으로 만들려면 create_relationship에 inverse_label을 주세요.\n")
	b.WriteString("예시:\n")
	b.WriteString("```\n")
	b.WriteString(`{"summary":"복수극 라인 추가","ops":[{"op":"create_thread","ref":"t1","name":"복수극"},{"op":"add_beat","thread_ref":"t1","label":"결심","description":"주인공이 복수를 다짐한다"}]}` + "\n")
	b.WriteString(`{"summary":"1부 아웃라인과 플롯 비트 작성","ops":[{"op":"create_outline_node","ref":"p1","kind":"container","label":"1부","title":"추적의 시작"},{"op":"create_outline_node","ref":"c1","kind":"container","parent_node_ref":"p1","label":"1장"},{"op":"create_outline_node","ref":"s1","kind":"leaf","parent_node_ref":"c1","label":"씬 1","title":"사라진 메시지"},{"op":"create_thread","ref":"t1","name":"1부 추적선"},{"op":"add_beat","thread_ref":"t1","node_ref":"s1","label":"단서 발견","description":"주인공이 지워진 문자 기록에서 첫 단서를 얻는다"}]}` + "\n")
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
	if s := strings.TrimSpace(d.OutlineStructure); s != "" {
		b.WriteString("## 아웃라인 구조 프리셋\n")
		b.WriteString(s)
		b.WriteString("\n")
		b.WriteString("아웃라인 트리를 새로 만들거나 정리할 때 이 계층과 라벨 예시를 따르세요.\n\n")
	}
	if len(d.OutlineNodes) > 0 {
		b.WriteString("## 아웃라인 트리 (기존 구조 — node_id)\n")
		for _, n := range d.OutlineNodes {
			indent := strings.Repeat("  ", n.Depth)
			line := fmt.Sprintf("%s- [%s] (%s) %s", indent, n.ID, n.Kind, n.Label)
			if strings.TrimSpace(n.Title) != "" {
				line += " — " + strings.TrimSpace(n.Title)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	if len(d.Memories) > 0 {
		b.WriteString("## 기억\n")
		for _, m := range d.Memories {
			b.WriteString("- " + m + "\n")
		}
		b.WriteString("\n")
	}
	if len(d.Facts) > 0 {
		b.WriteString("## 팩트 자료집\n")
		for _, f := range d.Facts {
			line := fmt.Sprintf("- [%s] (%s) %s", f.ID, f.Status, f.Claim)
			if strings.TrimSpace(f.Category) != "" {
				line += " / " + f.Category
			}
			if strings.TrimSpace(f.Result) != "" {
				line += ": " + f.Result
			}
			b.WriteString(line + "\n")
			for _, src := range f.Sources {
				if strings.TrimSpace(src.URL) == "" {
					continue
				}
				title := strings.TrimSpace(src.Title)
				if title == "" {
					title = src.URL
				}
				b.WriteString(fmt.Sprintf("  · %s — %s\n", title, src.URL))
			}
		}
		b.WriteString("\n")
	}
	if len(d.SceneExcerpts) > 0 {
		b.WriteString("## 작성된 본문 발췌\n")
		for _, s := range d.SceneExcerpts {
			if strings.TrimSpace(s.Text) == "" {
				continue
			}
			current := ""
			if s.IsCurrent {
				current = " (현재 씬)"
			}
			b.WriteString(fmt.Sprintf("### [%s] %s%s\n", s.NodeID, s.Label, current))
			b.WriteString(strings.TrimSpace(s.Text))
			b.WriteString("\n\n")
		}
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

func applyContextSelection(d PromptData, selection ai.ContextSelection) PromptData {
	if !selection.Enabled(ai.ContextKeyCurrentScene) {
		d.SceneExcerpts = nil
	}
	if !selection.Enabled(ai.ContextKeyOverview) {
		d.Outline = ""
	}
	if !selection.Enabled(ai.ContextKeyPlot) {
		d.HasSpine = false
		d.Spine = plot.Spine{}
		d.Threads = nil
	}
	if !selection.Enabled(ai.ContextKeyEntities) {
		d.Entities = nil
	}
	if !selection.Enabled(ai.ContextKeyRelationships) {
		d.Relationships = nil
	}
	if !selection.Enabled(ai.ContextKeyFacts) {
		d.Facts = nil
	}
	if !selection.Enabled(ai.ContextKeyMemories) {
		d.Memories = nil
	}
	return d
}

func previewFromPromptData(d PromptData, selection ai.ContextSelection) ai.ContextPreview {
	sections := []ai.PreviewSection{}
	add := func(id ai.ContextKey, label string, count int, preview string) {
		present := count > 0 || strings.TrimSpace(preview) != ""
		sections = append(sections, ai.PreviewSection{
			ID:       id,
			Label:    label,
			Present:  present,
			Selected: present && selection.Enabled(id),
			Count:    count,
			Preview:  trimPreview(strings.TrimSpace(preview)),
		})
	}

	add(ai.ContextKeyCurrentScene, "작성된 본문 발췌", len(d.SceneExcerpts), renderSceneExcerptsPreview(d.SceneExcerpts))

	overview := strings.TrimSpace(d.Outline)
	add(ai.ContextKeyOverview, "작품 개요", boolCount(overview != ""), overview)

	add(ai.ContextKeyFacts, "팩트 자료집", len(d.Facts), renderFactsPreview(d.Facts))
	add(ai.ContextKeyPlot, "플롯 (스토리라인&비트)", companionPlotCount(d), renderCompanionPlotPreview(d))
	add(ai.ContextKeyEntities, "등장 인물·장소", len(d.Entities), renderCompanionEntitiesPreview(d.Entities))
	add(ai.ContextKeyRelationships, "관계", len(d.Relationships), renderCompanionRelationshipsPreview(d.Entities, d.Relationships))
	add(ai.ContextKeyMemories, "컴패니언 기억", len(d.Memories), renderMemoriesPreview(d.Memories))

	selectedCount := 0
	for _, section := range sections {
		if !section.Selected {
			continue
		}
		if section.Count > 0 {
			selectedCount += section.Count
		} else {
			selectedCount++
		}
	}

	return ai.ContextPreview{
		PreviewCounts: ai.PreviewCounts{
			NearbyScenes:  len(d.SceneExcerpts),
			HasOutline:    overview != "",
			Entities:      len(d.Entities),
			Relationships: len(d.Relationships),
			PlotBeats:     companionPlotCount(d),
			Notes:         len(d.Memories),
		},
		Sections:          sections,
		SelectedItemCount: selectedCount,
	}
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

func renderSceneExcerptsPreview(excerpts []SceneExcerpt) string {
	var b strings.Builder
	for _, s := range excerpts {
		if strings.TrimSpace(s.Text) == "" {
			continue
		}
		current := ""
		if s.IsCurrent {
			current = " (현재 씬)"
		}
		b.WriteString(fmt.Sprintf("### [%s] %s%s\n", s.NodeID, s.Label, current))
		b.WriteString(strings.TrimSpace(s.Text))
		b.WriteString("\n\n")
	}
	return b.String()
}

func renderFactsPreview(facts []fact.Card) string {
	var b strings.Builder
	for _, f := range facts {
		line := fmt.Sprintf("- [%s] (%s) %s", f.ID, f.Status, f.Claim)
		if strings.TrimSpace(f.Category) != "" {
			line += " / " + f.Category
		}
		if strings.TrimSpace(f.Result) != "" {
			line += ": " + f.Result
		}
		b.WriteString(line + "\n")
		for _, src := range f.Sources {
			if strings.TrimSpace(src.URL) == "" {
				continue
			}
			title := strings.TrimSpace(src.Title)
			if title == "" {
				title = src.URL
			}
			b.WriteString(fmt.Sprintf("  · %s — %s\n", title, src.URL))
		}
	}
	return b.String()
}

func companionPlotCount(d PromptData) int {
	return len(d.Threads) + countCompanionSpineBeats(d.Spine)
}

func countCompanionSpineBeats(spine plot.Spine) int {
	n := len(spine.Current.Beats)
	if spine.Prev != nil {
		n += len(spine.Prev.Beats)
	}
	if spine.Next != nil {
		n += len(spine.Next.Beats)
	}
	return n
}

func renderCompanionPlotPreview(d PromptData) string {
	var b strings.Builder
	if d.HasSpine && hasSpineBeats(d.Spine) {
		writeScene(&b, "[이전 씬]", d.Spine.Prev)
		writeSceneVal(&b, "[현재 씬]", d.Spine.Current)
		writeScene(&b, "[다음 씬]", d.Spine.Next)
	}
	if d.HasSpine {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("씬 (node_id)\n")
		if d.Spine.Prev != nil {
			b.WriteString(fmt.Sprintf("- [%s] %s (이전 씬)\n", d.Spine.Prev.NodeID, d.Spine.Prev.Label))
		}
		b.WriteString(fmt.Sprintf("- [%s] %s (현재 씬)\n", d.Spine.Current.NodeID, d.Spine.Current.Label))
		if d.Spine.Next != nil {
			b.WriteString(fmt.Sprintf("- [%s] %s (다음 씬)\n", d.Spine.Next.NodeID, d.Spine.Next.Label))
		}
	}
	if len(d.Threads) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("스토리라인\n")
		for _, t := range d.Threads {
			line := fmt.Sprintf("- [%s] %s", t.ID, t.Name)
			if t.Summary != "" {
				line += " — " + t.Summary
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

func renderCompanionEntitiesPreview(entities []entity.Entity) string {
	var b strings.Builder
	for _, e := range entities {
		line := fmt.Sprintf("- [%s] (%s) %s", e.ID, kindLabel(e.Kind), e.Name)
		if e.Role != "" {
			line += " / " + e.Role
		}
		if e.Summary != "" {
			line += ": " + e.Summary
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func renderCompanionRelationshipsPreview(entities []entity.Entity, relationships []relationship.Relationship) string {
	nameByID := map[string]string{}
	for _, e := range entities {
		nameByID[e.ID] = e.Name
	}
	var b strings.Builder
	seen := map[string]bool{}
	for _, r := range relationships {
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
	return b.String()
}

func renderMemoriesPreview(memories []string) string {
	var b strings.Builder
	for _, m := range memories {
		b.WriteString("- " + m + "\n")
	}
	return b.String()
}

func trimPreview(s string) string {
	r := []rune(s)
	if len(r) <= 1200 {
		return s
	}
	return string(r[:1200]) + "…"
}

func boolCount(ok bool) int {
	if ok {
		return 1
	}
	return 0
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
