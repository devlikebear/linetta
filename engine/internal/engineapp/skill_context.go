package engineapp

import (
	"context"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

var _ storycontext.SkillSource = skillBriefSource{}

// skillBriefSource adapts *agentskills.Store to storycontext.SkillSource, so
// the story brief carries the same list the built-in agent gets in its system
// prompt. It is the brief's half of what agentSkillSource does for the prompt,
// and it exists as a second adapter rather than one shared type only because
// the two consumers want different shapes: agent.SkillSource hands over
// agentskills.Skill values, storycontext deliberately keeps no dependency on
// agentskills at all (see storycontext.SkillBrief).
//
// It lives here, untagged, rather than beside agentSkillSource in
// agent_enabled.go: the built-in agent is a !mobile feature, but the MCP
// story brief is not, and a mobile build that served briefs with no skills in
// them would be the silent omission this whole task is about.
//
// The reduction — enabled only, guard-passed, bodies left behind — is
// appendEnabledSkills' below, the same function the prompt's adapter uses, so
// there is one place that decides what may reach a model and not two that
// have to agree.
type skillBriefSource struct {
	store *agentskills.Store
}

// ContextSkills lists the writer's skills and, when a work is open, that
// work's. A nil store yields none rather than panicking, matching
// agentSkillSource — though engineapp.New never builds one that way; see
// TestProductionStoryContextBuilderCarriesEverySource.
func (s skillBriefSource) ContextSkills(_ context.Context, projectID string) []storycontext.SkillBrief {
	if s.store == nil {
		return nil
	}
	var skills []agentskills.Skill
	skills = appendEnabledSkills(skills, s.store, agentskills.ScopeWriter, "")
	// Work-scoped skills need a work to belong to; a brief built with no
	// project id simply has none to offer.
	if id := strings.TrimSpace(projectID); id != "" {
		skills = appendEnabledSkills(skills, s.store, agentskills.ScopeWork, id)
	}
	out := make([]storycontext.SkillBrief, 0, len(skills))
	for _, sk := range skills {
		out = append(out, storycontext.SkillBrief{
			Name:        sk.Name,
			Description: sk.Description,
			Scope:       string(sk.Scope),
		})
	}
	return out
}

// appendEnabledSkills lists one scope and appends its enabled, body-stripped
// skills to out. Shared by skillBriefSource above and agentSkillSource in
// agent_enabled.go, which is where its full rationale lives — in short: a
// list error is swallowed (best-effort, like the curated memory reads), and
// Store.List has already run agentskills.Guard and dropped anything that
// failed it as a Diagnostic, so a guard-failing skill is skipped rather than
// costing the whole list.
func appendEnabledSkills(out []agentskills.Skill, store *agentskills.Store, scope agentskills.Scope, projectID string) []agentskills.Skill {
	skills, _, err := store.List(scope, projectID)
	if err != nil {
		return out
	}
	for _, s := range skills {
		if !s.Enabled {
			continue
		}
		s.Body = "" // never leaves this adapter — see SkillSource's doc comment
		out = append(out, s)
	}
	return out
}
