package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/devlikebear/linetta/internal/memory"
	"github.com/devlikebear/linetta/internal/work"
)

type Options struct {
	Memory *memory.Repository
}

type Server struct {
	repo   *work.Repository
	memory *memory.Repository
	mux    *http.ServeMux
}

func New(repo *work.Repository, opts Options) http.Handler {
	s := &Server{
		repo:   repo,
		memory: opts.Memory,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/api/works", s.handleWorks)
	s.mux.HandleFunc("/api/works/", s.handleWorkPath)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleWorks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		works, err := s.repo.ListWorks(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, works)
	case http.MethodPost:
		var input work.CreateWorkInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := s.repo.CreateWork(r.Context(), input)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, work.ErrInvalidInput) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleWorkPath(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/works/"))
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	workID := parts[0]
	if len(parts) == 1 {
		s.handleWorkDetail(w, r, workID)
		return
	}
	if len(parts) == 2 && parts[1] == "episodes" {
		s.handleEpisodes(w, r, workID)
		return
	}
	if len(parts) == 4 && parts[1] == "episodes" && parts[3] == "blueprint" {
		s.handleEpisodeBlueprint(w, r, workID, parts[2])
		return
	}
	if len(parts) >= 2 && parts[1] == "memory" {
		s.handleMemoryPath(w, r, workID, parts[2:])
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleWorkDetail(w http.ResponseWriter, r *http.Request, workID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	item, err := s.repo.GetWork(r.Context(), workID)
	if err != nil {
		writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleEpisodes(w http.ResponseWriter, r *http.Request, workID string) {
	switch r.Method {
	case http.MethodGet:
		episodes, err := s.repo.ListEpisodes(r.Context(), workID)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, episodes)
	case http.MethodPost:
		var input struct {
			Title string `json:"title"`
		}
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := s.repo.CreateEpisode(r.Context(), workID, input.Title)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleEpisodeBlueprint(w http.ResponseWriter, r *http.Request, workID, episodeID string) {
	switch r.Method {
	case http.MethodGet:
		blueprint, err := s.repo.GetBlueprint(r.Context(), workID, episodeID)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, blueprint)
	case http.MethodPut:
		var input work.SaveBlueprintInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		blueprint, err := s.repo.SaveBlueprint(r.Context(), workID, episodeID, input)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, blueprint)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleMemoryPath(w http.ResponseWriter, r *http.Request, workID string, parts []string) {
	if s.memory == nil {
		writeError(w, http.StatusNotFound, "memory repository not configured")
		return
	}
	if len(parts) == 0 {
		s.handleMemoryCollection(w, r, workID)
		return
	}
	if len(parts) == 1 && parts[0] == "decisions" {
		s.handleMemoryDecisions(w, r, workID)
		return
	}
	if len(parts) == 1 && parts[0] == "search" {
		s.handleMemorySearch(w, r, workID)
		return
	}
	if len(parts) == 1 {
		s.handleMemoryItem(w, r, workID, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "archive" {
		s.handleArchiveMemoryItem(w, r, workID, parts[0])
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleMemoryCollection(w http.ResponseWriter, r *http.Request, workID string) {
	switch r.Method {
	case http.MethodGet:
		filter := memory.ListFilter{
			Kind:   memory.Kind(r.URL.Query().Get("kind")),
			Status: memory.Status(r.URL.Query().Get("status")),
		}
		items, err := s.memory.ListItems(r.Context(), workID, filter)
		if err != nil {
			writeMemoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var input memory.CreateItemInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		input.WorkID = workID
		item, err := s.memory.CreateItem(r.Context(), input)
		if err != nil {
			writeMemoryError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleMemoryItem(w http.ResponseWriter, r *http.Request, workID, itemID string) {
	switch r.Method {
	case http.MethodGet:
		item, err := s.memory.GetItem(r.Context(), itemID)
		if err != nil {
			writeMemoryError(w, err)
			return
		}
		if item.WorkID != workID {
			writeError(w, http.StatusNotFound, memory.ErrNotFound.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodPatch:
		item, err := s.memory.GetItem(r.Context(), itemID)
		if err != nil {
			writeMemoryError(w, err)
			return
		}
		if item.WorkID != workID {
			writeError(w, http.StatusNotFound, memory.ErrNotFound.Error())
			return
		}
		var input memory.UpdateItemInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		updated, err := s.memory.UpdateItem(r.Context(), itemID, input)
		if err != nil {
			writeMemoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleArchiveMemoryItem(w http.ResponseWriter, r *http.Request, workID, itemID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	item, err := s.memory.GetItem(r.Context(), itemID)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	if item.WorkID != workID {
		writeError(w, http.StatusNotFound, memory.ErrNotFound.Error())
		return
	}
	var input struct {
		Reason string `json:"reason"`
		Actor  string `json:"actor"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	archived, err := s.memory.ArchiveItem(r.Context(), itemID, input.Reason, input.Actor)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, archived)
}

func (s *Server) handleMemoryDecisions(w http.ResponseWriter, r *http.Request, workID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	decisions, err := s.memory.ListDecisions(r.Context(), workID)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decisions)
}

func (s *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request, workID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := s.memory.ListItems(r.Context(), workID, memory.ListFilter{Query: r.URL.Query().Get("q")})
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, work.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, work.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeMemoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, memory.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, memory.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func readJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func splitPath(path string) []string {
	var out []string
	for _, part := range strings.Split(path, "/") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
