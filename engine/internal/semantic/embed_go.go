//go:build !mobile

package semantic

import (
	"context"
	"sync"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/backends"
	"github.com/knights-analytics/hugot/pipelines"
)

// goEmbedder runs the model through hugot's pure-Go backend (GoMLX): no cgo
// and no native library, so every desktop build runs the same code.
type goEmbedder struct {
	mu       sync.Mutex
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
}

// OpenEmbedder loads the model in dir.
func OpenEmbedder(ctx context.Context, dir, onnxFile string) (Embedder, error) {
	session, err := hugot.NewGoSession(ctx)
	if err != nil {
		return nil, err
	}
	p, err := hugot.NewPipeline(session, backends.PipelineConfig[*pipelines.FeatureExtractionPipeline]{
		ModelPath: dir, Name: "semantic", OnnxFilename: onnxFile,
		Options: []backends.PipelineOption[*pipelines.FeatureExtractionPipeline]{pipelines.WithNormalization()},
	})
	if err != nil {
		_ = session.Destroy()
		return nil, err
	}
	return &goEmbedder{session: session, pipeline: p}, nil
}

func (e *goEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out, err := e.pipeline.RunPipeline(ctx, texts)
	if err != nil {
		return nil, err
	}
	return out.Embeddings, nil
}

func (e *goEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.session.Destroy()
}

// Supported: this build runs the model.
const Supported = true
