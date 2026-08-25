package export

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/project"
)

// CompanionHistorySource lists a project's companion transcript. Narrow on
// purpose: this package needs to read the archive, not manage it.
type CompanionHistorySource interface {
	List(ctx context.Context, q companion.HistoryQuery) ([]companion.HistoryMessage, error)
}

// CompanionMemorySource surfaces what the companion was told to remember, plus
// where the raw log lives so the document can point at it.
type CompanionMemorySource interface {
	Recall(projectID, query string, limit int) []string
	MemoryRoot(projectID string) string
}

// CompanionExportMemoryLimit is what one project's fact list can carry. The
// underlying store caps a search at 100 rows, so a longer list is truncated
// rather than silently complete — the document says so and names the file.
const CompanionExportMemoryLimit = 100

// companionHistoryLimit caps one project's transcript. Generous enough that no
// realistic companion history hits it; present so a runaway table cannot build
// a document too large to open.
const companionHistoryLimit = 20000

// ExportCompanion writes every project's companion transcript and remembered
// facts into one markdown document.
//
// This exists because the pivot retires the companion, and a conversation the
// writer had about their own book is not the kind of thing a version bump gets
// to delete. It is deliberately an archive rather than a view: one file, every
// project, readable without Linetta installed.
func ExportCompanion(
	ctx context.Context,
	pr *project.Repo,
	history CompanionHistorySource,
	mem CompanionMemorySource,
	now time.Time,
) (Payload, error) {
	projects, err := pr.List(ctx, project.ListFilter{IncludeArchived: true})
	if err != nil {
		return Payload{}, err
	}

	var sb strings.Builder
	sb.WriteString("# 리네타 컴패니언 기록\n\n")
	sb.WriteString(fmt.Sprintf("> 내보낸 시각: %s\n", now.Format("2006-01-02 15:04")))
	sb.WriteString("> 리네타는 순수 창작 도구로 전환했고, 내장 AI 컴패니언은 다음 주요 버전에서 제거됩니다.\n")
	sb.WriteString("> 이 파일은 그 전에 대화 기록을 남겨 두기 위한 것입니다.\n\n")

	wrote := false
	for _, p := range projects {
		msgs, err := history.List(ctx, companion.HistoryQuery{
			ProjectID: p.ID,
			Scope:     companion.HistoryViewProject,
			Limit:     companionHistoryLimit,
		})
		if err != nil {
			return Payload{}, err
		}
		var facts []string
		var memRoot string
		if mem != nil {
			facts = mem.Recall(p.ID, "", CompanionExportMemoryLimit)
			memRoot = mem.MemoryRoot(p.ID)
		}
		if len(msgs) == 0 && len(facts) == 0 {
			continue
		}
		wrote = true

		sb.WriteString("## " + titleOrUntitled(p.Title) + "\n\n")
		writeCompanionMessages(&sb, msgs)
		writeCompanionFacts(&sb, facts, memRoot)
	}

	if !wrote {
		sb.WriteString("남아 있는 컴패니언 대화나 기억이 없습니다.\n")
	}

	return Payload{
		Markdown:          sb.String(),
		SuggestedFilename: fmt.Sprintf("linetta-companion-%s.md", now.Format("20060102")),
	}, nil
}

func titleOrUntitled(title string) string {
	if strings.TrimSpace(title) == "" {
		return "제목 없음"
	}
	return title
}

func writeCompanionMessages(sb *strings.Builder, msgs []companion.HistoryMessage) {
	if len(msgs) == 0 {
		return
	}
	sb.WriteString("### 대화\n\n")
	for _, m := range msgs {
		stamp := time.UnixMilli(m.CreatedAt).Format("2006-01-02 15:04")
		who := companionSpeaker(m.Role)
		sb.WriteString(fmt.Sprintf("**%s · %s**", stamp, who))
		if m.NodeLabel != "" {
			sb.WriteString(fmt.Sprintf(" — %s", m.NodeLabel))
		}
		// A failed or cancelled turn is part of the record; saying so beats
		// presenting a stub as if the companion had answered.
		if m.Status != "" && m.Status != companion.HistoryStatusDone && m.Status != companion.HistoryStatusApplied {
			sb.WriteString(fmt.Sprintf(" (%s)", m.Status))
		}
		sb.WriteString("\n\n")
		body := strings.TrimSpace(m.Content)
		if body == "" {
			body = "(내용 없음)"
		}
		sb.WriteString(body)
		sb.WriteString("\n\n")
	}
}

func writeCompanionFacts(sb *strings.Builder, facts []string, memRoot string) {
	if len(facts) == 0 {
		return
	}
	sb.WriteString("### 기억한 것\n\n")
	for _, f := range facts {
		sb.WriteString("- " + f + "\n")
	}
	sb.WriteString("\n")
	if len(facts) >= CompanionExportMemoryLimit && memRoot != "" {
		sb.WriteString(fmt.Sprintf(
			"> 기억은 최대 %d개까지만 담깁니다. 전체 기록은 `%s` 아래 `memory/experiences.jsonl`에 그대로 남아 있으며, 이 파일은 컴패니언이 제거돼도 지워지지 않습니다.\n\n",
			CompanionExportMemoryLimit, memRoot))
	}
}

func companionSpeaker(role string) string {
	switch role {
	case "user":
		return "나"
	case "assistant":
		return "컴패니언"
	default:
		return role
	}
}
