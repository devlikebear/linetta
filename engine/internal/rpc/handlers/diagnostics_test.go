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
	staleHome := t.TempDir()
	t.Setenv("LINETTA_HOME", staleHome)
	st, err := store.Open(context.Background(), filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	caps := Capabilities{GitSyncAvailable: true}
	got, err := DiagnosticsVersion(st, home, "test-version", caps)(context.Background(), nil)
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
	if !payload.GitSyncAvailable {
		t.Fatalf("git_sync_available = false, want true")
	}
}

// The legacy-user gate hangs off this flag, so a wrong answer either hides a
// real user's provider settings or shows a newcomer clutter. Consent fields
// cannot answer it — switching provider zeroes them — so the message table is
// the durable evidence.
func TestDiagnosticsReportsCompanionHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", t.TempDir())
	st, err := store.Open(context.Background(), filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	read := func() bool {
		t.Helper()
		got, err := DiagnosticsVersion(st, home, "v", Capabilities{})(context.Background(), nil)
		if err != nil {
			t.Fatalf("DiagnosticsVersion: %v", err)
		}
		var payload diagnosticsPayload
		if err := json.Unmarshal(got, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return payload.CompanionHistoryExists
	}

	if read() {
		t.Fatalf("fresh library reports companion history")
	}

	ctx := context.Background()
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
		 VALUES ('p1', 't', '[]', 'short', 'first', 1, 1)`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO companion_messages (id, project_id, role, content, created_at)
		 VALUES ('m1', 'p1', 'user', 'hello', 1)`); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	if !read() {
		t.Fatalf("library with a companion message reports no history")
	}
}

func TestDiagnosticsGetIncludesOpsStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", t.TempDir())
	st, err := store.Open(context.Background(), filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ops := opsstatus.NewRepo(st)
	if err := ops.Record(context.Background(), opsstatus.JobBackup, 100, 150, true, "", nil); err != nil {
		t.Fatalf("ops.Record: %v", err)
	}

	got, err := DiagnosticsGet(st, ops, home, "test-version", Capabilities{GitSyncAvailable: true})(context.Background(), nil)
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
