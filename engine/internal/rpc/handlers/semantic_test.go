package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/semantic"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

func semanticDeps(t *testing.T, supported bool) SemanticDeps {
	t.Helper()
	home := t.TempDir()
	st, err := storetest.Open(context.Background(), filepath.Join(home, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg, err := settings.NewForHomeWithSecretStore(home, settings.NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	// An installed model: the files where Models looks, at the spec's sizes.
	spec := semantic.ModelSpec{Name: "fake", Dir: "fake", ONNXFile: "m.onnx", Files: []semantic.ModelFile{{Path: "m.onnx", Size: 2}}}
	_ = os.MkdirAll(filepath.Join(home, "models", "fake"), 0o755)
	_ = os.WriteFile(filepath.Join(home, "models", "fake", "m.onnx"), []byte("ok"), 0o644)
	models := semantic.NewModels(spec, filepath.Join(home, "models"), nil)
	svc := semantic.NewService(st.DB(), models, cfg.SemanticSearchEnabled, nil,
		func(context.Context, string, string) (semantic.Embedder, error) {
			return nil, errors.New("not in this test")
		})
	return SemanticDeps{Service: svc, Models: models, Settings: cfg, Supported: supported}
}

func TestSemanticEnableAndDisable(t *testing.T) {
	ctx := context.Background()
	d := semanticDeps(t, true)
	var st semanticStatus
	res, err := SemanticStatus(d)(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(res, &st)
	if st.Enabled || !st.Supported || !st.Model.Installed {
		t.Fatalf("initial = %+v", st)
	}
	res, err = SemanticEnable(d)(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(res, &st)
	if !st.Enabled || !d.Settings.SemanticSearchEnabled() {
		t.Fatalf("after enable = %+v", st)
	}
	res, err = SemanticDisable(d)(ctx, json.RawMessage(`{"delete_model":true}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(res, &st)
	if st.Enabled || st.Model.Installed {
		t.Fatalf("after disable = %+v", st)
	}
}

func TestSemanticEnableRefusedWhereTheModelCannotRun(t *testing.T) {
	d := semanticDeps(t, false)
	_, err := SemanticEnable(d)(context.Background(), nil)
	var me *rpc.MethodError
	if !errors.As(err, &me) || !strings.Contains(string(me.Data), "semantic_unsupported") {
		t.Fatalf("err = %v", err)
	}
	if d.Settings.SemanticSearchEnabled() {
		t.Fatal("switched on anyway")
	}
}
