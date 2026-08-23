//go:build !mobile

package mcphost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// DiscoveryFileName is the file the stdio bridge reads to find the running
// server. It sits next to library.db and settings.json in $LINETTA_HOME.
//
// Trust boundary: a process running as this user can already read library.db
// and the secret store directly, so carrying the token here does not lower the
// bar — it just spares the writer from pasting it into a bridge config.
const DiscoveryFileName = "mcp.json"

// Discovery is the on-disk contents of the discovery file.
type Discovery struct {
	Port      int    `json:"port"`
	Token     string `json:"token"`
	PID       int    `json:"pid"`
	StartedAt int64  `json:"started_at"`
}

func discoveryPath(home string) string {
	return filepath.Join(home, DiscoveryFileName)
}

// writeDiscoveryFile records the live endpoint at 0600. Written after the
// listener is up so a reader that finds the file can connect.
func writeDiscoveryFile(home string, port int, token string) error {
	if home == "" {
		return nil
	}
	raw, err := json.Marshal(Discovery{
		Port:      port,
		Token:     token,
		PID:       os.Getpid(),
		StartedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(discoveryPath(home), raw, 0o600)
}

// removeDiscoveryFile deletes the file on shutdown so a stale endpoint is
// never advertised. A missing file is not an error.
func removeDiscoveryFile(home string) {
	if home == "" {
		return
	}
	if err := os.Remove(discoveryPath(home)); err != nil && !os.IsNotExist(err) {
		logf("remove discovery file: %v", err)
	}
}

// ReadDiscoveryFile loads the endpoint written by a running app. The bridge
// binary uses this; exported so cmd/linetta-mcp does not duplicate the format.
func ReadDiscoveryFile(home string) (Discovery, error) {
	raw, err := os.ReadFile(discoveryPath(home))
	if err != nil {
		return Discovery{}, err
	}
	var d Discovery
	if err := json.Unmarshal(raw, &d); err != nil {
		return Discovery{}, err
	}
	return d, nil
}
