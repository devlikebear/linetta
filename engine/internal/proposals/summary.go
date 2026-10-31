package proposals

import (
	"context"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// DependencyRejected reports whether anything it depends on was rejected.
// siblings must be the item's whole proposal, not a status-filtered part.
func DependencyRejected(it Item, siblings []Item) bool {
	for _, dep := range it.DependsOn {
		for _, sib := range siblings {
			if sib.ID == dep && sib.Status == StatusRejected {
				return true
			}
		}
	}
	return false
}

// Summarize renders one op the way the inbox row's headline
// reads it ("새 인물: 하린 — 검술 사범"), resolving a referenced entity/
// thread's name either from the work (an update op's real id) or from a
// sibling create_entity/create_thread item in the SAME proposal (a ref that
// has not been accepted yet, so it has no real id to look up).
func (s *Service) Summarize(ctx context.Context, op storyops.Op, siblings []Item) string {
	resolveEntity := func(idOrRef string) string {
		if idOrRef == "" {
			return ""
		}
		if s.entities != nil {
			if e, err := s.entities.Get(ctx, idOrRef); err == nil {
				return e.Name
			}
		}
		for _, sib := range siblings {
			if sib.Op.Type == "create_entity" && sib.Op.Ref == idOrRef {
				return sib.Op.Name
			}
		}
		return idOrRef
	}
	resolveThread := func(idOrRef string) string {
		if idOrRef == "" {
			return ""
		}
		if s.threads != nil {
			if th, err := s.threads.Get(ctx, idOrRef); err == nil {
				return th.Name
			}
		}
		for _, sib := range siblings {
			if sib.Op.Type == "create_thread" && sib.Op.Ref == idOrRef {
				return sib.Op.Name
			}
		}
		return idOrRef
	}

	switch op.Type {
	case "create_entity":
		if op.Role != "" {
			return fmt.Sprintf("새 %s: %s — %s", entityKindLabelKo(op.Kind), op.Name, op.Role)
		}
		return fmt.Sprintf("새 %s: %s", entityKindLabelKo(op.Kind), op.Name)
	case "update_entity":
		name := op.Name
		if name == "" {
			name = resolveEntity(op.EntityID)
		}
		return fmt.Sprintf("%s 정보 수정", name)
	case "create_relationship":
		from := op.From
		if from == "" {
			from = op.FromRef
		}
		to := op.To
		if to == "" {
			to = op.ToRef
		}
		return fmt.Sprintf("관계: %s → %s (%s)", resolveEntity(from), resolveEntity(to), op.Label)
	case "create_fact_card":
		return fmt.Sprintf("팩트: %s", op.Claim)
	case "create_thread":
		return fmt.Sprintf("새 스토리라인: %s", op.Name)
	case "update_thread":
		name := op.Name
		if name == "" {
			name = resolveThread(op.ThreadID)
		}
		return fmt.Sprintf("스토리라인 수정: %s", name)
	case "add_beat":
		return fmt.Sprintf("비트 추가: %s", op.Label)
	case "update_beat":
		return fmt.Sprintf("비트 수정: %s", op.Label)
	default:
		return op.Type
	}
}

func entityKindLabelKo(kind string) string {
	switch kind {
	case "place":
		return "장소"
	case "item":
		return "사물"
	case "concept":
		return "개념"
	default:
		return "인물"
	}
}
