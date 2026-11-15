package semantic

import (
	"context"
	"database/sql"
	"slices"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
)

// embedBatch is how many paragraphs go to the model at once.
const embedBatch = 16

// syncDelay lets a burst of saves settle before the worker reads them.
const syncDelay = 2 * time.Second

// Progress is how far a work's paragraphs are embedded.
type Progress struct {
	ProjectID string `json:"project_id"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
}

// Opener loads an Embedder: OpenEmbedder, or a fake in tests.
type Opener func(ctx context.Context, dir, onnxFile string) (Embedder, error)

// Service keeps each work's paragraph vectors up to date in the background
// and answers meaning search.
type Service struct {
	db      *sql.DB
	models  *Models
	enabled func() bool
	notify  func(method string, params any)
	open    Opener
	delay   time.Duration

	mu      sync.Mutex
	emb     Embedder
	pending map[string]bool
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
}

// NewService answers for the model in models while enabled says the writer
// has meaning search on. notify hears "search.semantic_progress".
func NewService(db *sql.DB, models *Models, enabled func() bool, notify func(string, any), open Opener) *Service {
	return &Service{
		db: db, models: models, enabled: enabled, notify: notify, open: open, delay: syncDelay,
		pending: map[string]bool{}, wake: make(chan struct{}, 1),
	}
}

// Ready reports whether meaning search can run: on, and the model in place.
func (s *Service) Ready() bool {
	return s != nil && s.enabled != nil && s.enabled() && s.models.Status().Installed
}

// Start runs the worker until Close.
func (s *Service) Start() {
	s.mu.Lock()
	if s.stop != nil {
		s.mu.Unlock()
		return
	}
	stop, done := make(chan struct{}), make(chan struct{})
	s.stop, s.done = stop, done
	s.mu.Unlock()
	// The worker gets its own channels: Close clears the fields under the
	// lock, and the worker must not read them while it does.
	go s.loop(stop, done)
}

// Close stops the worker and lets go of the model.
func (s *Service) Close() error {
	s.mu.Lock()
	stop, done := s.stop, s.done
	s.stop = nil
	s.mu.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
	return s.release()
}

func (s *Service) release() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emb == nil {
		return nil
	}
	err := s.emb.Close()
	s.emb = nil
	return err
}

// Kick asks for a work's vectors to be brought up to date soon.
func (s *Service) Kick(projectID string) {
	if s == nil || projectID == "" {
		return
	}
	s.mu.Lock()
	s.pending[projectID] = true
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// KickAll asks for every work.
func (s *Service) KickAll(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT project_id FROM manuscript_chunks`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		s.Kick(id)
	}
	return rows.Err()
}

func (s *Service) loop(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-stop:
			return
		case <-s.wake:
		}
		select {
		case <-stop:
			return
		case <-time.After(s.delay):
		}
		s.mu.Lock()
		ids := make([]string, 0, len(s.pending))
		for id := range s.pending {
			ids = append(ids, id)
		}
		s.pending = map[string]bool{}
		s.mu.Unlock()
		slices.Sort(ids)
		for _, id := range ids {
			// A failure leaves the work to the next save or start.
			_ = s.Sync(context.Background(), id)
		}
	}
}

func (s *Service) embedder(ctx context.Context) (Embedder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emb != nil {
		return s.emb, nil
	}
	e, err := s.open(ctx, s.models.Dir(), s.models.Spec().ONNXFile)
	if err != nil {
		return nil, err
	}
	s.emb = e
	return e, nil
}

