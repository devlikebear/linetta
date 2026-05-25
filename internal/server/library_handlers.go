package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/devlikebear/linetta/internal/library"
)

// LibraryBackupRequest is the body of POST /api/library/backup.
type LibraryBackupRequest struct {
	OutPath string `json:"out_path"`
}

// LibraryBackupResponse is the response of POST /api/library/backup.
type LibraryBackupResponse struct {
	OutPath   string `json:"out_path"`
	SizeBytes int64  `json:"size_bytes"`
}

// LibraryRestoreRequest is the body of POST /api/library/restore.
// in_path: path to the source zip
// db_out: path to write the restored database (must NOT be the live DB)
// config_out: optional path to extract the snapshotted Tessera config to
// force: overwrite db_out if it already exists
type LibraryRestoreRequest struct {
	InPath    string `json:"in_path"`
	DBOut     string `json:"db_out"`
	ConfigOut string `json:"config_out"`
	Force     bool   `json:"force"`
}

// LibraryRestoreResponse mirrors what was written.
type LibraryRestoreResponse struct {
	DBPath     string `json:"db_path"`
	ConfigPath string `json:"config_path"`
}

// LibraryInfoResponse returns the server's live DB / Tessera config paths so
// the macOS Settings UI can present them without duplicating the @AppStorage
// value (which only reflects what the user typed, not what the engine boot
// actually used).
type LibraryInfoResponse struct {
	DBPath     string `json:"db_path"`
	ConfigPath string `json:"config_path"`
}

func (s *Server) handleLibraryInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, LibraryInfoResponse{
		DBPath:     s.dbPath,
		ConfigPath: s.configPath,
	})
}

func (s *Server) handleLibraryBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if strings.TrimSpace(s.dbPath) == "" {
		writeError(w, http.StatusServiceUnavailable, "server was started without a DB path")
		return
	}
	var req LibraryBackupRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.OutPath) == "" {
		writeError(w, http.StatusBadRequest, "out_path is required")
		return
	}
	err := library.Export(library.ExportOptions{
		DBPath:     s.dbPath,
		ConfigPath: s.configPath,
		OutPath:    req.OutPath,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	info, err := os.Stat(req.OutPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, LibraryBackupResponse{
		OutPath:   req.OutPath,
		SizeBytes: info.Size(),
	})
}

func (s *Server) handleLibraryRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req LibraryRestoreRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.InPath) == "" {
		writeError(w, http.StatusBadRequest, "in_path is required")
		return
	}
	if strings.TrimSpace(req.DBOut) == "" {
		writeError(w, http.StatusBadRequest, "db_out is required")
		return
	}
	// Safety: refuse to overwrite the live DB. Restoring to a staging path is
	// the supported flow; the user must restart the engine pointing at db_out.
	if s.dbPath != "" && req.DBOut == s.dbPath {
		writeError(w, http.StatusConflict, "cannot restore over the live database; pick a different db_out path and restart the engine")
		return
	}
	err := library.Import(library.ImportOptions{
		InPath:    req.InPath,
		DBPath:    req.DBOut,
		ConfigOut: req.ConfigOut,
		Force:     req.Force,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "database already exists") {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, LibraryRestoreResponse{
		DBPath:     req.DBOut,
		ConfigPath: req.ConfigOut,
	})
}
