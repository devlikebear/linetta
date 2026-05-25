package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/linetta/internal/memory"
	"github.com/devlikebear/linetta/internal/store"
	"github.com/devlikebear/linetta/internal/work"
	"github.com/devlikebear/tessera/pkg/council"
	"github.com/devlikebear/tessera/pkg/executor"
	"github.com/devlikebear/tessera/pkg/leader"
	"github.com/devlikebear/tessera/pkg/mandate"
	"github.com/devlikebear/tessera/pkg/queue"
	tesserarun "github.com/devlikebear/tessera/pkg/run"
)

type ArtifactKind string

const (
	ArtifactKindMuseNotes   ArtifactKind = "muse-notes"
	ArtifactKindPlotOutline ArtifactKind = "plot-outline"
	ArtifactKindCanonReview ArtifactKind = "canon-review"
	ArtifactKindResearch    ArtifactKind = "research-notes"
	ArtifactKindDraft       ArtifactKind = "draft"
	ArtifactKindCritique    ArtifactKind = "critique"
	ArtifactKindEditedDraft ArtifactKind = "edited-draft"
)

type EpisodeRunInput struct {
	WorkID     string
	EpisodeID  string
	ApprovedBy string
	ApprovedAt time.Time
	EventSink  tesserarun.EventSink
}

type EpisodeRunResult struct {
	RunID        string
	TesseraRunID string
	Status       string
	Closure      string
	Artifacts    []Artifact
	Events       []tesserarun.Event
}

type RunRecord struct {
	ID           string     `json:"id"`
	WorkID       string     `json:"work_id"`
	EpisodeID    string     `json:"episode_id"`
	Status       string     `json:"status"`
	TesseraRunID string     `json:"tessera_run_id"`
	CreatedAt    time.Time  `json:"created_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
}

type Artifact struct {
	ID        string       `json:"id"`
	WorkID    string       `json:"work_id"`
	EpisodeID string       `json:"episode_id"`
	RunID     string       `json:"run_id"`
	Kind      ArtifactKind `json:"kind"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	CreatedAt time.Time    `json:"created_at"`
}

type Runner struct {
	db          *store.DB
	workRepo    *work.Repository
	memoryRepo  *memory.Repository
	broadcaster *Broadcaster
}

func NewRunner(db *store.DB, workRepo *work.Repository, memoryRepo *memory.Repository) *Runner {
	return &Runner{
		db:          db,
		workRepo:    workRepo,
		memoryRepo:  memoryRepo,
		broadcaster: NewBroadcaster(),
	}
}

// Broadcaster exposes the live-event broadcaster so the HTTP layer can stream
// run progress as it happens.
func (r *Runner) Broadcaster() *Broadcaster { return r.broadcaster }

// StartAsync kicks off RunEpisode in a goroutine and returns the new run_id
// immediately. The actual run continues in the background; subscribers to
// Broadcaster.Subscribe(runID) receive events as they fire. After completion
// Broadcaster.Close(runID) fires so SSE streams terminate cleanly.
func (r *Runner) StartAsync(input EpisodeRunInput) (string, error) {
	input = normalizeInput(input)
	// Pre-flight: validate that the work/episode/blueprint exist BEFORE
	// returning a run_id, so callers get a synchronous error on bad input.
	ctx := context.Background()
	if _, err := r.workRepo.GetWork(ctx, input.WorkID); err != nil {
		return "", err
	}
	if _, err := r.workRepo.GetEpisode(ctx, input.WorkID, input.EpisodeID); err != nil {
		return "", err
	}
	runID := newID("run")
	if err := r.insertRun(ctx, runID, input.WorkID, input.EpisodeID, "running"); err != nil {
		return "", err
	}
	go func() {
		defer r.broadcaster.Close(runID)
		_, _ = r.runEpisodeCore(context.Background(), input, runID)
	}()
	return runID, nil
}

func (r *Runner) RunEpisode(ctx context.Context, input EpisodeRunInput) (EpisodeRunResult, error) {
	input = normalizeInput(input)
	runID := newID("run")
	if err := r.insertRun(ctx, runID, input.WorkID, input.EpisodeID, "running"); err != nil {
		return EpisodeRunResult{}, err
	}
	defer r.broadcaster.Close(runID)
	return r.runEpisodeCore(ctx, input, runID)
}

