// Package paths resolves where Linetta keeps its data on disk.
// All callers should go through this package — never hard-code paths.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AppIdentifier is used as the directory name under the OS app-data location.
const AppIdentifier = "com.devlikebear.linetta"

// Home returns the directory under which Linetta stores its database, settings,
// and backups. Honors LINETTA_HOME if non-empty; otherwise uses the OS default.
func Home() (string, error) {
	if v := os.Getenv("LINETTA_HOME"); v != "" {
		return v, nil
	}
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", AppIdentifier), nil
	case "linux":
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, AppIdentifier), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", AppIdentifier), nil
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, AppIdentifier), nil
		}
		return "", fmt.Errorf("APPDATA unset on Windows")
	default:
		return "", fmt.Errorf("unsupported os: %s", runtime.GOOS)
	}
}

// DBPath returns the absolute path to library.db.
func DBPath() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "library.db"), nil
}

// EnsureHome creates the Home directory if it does not exist (mode 0700).
func EnsureHome() error {
	home, err := Home()
	if err != nil {
		return err
	}
	return os.MkdirAll(home, 0o700)
}
