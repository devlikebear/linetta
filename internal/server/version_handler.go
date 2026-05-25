package server

import (
	"net/http"
	"runtime/debug"
	"strings"
)

// VersionResponse is the body of GET /api/version.
type VersionResponse struct {
	Linetta string `json:"linetta"`
	Tessera string `json:"tessera"`
	Go      string `json:"go"`
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, VersionResponse{
		Linetta: linettaVersion(),
		Tessera: tesseraVersion(),
		Go:      goRuntimeVersion(),
	})
}

// linettaVersion returns the build version if available, falling back to "dev".
// In a release build via `go install -ldflags="-X main.version=v1.0"` this
// would be wired through, but for now we use the module's recorded version.
func linettaVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return "dev"
}

// tesseraVersion reads the github.com/devlikebear/tessera version from the
// embedded BuildInfo. Returns empty string if not found (e.g., running with
// a local replace directive).
func tesseraVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if strings.HasSuffix(dep.Path, "/tessera") {
			if dep.Replace != nil && dep.Replace.Version != "" {
				return dep.Replace.Version + " (replaced)"
			}
			return dep.Version
		}
	}
	return ""
}

func goRuntimeVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.GoVersion
	}
	return ""
}
