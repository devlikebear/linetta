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
	language string,
) (Payload, error) {
	projects, err := pr.List(ctx, project.ListFilter{IncludeArchived: true})
	if err != nil {
		return Payload{}, err
	}

	w := companionStringsFor(language)
	var sb strings.Builder
	sb.WriteString("# " + w.title + "\n\n")
	sb.WriteString(fmt.Sprintf("> %s: %s\n", w.exportedAt, now.Format("2006-01-02 15:04")))
	sb.WriteString("> " + w.noticeLine1 + "\n")
	sb.WriteString("> " + w.noticeLine2 + "\n\n")

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

		sb.WriteString("## " + titleOrUntitled(p.Title, w) + "\n\n")
		writeCompanionMessages(&sb, msgs, w)
		writeCompanionFacts(&sb, facts, memRoot, w)
	}

	if !wrote {
		sb.WriteString(w.nothingLeft + "\n")
	}

	return Payload{
		Markdown:          sb.String(),
		SuggestedFilename: fmt.Sprintf("linetta-companion-%s.md", now.Format("20060102")),
	}, nil
}

func titleOrUntitled(title string, w companionStrings) string {
	if strings.TrimSpace(title) == "" {
		return w.untitled
	}
	return title
}

func writeCompanionMessages(sb *strings.Builder, msgs []companion.HistoryMessage, w companionStrings) {
	if len(msgs) == 0 {
		return
	}
	sb.WriteString("### " + w.conversation + "\n\n")
	for _, m := range msgs {
		stamp := time.UnixMilli(m.CreatedAt).Format("2006-01-02 15:04")
		who := companionSpeaker(m.Role, w)
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
			body = w.emptyMessage
		}
		sb.WriteString(body)
		sb.WriteString("\n\n")
	}
}

func writeCompanionFacts(sb *strings.Builder, facts []string, memRoot string, w companionStrings) {
	if len(facts) == 0 {
		return
	}
	sb.WriteString("### " + w.remembered + "\n\n")
	for _, f := range facts {
		sb.WriteString("- " + f + "\n")
	}
	sb.WriteString("\n")
	if len(facts) >= CompanionExportMemoryLimit && memRoot != "" {
		sb.WriteString(fmt.Sprintf(w.memoryTruncate, CompanionExportMemoryLimit, memRoot))
	}
}

func companionSpeaker(role string, w companionStrings) string {
	switch role {
	case "user":
		return w.speakerUser
	case "assistant":
		return w.speakerAgent
	default:
		return role
	}
}
