package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/devlikebear/linetta/internal/blueprint"
	"github.com/devlikebear/linetta/internal/memory"
	"github.com/devlikebear/linetta/internal/work"
)

// BlueprintSuggestRequest is the body of POST /api/works/{wid}/episodes/{eid}/blueprint/suggest.
// All fields are optional — the user can submit a wholly empty draft to get a
// fully fresh suggestion (still grounded in Canon memory when available).
type BlueprintSuggestRequest struct {
	Premise        string `json:"premise"`
	Theme          string `json:"theme"`
	Situation      string `json:"situation"`
	MustInclude    string `json:"must_include"`
	MustAvoid      string `json:"must_avoid"`
	StructureNotes string `json:"structure_notes"`
}

// BlueprintSuggestResponse mirrors blueprint.Suggestion. Source is "llm" when
// the suggestion came from a configured OpenAI key, "fallback" otherwise — the
// client can show the user where their suggestion came from.
type BlueprintSuggestResponse struct {
	Premise        string `json:"premise"`
	Theme          string `json:"theme"`
	Situation      string `json:"situation"`
	MustInclude    string `json:"must_include"`
	MustAvoid      string `json:"must_avoid"`
	StructureNotes string `json:"structure_notes"`
	Source         string `json:"source"`
}

func (s *Server) handleBlueprintSuggest(w http.ResponseWriter, r *http.Request, workID, episodeID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req BlueprintSuggestRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workItem, err := s.repo.GetWork(r.Context(), workID)
	if err != nil {
		writeRepositoryError(w, err)
		return
	}
	episode, err := s.repo.GetEpisode(r.Context(), workID, episodeID)
	if err != nil {
		writeRepositoryError(w, err)
		return
	}
	var canon []memory.Item
	if s.memory != nil {
		// Use Canon-status items so suggestions don't reference rejected/archived lore.
		canon, _ = s.memory.ListItems(r.Context(), workID, memory.ListFilter{Status: memory.StatusCanon})
	}
	priorPremises := s.recentEpisodePremises(r.Context(), workID, episodeID, 3) // limit 3

	suggestion, err := blueprint.Suggest(r.Context(), blueprint.Input{
		Work:    workItem,
		Episode: episode,
		Partial: work.EpisodeBlueprint{
			Premise:        req.Premise,
			Theme:          req.Theme,
			Situation:      req.Situation,
			MustInclude:    req.MustInclude,
			MustAvoid:      req.MustAvoid,
			StructureNotes: req.StructureNotes,
		},
		Canon:    canon,
		PriorMSS: priorPremises,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, BlueprintSuggestResponse{
		Premise:        suggestion.Premise,
		Theme:          suggestion.Theme,
		Situation:      suggestion.Situation,
		MustInclude:    suggestion.MustInclude,
		MustAvoid:      suggestion.MustAvoid,
		StructureNotes: suggestion.StructureNotes,
		Source:         suggestion.Source,
	})
}

// recentEpisodePremises pulls premises from the most recently-edited siblings
// for continuity flavor in the prompt. Excludes the current episode.
func (s *Server) recentEpisodePremises(ctx context.Context, workID, currentEpisodeID string, limit int) []string {
	episodes, err := s.repo.ListEpisodes(ctx, workID)
	if err != nil {
		return nil
	}
	var out []string
	for _, ep := range episodes {
		if ep.ID == currentEpisodeID {
			continue
		}
		bp, err := s.repo.GetBlueprint(ctx, workID, ep.ID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(bp.Premise) == "" {
			continue
		}
		out = append(out, bp.Premise)
		if len(out) >= limit {
			break
		}
	}
	return out
}