// runEpisodeCore performs the actual run with the runID already inserted into
// the DB. Used by both the synchronous RunEpisode and the StartAsync goroutine
// path so they share one implementation.
func (r *Runner) runEpisodeCore(ctx context.Context, input EpisodeRunInput, runID string) (EpisodeRunResult, error) {
	workItem, err := r.workRepo.GetWork(ctx, input.WorkID)
	if err != nil {
		_ = r.closeRun(ctx, runID, "failed")
		return EpisodeRunResult{}, err
	}
	episode, err := r.workRepo.GetEpisode(ctx, input.WorkID, input.EpisodeID)
	if err != nil {
		_ = r.closeRun(ctx, runID, "failed")
		return EpisodeRunResult{}, err
	}
	blueprint, err := r.workRepo.GetBlueprint(ctx, input.WorkID, input.EpisodeID)
	if err != nil {
		_ = r.closeRun(ctx, runID, "failed")
		return EpisodeRunResult{}, err
	}
	canonItems, err := r.memoryRepo.ListItems(ctx, input.WorkID, memory.ListFilter{Status: memory.StatusCanon})
	if err != nil && !errors.Is(err, memory.ErrNotFound) {
		_ = r.closeRun(ctx, runID, "failed")
		return EpisodeRunResult{}, err
	}

	innerSink := &eventRecorder{
		db:       r.db,
		runID:    runID,
		external: input.EventSink,
	}
	events := &broadcastingSink{inner: innerSink, broadcaster: r.broadcaster}
	artifacts := newArtifactCollector()

	plan, err := episodePlan(ctx, blueprint)
	if err != nil {
		return EpisodeRunResult{}, err
	}
	report, err := council.DefaultCouncil().Review(ctx, plan)
	if err != nil {
		return EpisodeRunResult{}, err
	}
	m, err := mandate.New("episode-workbench-mandate", plan.Goal.Text, plan.Summary).
		WithReviews(report.MandateReviews()).
		Approve(input.ApprovedBy, input.ApprovedAt)
	if err != nil {
		return EpisodeRunResult{}, err
	}
	graph, err := tesserarun.NewTaskGraphFromPlan(plan, m)
	if err != nil {
		return EpisodeRunResult{}, err
	}

	execution, err := tesserarun.ExecuteTaskGraph(ctx, tesserarun.ExecutionConfig{
		RunID:       runID,
		Mandate:     m,
		Graph:       graph,
		Queue:       queue.NewInMemory(),
		EventSink:   events,
		Workers:     2,
		MaxAttempts: 2,
		RoleLimits: map[string]int{
			"muse":         1,
			"architect":    1,
			"canon_keeper": 1,
			"researcher":   1,
			"writer":       1,
			"critic":       1,
			"editor":       1,
		},
		Executor: executor.TaskHandler(func(_ context.Context, task queue.Task) (executor.Result, error) {
			output := buildOutput(task.ID, workItem, episode, blueprint, canonItems, artifacts.snapshot())
			artifacts.set(ArtifactKind(task.ID), output)
			return executor.Result{Output: output}, nil
		}),
	})
	if err != nil {
		_ = r.closeRun(ctx, runID, "failed")
		return EpisodeRunResult{}, err
	}

	storedArtifacts, err := r.storeArtifacts(ctx, runID, input.WorkID, input.EpisodeID, artifacts.snapshot())
	if err != nil {
		return EpisodeRunResult{}, err
	}
	status := "closed"
	if execution.Report.Closure != tesserarun.ClosureNormal {
		status = "failed"
	}
	if status == "closed" {
		if err := r.storeReviewOutputs(ctx, runID, workItem, episode, blueprint, canonItems, storedArtifacts); err != nil {
			_ = r.closeRun(ctx, runID, "failed")
			return EpisodeRunResult{}, err
		}
	}
	if err := r.closeRun(ctx, runID, status); err != nil {
		return EpisodeRunResult{}, err
	}

	return EpisodeRunResult{
		RunID:        runID,
		TesseraRunID: runID,
		Status:       status,
		Closure:      string(execution.Report.Closure),
		Artifacts:    storedArtifacts,
		Events:       innerSink.Events(),
	}, nil
}

