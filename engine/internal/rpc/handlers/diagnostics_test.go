package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func TestDiagnosticsVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	st, err := store.Open(context.Background(), filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	got, err := DiagnosticsVersion(st, "test-version", nil)(context.Background(), nil)
	if err != nil {
		t.Fatalf("DiagnosticsVersion: %v", err)
	}
	var payload diagnosticsPayload
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Version != "test-version" {
		t.Fatalf("version = %q", payload.Version)
	}
	if payload.Home != home {
		t.Fatalf("home = %q, want %q", payload.Home, home)
	}
	if payload.DBPath != filepath.Join(home, "library.db") {
		t.Fatalf("db_path = %q", payload.DBPath)
	}
	if payload.MigrationVersion == 0 || payload.MigrationCount == 0 {
		t.Fatalf("migration metadata not populated: %+v", payload)
	}
}

func TestDiagnosticsGetIncludesOpsStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	st, err := store.Open(context.Background(), filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ops := opsstatus.NewRepo(st)
	if err := ops.Record(context.Background(), opsstatus.JobBackup, 100, 150, true, "", nil); err != nil {
		t.Fatalf("ops.Record: %v", err)
	}

	got, err := DiagnosticsGet(st, ops, "test-version", nil)(context.Background(), nil)
	if err != nil {
		t.Fatalf("DiagnosticsGet: %v", err)
	}
	var payload diagnosticsGetPayload
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Version != "test-version" {
		t.Fatalf("version = %q", payload.Version)
	}
	if len(payload.OpsStatus) != 1 || payload.OpsStatus[0].JobName != opsstatus.JobBackup {
		t.Fatalf("ops_status = %+v", payload.OpsStatus)
	}
}
