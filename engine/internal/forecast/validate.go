package forecast

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
)

// GlobalIndex marks a Problem that is about the whole submission, not any
// one branch — proposals.GlobalIndex's convention.
const GlobalIndex = -1

// Problem is one thing wrong with a save, phrased so a model reading it
// back can act on it directly — continuity.Problem / proposals.Problem's
// shape. Index is the branch's position in the submitted list (0-based),
// or GlobalIndex.
type Problem struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// refLookup answers whether a ref Validate is checking actually belongs to
// the project a forecast is being saved for — proposals.refLookup's
// convention, applied to the refs a branch's decisions and risks can name.
type refLookup struct {
	ctx       context.Context
	projectID string
	entities  *entity.Repo
	facts     *fact.Repo
	nodes     *node.Repo
}

func (l refLookup) HasEntity(id string) bool {
	if l.entities == nil || id == "" {
		return false
	}
	e, err := l.entities.Get(l.ctx, id)
	return err == nil && e.ProjectID == l.projectID
}

func (l refLookup) HasFact(id string) bool {
	if l.facts == nil || id == "" {
		return false
	}
	f, err := l.facts.Get(l.ctx, id)
	return err == nil && f.ProjectID == l.projectID
}

func (l refLookup) HasNode(id string) bool {
	if l.nodes == nil || id == "" {
		return false
	}
	n, err := l.nodes.Get(l.ctx, id)
	return err == nil && n.ProjectID == l.projectID
}

// Validate checks a save against every rule phase-4-forecast.md's 4.A.2
// lists, collecting every problem rather than stopping at the first —
// continuity.Validate and proposals.Submit's own retry shape: a model
// fixing what it can see should never discover a second round of problems
// that were sitting right next to the first.
func Validate(lookup refLookup, in SaveInput) []Problem {
	var problems []Problem
	add := func(idx int, format string, args ...any) {
		problems = append(problems, Problem{Index: idx, Message: fmt.Sprintf(format, args...)})
	}

	if strings.TrimSpace(in.Divergence) == "" {
		add(GlobalIndex, "divergence is required")
	} else if utf8.RuneCountInString(in.Divergence) > MaxDivergenceRunes {
		add(GlobalIndex, "divergence is over %d characters", MaxDivergenceRunes)
	}

	if in.Horizon < MinHorizon || in.Horizon > MaxHorizon {
		add(GlobalIndex, "horizon must be %d..%d episodes, got %d", MinHorizon, MaxHorizon, in.Horizon)
	}

	n := len(in.Branches)
	if n < MinBranches || n > MaxBranches {
		add(GlobalIndex, "a forecast needs %d..%d branches to compare, got %d", MinBranches, MaxBranches, n)
	}

	for i, b := range in.Branches {
		validateBranch(lookup, add, i, b, in.Horizon)
	}

	return problems
}

