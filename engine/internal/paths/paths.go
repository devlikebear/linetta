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
//
// Under the macOS App Sandbox (App Store build) os.UserHomeDir resolves to the
// app's container, so the default path stays inside the sandbox. A LINETTA_HOME
// override that points outside the container will fail at write time — it is a
// power-user escape hatch only meaningful for the non-sandboxed build.
func Home() (string, error) {
	return HomeFor(AppIdentifier, "LINETTA_HOME")
}

// HomeFor returns the data directory for the app identified by appID. If
// envVar is non-empty and set in the environment, its value wins; an empty
// envVar means there is no environment override. Otherwise the OS default
// location joined with appID is used (see Home for platform details).
func HomeFor(appID, envVar string) (string, error) {
	if envVar != "" {
		if v := os.Getenv(envVar); v != "" {
			return v, nil
		}
	}
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", appID), nil
	case "linux":
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, appID), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", appID), nil
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, appID), nil
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