// Sync embeds the paragraphs of a work that have no vector yet and drops the
// vectors of paragraphs no longer in it. Nothing runs while meaning search is
// off or the model is missing.
func (s *Service) Sync(ctx context.Context, projectID string) error {
	if !s.Ready() {
		return nil
	}
	model := s.models.Spec().Name
	paras, err := paragraphs(ctx, s.db, projectID)
	if err != nil {
		return err
	}
	have, err := vectors(ctx, s.db, projectID, model)
	if err != nil {
		return err
	}
	want := map[string]string{}
	var todo []string
	for _, p := range paras {
		if _, seen := want[p.Hash]; seen {
			continue
		}
		want[p.Hash] = p.Text
		if _, ok := have[p.Hash]; !ok {
			todo = append(todo, p.Hash)
		}
	}
	total := len(want)
	done := total - len(todo)
	if len(todo) > 0 {
		e, err := s.embedder(ctx)
		if err != nil {
			return err
		}
		s.progress(Progress{ProjectID: projectID, Done: done, Total: total})
		for i := 0; i < len(todo); i += embedBatch {
			if !s.Ready() {
				return nil
			}
			batch := todo[i:min(i+embedBatch, len(todo))]
			texts := make([]string, len(batch))
			for j, h := range batch {
				texts[j] = passage(want[h])
			}
			vecs, err := e.Embed(ctx, texts)
			if err != nil {
				return err
			}
			if err := s.store(ctx, projectID, model, batch, vecs); err != nil {
				return err
			}
			done += len(batch)
			s.progress(Progress{ProjectID: projectID, Done: done, Total: total})
		}
	}
	return s.prune(ctx, projectID, model, have, want)
}

func (s *Service) store(ctx context.Context, projectID, model string, hashes []string, vecs [][]float32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, h := range hashes {
		if i >= len(vecs) {
			break
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO paragraph_vectors (project_id, model, text_hash, vec) VALUES (?, ?, ?, ?)`,
			projectID, model, h, quantize(vecs[i])); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) prune(ctx context.Context, projectID, model string, have map[string][]byte, want map[string]string) error {
	for h := range have {
		if _, ok := want[h]; ok {
			continue
		}
		if _, err := s.db.ExecContext(ctx,
			`DELETE FROM paragraph_vectors WHERE project_id = ? AND model = ? AND text_hash = ?`, projectID, model, h); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) progress(p Progress) {
	if s.notify != nil {
		s.notify("search.semantic_progress", map[string]any{"phase": "index", "project_id": p.ProjectID, "done": p.Done, "total": p.Total})
	}
}

// Progress is how far a work is embedded now.
func (s *Service) Progress(ctx context.Context, projectID string) (Progress, error) {
	out := Progress{ProjectID: projectID}
	paras, err := paragraphs(ctx, s.db, projectID)
	if err != nil {
		return out, err
	}
	have, err := vectors(ctx, s.db, projectID, s.models.Spec().Name)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, p := range paras {
		if seen[p.Hash] {
			continue
		}
		seen[p.Hash] = true
		out.Total++
		if _, ok := have[p.Hash]; ok {
			out.Done++
		}
	}
	return out, nil
}

// Search finds the k paragraphs of a work closest in meaning to q. ok is
// false while it cannot answer: off, no model, or nothing embedded yet.
func (s *Service) Search(ctx context.Context, projectID, q string, k int) ([]manuscript.SemanticMatch, bool, error) {
	if !s.Ready() {
		return nil, false, nil
	}
	have, err := vectors(ctx, s.db, projectID, s.models.Spec().Name)
	if err != nil || len(have) == 0 {
		return nil, false, err
	}
	e, err := s.embedder(ctx)
	if err != nil {
		return nil, false, err
	}
	qv, err := e.Embed(ctx, []string{query(q)})
	if err != nil || len(qv) == 0 {
		return nil, false, err
	}
	paras, err := paragraphs(ctx, s.db, projectID)
	if err != nil {
		return nil, false, err
	}
	matches := make([]manuscript.SemanticMatch, 0, len(paras))
	for _, p := range paras {
		v, ok := have[p.Hash]
		if !ok {
			continue
		}
		matches = append(matches, manuscript.SemanticMatch{NodeID: p.NodeID, Para: p.Para, Text: p.Text, Score: dot(qv[0], v)})
	}
	slices.SortStableFunc(matches, func(a, b manuscript.SemanticMatch) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		}
		return 0
	})
	if len(matches) > k {
		matches = matches[:k]
	}
	return matches, true, nil
}

// Forget drops every stored vector and the loaded model: the writer removed
// the model, and vectors of a model that is gone answer nothing.
func (s *Service) Forget(ctx context.Context) error {
	if err := s.release(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM paragraph_vectors`)
	return err
}