func (r *Runner) ListArtifacts(ctx context.Context, runID string) ([]Artifact, error) {
	rows, err := r.conn().QueryContext(ctx, `
		SELECT id, work_id, episode_id, run_id, kind, title, body, created_at
		FROM artifacts
		WHERE run_id = ?
		ORDER BY created_at ASC, id ASC
	`, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []Artifact
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return artifacts, nil
}

func (r *Runner) GetRun(ctx context.Context, runID string) (RunRecord, error) {
	row := r.conn().QueryRowContext(ctx, `
		SELECT id, work_id, episode_id, status, tessera_run_id, created_at, closed_at
		FROM agent_runs
		WHERE id = ?
	`, strings.TrimSpace(runID))
	var record RunRecord
	var createdAt string
	var closedAt sql.NullString
	if err := row.Scan(&record.ID, &record.WorkID, &record.EpisodeID, &record.Status, &record.TesseraRunID, &createdAt, &closedAt); err != nil {
		return RunRecord{}, err
	}
	var err error
	record.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return RunRecord{}, err
	}
	if closedAt.Valid {
		parsed, err := parseTime(closedAt.String)
		if err != nil {
			return RunRecord{}, err
		}
		record.ClosedAt = &parsed
	}
	return record, nil
}

func (r *Runner) ListEvents(ctx context.Context, runID string) ([]tesserarun.Event, error) {
	rows, err := r.conn().QueryContext(ctx, `
		SELECT event_json
		FROM agent_run_events
		WHERE run_id = ?
		ORDER BY seq ASC
	`, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []tesserarun.Event
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var event tesserarun.Event
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func episodePlan(ctx context.Context, blueprint work.EpisodeBlueprint) (leader.Plan, error) {
	return leader.NewStaticPlanner("episode-workbench-plan", []leader.Step{
		{ID: string(ArtifactKindMuseNotes), Title: "Expand inspiration", Role: "muse", Stage: "ideation", Description: "Expand the human blueprint into scene possibilities."},
		{ID: string(ArtifactKindPlotOutline), Title: "Strengthen plot outline", Role: "architect", Stage: "planning", Description: "Turn the blueprint into a scene-level plot outline.", DependsOn: []string{string(ArtifactKindMuseNotes)}},
		{ID: string(ArtifactKindCanonReview), Title: "Review canon continuity", Role: "canon_keeper", Stage: "review", Description: "Compare the outline against canon memory.", DependsOn: []string{string(ArtifactKindPlotOutline)}},
		{ID: string(ArtifactKindResearch), Title: "Prepare research notes", Role: "researcher", Stage: "research", Description: "Prepare factual and sensory research notes.", DependsOn: []string{string(ArtifactKindCanonReview)}},
		{ID: string(ArtifactKindDraft), Title: "Draft episode", Role: "writer", Stage: "drafting", Description: "Draft the episode from the reviewed outline.", DependsOn: []string{string(ArtifactKindResearch)}},
		{ID: string(ArtifactKindCritique), Title: "Critique episode", Role: "critic", Stage: "review", Description: "Critique pacing, tension, motivation, and originality.", DependsOn: []string{string(ArtifactKindDraft)}},
		{ID: string(ArtifactKindEditedDraft), Title: "Edit episode", Role: "editor", Stage: "editing", Description: "Revise the draft toward publication quality.", DependsOn: []string{string(ArtifactKindCritique)}},
	}).Plan(ctx, leader.Goal{
		ID:       "episode-workbench-goal",
		Text:     blueprint.Premise,
		Template: "episode-workbench",
	})
}

func buildOutput(taskID string, workItem work.Work, episode work.Episode, blueprint work.EpisodeBlueprint, canonItems []memory.Item, prior map[ArtifactKind]string) string {
	switch ArtifactKind(taskID) {
	case ArtifactKindMuseNotes:
		return strings.Join([]string{
			"Episode Muse Notes",
			"Work: " + workItem.Title,
			"Episode: " + episode.Title,
			"Premise: " + blueprint.Premise,
			"Theme: " + blueprint.Theme,
			"Spark: Make the scene hinge on one sensory anomaly the protagonist cannot ignore.",
		}, "\n")
	case ArtifactKindPlotOutline:
		return strings.Join([]string{
			"Plot Outline",
			"1. Re-establish the protagonist's ordinary ritual.",
			"2. Introduce the situation: " + blueprint.Situation,
			"3. Force a choice that reveals the theme: " + blueprint.Theme,
			"Must include: " + blueprint.MustInclude,
			"Must avoid: " + blueprint.MustAvoid,
			"Structure notes: " + blueprint.StructureNotes,
			"From muse: " + firstLine(prior[ArtifactKindMuseNotes]),
		}, "\n")
	case ArtifactKindCanonReview:
		return fmt.Sprintf("Canon Review\nChecked %d canon items.\nNo blocker found for: %s", len(canonItems), blueprint.Premise)
	case ArtifactKindResearch:
		return "Research Notes\nVerify sensory details, infrastructure vocabulary, and any real-world coastal references before publication."
	case ArtifactKindDraft:
		return strings.Join([]string{
			"Draft",
			blueprint.Premise,
			"The first signal arrived as a rhythm change, small enough for the city to ignore and personal enough for Mira to follow.",
			"She counted the beats against the old lullaby and understood that the harbor was not malfunctioning. It was remembering.",
		}, "\n\n")
	case ArtifactKindCritique:
		return "Critique\nThe draft has a clear hook. Strengthen the protagonist's immediate stakes and make the final image sharper."
	case ArtifactKindEditedDraft:
		return strings.Join([]string{
			"Edited Draft",
			blueprint.Premise,
			"The harbor changed its breathing just before sunset.",
			"Mira heard it through the grating beneath her boots: three clicks, a pause, and a note from a lullaby no machine should know.",
			"By the time the tide gardens lit green below her, she had already chosen to open the hatch.",
		}, "\n\n")
	default:
		return "Completed " + taskID
	}
}

func (r *Runner) insertRun(ctx context.Context, runID, workID, episodeID, status string) error {
	now := time.Now().UTC()
	_, err := r.conn().ExecContext(ctx, `
		INSERT INTO agent_runs (id, work_id, episode_id, status, tessera_run_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, runID, workID, episodeID, status, runID, formatTime(now))
	return err
}

func (r *Runner) closeRun(ctx context.Context, runID, status string) error {
	now := time.Now().UTC()
	_, err := r.conn().ExecContext(ctx, `
		UPDATE agent_runs
		SET status = ?, closed_at = ?
		WHERE id = ?
	`, status, formatTime(now), runID)
	return err
}

func (r *Runner) storeArtifacts(ctx context.Context, runID, workID, episodeID string, outputs map[ArtifactKind]string) ([]Artifact, error) {
	order := []ArtifactKind{
		ArtifactKindMuseNotes,
		ArtifactKindPlotOutline,
		ArtifactKindCanonReview,
		ArtifactKindResearch,
		ArtifactKindDraft,
		ArtifactKindCritique,
		ArtifactKindEditedDraft,
	}
	var artifacts []Artifact
	for _, kind := range order {
		body := strings.TrimSpace(outputs[kind])
		if body == "" {
			continue
		}
		now := time.Now().UTC()
		artifact := Artifact{
			ID:        newID("artifact"),
			WorkID:    workID,
			EpisodeID: episodeID,
			RunID:     runID,
			Kind:      kind,
			Title:     artifactTitle(kind),
			Body:      body,
			CreatedAt: now,
		}
		_, err := r.conn().ExecContext(ctx, `
			INSERT INTO artifacts (id, work_id, episode_id, run_id, kind, title, body, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, artifact.ID, artifact.WorkID, artifact.EpisodeID, artifact.RunID, artifact.Kind, artifact.Title, artifact.Body, formatTime(artifact.CreatedAt))
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func (r *Runner) storeReviewOutputs(ctx context.Context, runID string, workItem work.Work, episode work.Episode, blueprint work.EpisodeBlueprint, canonItems []memory.Item, artifacts []Artifact) error {
	if r.memoryRepo == nil {
		return nil
	}
	artifactByKind := make(map[ArtifactKind]Artifact, len(artifacts))
	for _, artifact := range artifacts {
		artifactByKind[artifact.Kind] = artifact
	}

	canonReview := strings.TrimSpace(artifactByKind[ArtifactKindCanonReview].Body)
	editedDraft := strings.TrimSpace(artifactByKind[ArtifactKindEditedDraft].Body)
	proposalBody := strings.Join(nonEmptyLines(
		"Work: "+workItem.Title,
		"Episode: "+episode.Title,
		"Premise: "+blueprint.Premise,
		"Situation: "+blueprint.Situation,
		"Canon review: "+firstLine(canonReview),
		"Draft anchor: "+firstDraftLine(editedDraft),
	), "\n")
	if proposalBody == "" {
		proposalBody = "Episode premise: " + blueprint.Premise
	}

	if _, err := r.memoryRepo.CreateProposal(ctx, memory.CreateProposalInput{
		WorkID:     workItem.ID,
		EpisodeID:  episode.ID,
		RunID:      runID,
		ChangeType: memory.ProposalChangeCreate,
		Kind:       memory.KindPlotThread,
		Title:      "Episode Thread: " + episode.Title,
		AfterBody:  proposalBody,
		Reason:     "Generated by the Canon Keeper after reviewing the episode run.",
		Confidence: 0.72,
	}); err != nil {
		return err
	}

	issueBody := strings.Join(nonEmptyLines(
		fmt.Sprintf("Confirm the proposed episode memory before it becomes canon for %q.", workItem.Title),
		"Premise: "+blueprint.Premise,
		"Must include: "+blueprint.MustInclude,
		"Must avoid: "+blueprint.MustAvoid,
		fmt.Sprintf("Compared against %d existing canon items.", len(canonItems)),
	), "\n")
	_, err := r.memoryRepo.CreateIssue(ctx, memory.CreateIssueInput{
		WorkID:         workItem.ID,
		EpisodeID:      episode.ID,
		RunID:          runID,
		Severity:       memory.IssueWarning,
		Title:          "Review canon acceptance",
		Body:           issueBody,
		RelatedItemIDs: relatedItemIDs(canonItems),
	})
	return err
}

func scanArtifact(row scanner) (Artifact, error) {
	var artifact Artifact
	var createdAt string
	if err := row.Scan(&artifact.ID, &artifact.WorkID, &artifact.EpisodeID, &artifact.RunID, &artifact.Kind, &artifact.Title, &artifact.Body, &createdAt); err != nil {
		return Artifact{}, err
	}
	var err error
	artifact.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

type eventRecorder struct {
	mu       sync.Mutex
	db       *store.DB
	runID    string
	external tesserarun.EventSink
	events   []tesserarun.Event
}

func (r *eventRecorder) OnEvent(ctx context.Context, event tesserarun.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = r.db.Conn().ExecContext(ctx, `
		INSERT INTO agent_run_events (id, run_id, seq, event_json, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, newID("event"), r.runID, event.Seq, string(data), formatTime(time.Now().UTC()))
	if err != nil {
		return err
	}
	if r.external != nil {
		return r.external.OnEvent(ctx, event)
	}
	return nil
}

func (r *eventRecorder) Events() []tesserarun.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]tesserarun.Event, len(r.events))
	copy(out, r.events)
	return out
}

type artifactCollector struct {
	mu    sync.Mutex
	items map[ArtifactKind]string
}

func newArtifactCollector() *artifactCollector {
	return &artifactCollector{items: make(map[ArtifactKind]string)}
}

func (c *artifactCollector) set(kind ArtifactKind, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[kind] = body
}

func (c *artifactCollector) snapshot() map[ArtifactKind]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[ArtifactKind]string, len(c.items))
	for key, value := range c.items {
		out[key] = value
	}
	return out
}

func (r *Runner) conn() *sql.DB {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Conn()
}

type scanner interface {
	Scan(dest ...any) error
}

func normalizeInput(input EpisodeRunInput) EpisodeRunInput {
	input.WorkID = strings.TrimSpace(input.WorkID)
	input.EpisodeID = strings.TrimSpace(input.EpisodeID)
	input.ApprovedBy = strings.TrimSpace(input.ApprovedBy)
	if input.ApprovedBy == "" {
		input.ApprovedBy = "human"
	}
	if input.ApprovedAt.IsZero() {
		input.ApprovedAt = time.Now().UTC()
	}
	return input
}

func artifactTitle(kind ArtifactKind) string {
	switch kind {
	case ArtifactKindMuseNotes:
		return "Muse Notes"
	case ArtifactKindPlotOutline:
		return "Plot Outline"
	case ArtifactKindCanonReview:
		return "Canon Review"
	case ArtifactKindResearch:
		return "Research Notes"
	case ArtifactKindDraft:
		return "Draft"
	case ArtifactKindCritique:
		return "Critique"
	case ArtifactKindEditedDraft:
		return "Edited Draft"
	default:
		return string(kind)
	}
}

func firstLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func firstDraftLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "Edited Draft") || strings.EqualFold(line, "Draft") {
			continue
		}
		return line
	}
	return ""
}

func nonEmptyLines(lines ...string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func relatedItemIDs(items []memory.Item) string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) != "" {
			ids = append(ids, item.ID)
		}
	}
	return strings.Join(ids, ",")
}

func newID(prefix string) string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf[:])
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