func validateBranch(lookup refLookup, add func(int, string, ...any), i int, b BranchInput, horizon int) {
	if strings.TrimSpace(b.Title) == "" {
		add(i, "branch %d: title is required", i+1)
	} else if utf8.RuneCountInString(b.Title) > MaxTitleRunes {
		add(i, "branch %d: title is over %d characters", i+1, MaxTitleRunes)
	}
	if strings.TrimSpace(b.Premise) == "" {
		add(i, "branch %d: premise is required", i+1)
	} else if utf8.RuneCountInString(b.Premise) > MaxPremiseRunes {
		add(i, "branch %d: premise is over %d characters", i+1, MaxPremiseRunes)
	}
	if utf8.RuneCountInString(b.DirectionFit) > MaxDirectionFitRunes {
		add(i, "branch %d: direction_fit is over %d characters", i+1, MaxDirectionFitRunes)
	}

	if len(b.Beats) < MinBeatsPerBranch || len(b.Beats) > MaxBeatsPerBranch {
		add(i, "branch %d: needs %d..%d beats, got %d", i+1, MinBeatsPerBranch, MaxBeatsPerBranch, len(b.Beats))
	}
	for bi, beat := range b.Beats {
		if beat.EpisodeOffset < 1 || beat.EpisodeOffset > horizon {
			add(i, "branch %d beat %d: episode_offset must be 1..%d (the forecast's horizon), got %d",
				i+1, bi+1, horizon, beat.EpisodeOffset)
		}
		if strings.TrimSpace(beat.Summary) == "" {
			add(i, "branch %d beat %d: summary is required", i+1, bi+1)
		} else if utf8.RuneCountInString(beat.Summary) > MaxBeatSummaryRunes {
			add(i, "branch %d beat %d: summary is over %d characters", i+1, bi+1, MaxBeatSummaryRunes)
		}
	}

	if len(b.Decisions) > MaxDecisionsPerBranch {
		add(i, "branch %d: at most %d character decisions, got %d", i+1, MaxDecisionsPerBranch, len(b.Decisions))
	}
	for di, d := range b.Decisions {
		if strings.TrimSpace(d.EntityID) == "" {
			add(i, "branch %d decision %d: entity_id is required", i+1, di+1)
		} else if !lookup.HasEntity(d.EntityID) {
			add(i, "branch %d decision %d: entity_id %q not found in this work", i+1, di+1, d.EntityID)
		}
		if strings.TrimSpace(d.Decision) == "" {
			add(i, "branch %d decision %d: decision is required", i+1, di+1)
		} else if utf8.RuneCountInString(d.Decision) > MaxDecisionRunes {
			add(i, "branch %d decision %d: decision is over %d characters", i+1, di+1, MaxDecisionRunes)
		}
	}

	validateChangeList(add, i, "characters", b.Changes.Characters)
	validateChangeList(add, i, "relationships", b.Changes.Relationships)
	validateChangeList(add, i, "world", b.Changes.World)
	validateChangeList(add, i, "threads", b.Changes.Threads)

	if len(b.Risks) > MaxRisksPerBranch {
		add(i, "branch %d: at most %d risks, got %d", i+1, MaxRisksPerBranch, len(b.Risks))
	}
	for ri, r := range b.Risks {
		if !ValidRiskKind(r.Kind) {
			add(i, "branch %d risk %d: kind %q is not valid; use continuity, causality, character, direction, pacing or other",
				i+1, ri+1, r.Kind)
		}
		if strings.TrimSpace(r.Description) == "" {
			add(i, "branch %d risk %d: description is required", i+1, ri+1)
		} else if utf8.RuneCountInString(r.Description) > MaxRiskDescriptionRunes {
			add(i, "branch %d risk %d: description is over %d characters", i+1, ri+1, MaxRiskDescriptionRunes)
		}
		if r.EntityID != "" && !lookup.HasEntity(r.EntityID) {
			add(i, "branch %d risk %d: entity_id %q not found in this work", i+1, ri+1, r.EntityID)
		}
		if r.FactID != "" && !lookup.HasFact(r.FactID) {
			add(i, "branch %d risk %d: fact_id %q not found in this work", i+1, ri+1, r.FactID)
		}
		if r.NodeID != "" && !lookup.HasNode(r.NodeID) {
			add(i, "branch %d risk %d: node_id %q not found in this work", i+1, ri+1, r.NodeID)
		}
	}

	if len(b.Unknowns) > MaxUnknownsPerBranch {
		add(i, "branch %d: at most %d unknowns, got %d", i+1, MaxUnknownsPerBranch, len(b.Unknowns))
	}
	for ui, u := range b.Unknowns {
		if utf8.RuneCountInString(u) > MaxUnknownRunes {
			add(i, "branch %d unknown %d: over %d characters", i+1, ui+1, MaxUnknownRunes)
		}
	}
}

func validateChangeList(add func(int, string, ...any), branchIdx int, category string, items []string) {
	if len(items) > MaxChangeItemsPerCategory {
		add(branchIdx, "branch %d: changes.%s has %d items, at most %d", branchIdx+1, category, len(items), MaxChangeItemsPerCategory)
	}
	for i, it := range items {
		if utf8.RuneCountInString(it) > MaxChangeItemRunes {
			add(branchIdx, "branch %d: changes.%s[%d] is over %d characters", branchIdx+1, category, i+1, MaxChangeItemRunes)
		}
	}
}
