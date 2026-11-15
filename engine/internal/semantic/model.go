package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Model is the one model meaning search runs: multilingual-e5-small, int8
// ONNX (MIT licence), pinned to a revision so the bytes are the ones measured
// in the spike. It is not in the installer: a writer who never switches meaning
// search on never downloads it.
var Model = ModelSpec{
	Name:     "multilingual-e5-small-int8",
	Dir:      "multilingual-e5-small",
	BaseURL:  "https://huggingface.co/Xenova/multilingual-e5-small/resolve/761b726dd34fb83930e26aab4e9ac3899aa1fa78/",
	ONNXFile: "model_quantized.onnx",
	Files: []ModelFile{
		{Path: "config.json", Size: 658, SHA256: "cb99455288675345e1a4f411438d5d0adbba5fbd3a67ea4fb03c015433b996c1"},
		{Path: "tokenizer.json", Size: 17082730, SHA256: "0b44a9d7b51c3c62626640cda0e2c2f70fdacdc25bbbd68038369d14ebdf4c39"},
		{Path: "onnx/model_quantized.onnx", Local: "model_quantized.onnx", Size: 118308185, SHA256: "f80102d3f2a1229f387d3c81909990d8945513e347b0eab049f7de3c6f98c193"},
	},
}

// ModelSpec names a model's files and where to fetch them.
type ModelSpec struct {
	// Name identifies the vectors it makes: vectors of another model are
	// not comparable.
	Name, Dir, BaseURL, ONNXFile string
	Files                        []ModelFile
}

// ModelFile is one file of a model: its path under BaseURL, its name in the
// model's folder (Path's base name when empty), its size and its digest.
type ModelFile struct {
	Path, Local, SHA256 string
	Size                int64
}

func (f ModelFile) local() string {
	if f.Local != "" {
		return f.Local
	}
	return filepath.Base(f.Path)
}

// Total is the download's size in bytes.
func (m ModelSpec) Total() int64 {
	var n int64
	for _, f := range m.Files {
		n += f.Size
	}
	return n
}

// ModelStatus is what the settings pane and the search box show.
type ModelStatus struct {
	Installed   bool   `json:"installed"`
	Downloading bool   `json:"downloading"`
	Received    int64  `json:"received"`
	Total       int64  `json:"total"`
	Error       string `json:"error,omitempty"`
}

// Models keeps the model in <home>/models and downloads it on request.
type Models struct {
	spec   ModelSpec
	root   string
	client *http.Client
	notify func(ModelStatus)

	mu     sync.Mutex
	status ModelStatus
	cancel context.CancelFunc
}

// NewModels manages spec under root. notify, if set, hears every change of
// status while a download runs.
func NewModels(spec ModelSpec, root string, notify func(ModelStatus)) *Models {
	m := &Models{spec: spec, root: root, client: &http.Client{Timeout: 30 * time.Minute}, notify: notify}
	m.status = ModelStatus{Installed: m.present(), Total: spec.Total()}
	return m
}

// Dir is where the model's files are once installed.
func (m *Models) Dir() string { return filepath.Join(m.root, m.spec.Dir) }

// Spec is the model managed.
func (m *Models) Spec() ModelSpec { return m.spec }

func (m *Models) present() bool {
	for _, f := range m.spec.Files {
		info, err := os.Stat(filepath.Join(m.Dir(), f.local()))
		if err != nil || info.Size() != f.Size {
			return false
		}
	}
	return true
}

// Status is the model's state now.
func (m *Models) Status() ModelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Models) set(f func(*ModelStatus)) {
	m.mu.Lock()
	f(&m.status)
	s := m.status
	m.mu.Unlock()
	if m.notify != nil {
		m.notify(s)
	}
}

// Download fetches the model in the background unless it is installed or
// already coming; done hears the error, nil when it is in place.
func (m *Models) Download(done func(error)) {
	m.mu.Lock()
	if m.status.Installed || m.status.Downloading {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.status = ModelStatus{Downloading: true, Total: m.spec.Total()}
	m.mu.Unlock()
	go func() {
		defer cancel()
		err := m.fetch(ctx)
		m.set(func(s *ModelStatus) {
			s.Downloading = false
			s.Installed = err == nil
			s.Error = ""
			// A cancelled download is the writer turning it off, not a failure.
			if err != nil && !errors.Is(err, context.Canceled) {
				s.Error = err.Error()
			}
		})
		if done != nil {
			done(err)
		}
	}()
}

// fetch downloads every file into a staging folder, checks each digest, and
// only then moves the folder into place: a half-downloaded model is never
// mistaken for one.
func (m *Models) fetch(ctx context.Context) error {
	staging := m.Dir() + ".partial"
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	var received int64
	for _, f := range m.spec.Files {
		n, err := m.fetchFile(ctx, f, filepath.Join(staging, f.local()), received)
		if err != nil {
			return err
		}
		received += n
	}
	_ = os.RemoveAll(m.Dir())
	return os.Rename(staging, m.Dir())
}

func (m *Models) fetchFile(ctx context.Context, f ModelFile, dest string, before int64) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.spec.BaseURL+f.Path, nil)
	if err != nil {
		return 0, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("semantic: downloading %s: %s", f.Path, resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	sum := sha256.New()
	var n int64
	buf := make([]byte, 256<<10)
	last := time.Time{}
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			if _, err := out.Write(buf[:k]); err != nil {
				return n, err
			}
			sum.Write(buf[:k])
			n += int64(k)
			if n > f.Size {
				return n, fmt.Errorf("semantic: %s is larger than expected", f.Path)
			}
			if time.Since(last) > 200*time.Millisecond {
				last = time.Now()
				got := before + n
				m.set(func(s *ModelStatus) { s.Received = got })
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return n, rerr
		}
	}
	if n != f.Size || hex.EncodeToString(sum.Sum(nil)) != f.SHA256 {
		return n, fmt.Errorf("semantic: %s did not match its checksum", f.Path)
	}
	m.set(func(s *ModelStatus) { s.Received = before + n })
	return n, out.Close()
}

// Delete stops a download and removes the model.
func (m *Models) Delete() error {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	err := os.RemoveAll(m.Dir())
	m.set(func(s *ModelStatus) { *s = ModelStatus{Total: m.spec.Total()} })
	return err
}
