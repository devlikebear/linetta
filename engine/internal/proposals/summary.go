package proposals

import (
	"context"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/relationship"
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
	case "change_relationship":
		rel, err := relationship.NewRepo(s.store).Get(ctx, op.RelationshipID)
		if err != nil {
			return fmt.Sprintf("관계 변화: %s", op.Label)
		}
		was := rel.Label
		scene := ""
		if s.nodes != nil {
			if n, err := s.nodes.Get(ctx, op.NodeID); err == nil {
				scene = n.Title
				if label := node.DisplayLabel(n.Label, "ko"); label != "" {
					scene = label
				}
			}
		}
		if scene != "" {
			return fmt.Sprintf("관계 변화: %s → %s (%s → %s, %s부터)", resolveEntity(rel.FromID), resolveEntity(rel.ToID), was, op.Label, scene)
		}
		return fmt.Sprintf("관계 변화: %s → %s (%s → %s)", resolveEntity(rel.FromID), resolveEntity(rel.ToID), was, op.Label)
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

// LedgerOpType is what the inbox and the agent's inbox listing call a ledger
// item, which has no storyops op.
const LedgerOpType = "ledger_entry"

// ItemOpType is it's op type for display: its op's, or LedgerOpType.
func ItemOpType(it Item) string {
	if it.Ledger != nil {
		return LedgerOpType
	}
	return it.Op.Type
}

// SummarizeItem is Summarize for any item, a ledger item included.
func (s *Service) SummarizeItem(ctx context.Context, it Item, siblings []Item) string {
	if it.Ledger == nil {
		return s.Summarize(ctx, it.Op, siblings)
	}
	l := it.Ledger
	if l.EntryID == "" {
		return fmt.Sprintf("%s 추가: %s", ledgerKindLabelKo(l.Kind), ledgerLine(l.Subject, l.Body))
	}
	name := l.EntryID
	if s.ledger != nil {
		if e, err := s.ledger.Get(ctx, l.EntryID); err == nil {
			name = ledgerLine(e.Subject, e.Body)
		}
	}
	switch l.Status {
	case ledger.StatusPaid:
		return fmt.Sprintf("회수로 표시: %s", name)
	case ledger.StatusDropped:
		return fmt.Sprintf("버림으로 표시: %s", name)
	}
	return fmt.Sprintf("원장 수정: %s", name)
}

func ledgerLine(subject, body string) string {
	if subject == "" {
		return body
	}
	return subject + " — " + body
}

func ledgerKindLabelKo(kind string) string {
	switch kind {
	case ledger.KindPromise:
		return "복선"
	case ledger.KindGag:
		return "반복 개그"
	case ledger.KindQuestion:
		return "독자의 질문"
	case ledger.KindDecision:
		return "결정"
	}
	return "원장 항목"
}
