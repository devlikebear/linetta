package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func digest(b string) string {
	s := sha256.Sum256([]byte(b))
	return hex.EncodeToString(s[:])
}

func serve(t *testing.T, files map[string]string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/"
}

func spec(base string, files map[string]string, wrongDigest bool) ModelSpec {
	s := ModelSpec{Name: "t", Dir: "t", BaseURL: base, ONNXFile: "model.onnx"}
	for _, p := range []string{"config.json", "onnx/model.onnx"} {
		d := digest(files[p])
		if wrongDigest && p == "onnx/model.onnx" {
			d = digest("something else")
		}
		s.Files = append(s.Files, ModelFile{Path: p, Size: int64(len(files[p])), SHA256: d})
	}
	return s
}

func TestDownloadPutsTheCheckedModelInPlace(t *testing.T) {
	files := map[string]string{"config.json": "{}", "onnx/model.onnx": "weights"}
	root := t.TempDir()
	var seen []ModelStatus
	m := NewModels(spec(serve(t, files), files, false), root, func(s ModelStatus) { seen = append(seen, s) })
	if m.Status().Installed {
		t.Fatal("installed before download")
	}
	errc := make(chan error, 1)
	m.Download(func(err error) { errc <- err })
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	st := m.Status()
	if !st.Installed || st.Downloading || st.Received != st.Total {
		t.Fatalf("status = %+v", st)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "t", "model.onnx")); string(b) != "weights" {
		t.Fatalf("model file = %q", b)
	}
	if len(seen) == 0 {
		t.Fatal("no progress reported")
	}
	// A new Models over the same folder finds it installed.
	if !NewModels(m.Spec(), root, nil).Status().Installed {
		t.Fatal("not found installed again")
	}
	if err := m.Delete(); err != nil || m.Status().Installed {
		t.Fatalf("delete: %v %+v", err, m.Status())
	}
	if _, err := os.Stat(filepath.Join(root, "t")); !os.IsNotExist(err) {
		t.Fatal("model folder left behind")
	}
}

func TestDownloadRefusesAFileThatDoesNotMatch(t *testing.T) {
	files := map[string]string{"config.json": "{}", "onnx/model.onnx": "weights"}
	root := t.TempDir()
	m := NewModels(spec(serve(t, files), files, true), root, nil)
	errc := make(chan error, 1)
	m.Download(func(err error) { errc <- err })
	if err := <-errc; err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if st := m.Status(); st.Installed || st.Error == "" {
		t.Fatalf("status = %+v", st)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
}

func TestThePinnedModelAddsUp(t *testing.T) {
	if Model.Total() < 100<<20 || Model.Total() > 200<<20 || !strings.Contains(Model.BaseURL, "/resolve/761b726") {
		t.Fatalf("model spec = %+v", Model)
	}
	for _, f := range Model.Files {
		if len(f.SHA256) != 64 || f.Size <= 0 {
			t.Fatalf("file %+v", f)
		}
	}
}
