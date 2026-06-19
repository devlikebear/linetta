package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/foldersync"
)

func TestReportFolderSyncSerializes(t *testing.T) {
	s := &foldersync.Syncer{}
	h := ReportFolderSync(s)
	out, err := h(context.Background(), json.RawMessage(`{"ok":true,"files_copied":2}`))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var got map[string]bool
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got["ok"] {
		t.Fatalf("expected ok=true, got %v", got)
	}
}
