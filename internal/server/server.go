package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/devlikebear/linetta/internal/work"
)

type Options struct{}

type Server struct {
	repo *work.Repository
	mux  *http.ServeMux
}

func New(repo *work.Repository, _ Options) http.Handler {
	s := &Server{
		repo: repo,
		mux:  http.NewServeMux(),
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
