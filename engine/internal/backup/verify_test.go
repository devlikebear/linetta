package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyBackupRejectsInvalidSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.db")
	if err := os.WriteFile(path, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatalf("write invalid database: %v", err)
	}

	if err := verifyBackup(context.Background(), path); err == nil {
		t.Fatal("verifyBackup accepted an invalid SQLite database")
	}
}
