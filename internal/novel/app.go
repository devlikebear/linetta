package novel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tesseraconfig "github.com/devlikebear/tessera/pkg/config"
	"github.com/devlikebear/tessera/pkg/council"
	"github.com/devlikebear/tessera/pkg/executor"
	"github.com/devlikebear/tessera/pkg/leader"
	"github.com/devlikebear/tessera/pkg/mandate"
	"github.com/devlikebear/tessera/pkg/observe"
	"github.com/devlikebear/tessera/pkg/queue"
	"github.com/devlikebear/tessera/pkg/run"
	"github.com/devlikebear/tessera/pkg/visualize"
)

const appName = "linetta"

var ErrBlankGoal = errors.New("novel goal is required")

type Config struct {
	Goal           string
	Title          string
	ApprovedBy     string
	ApprovedAt     time.Time
	RunID          string
	Workers        int
	MaxAttempts    int
	RoleLimits     map[string]int
	LeaseTimeout   time.Duration
	EventsPath     string
	ReportPath     string
	HTMLReportPath string
	EventSink      run.EventSink
}

type Review struct {
	Role    string `json:"role"`
	Verdict string `json:"verdict"`
	Notes   string `json:"notes"`
}

type Report struct {
	App            string            `json:"app"`
	Title          string            `json:"title"`
	Goal           string            `json:"goal"`
	PlanID         string            `json:"plan_id"`
	RunID          string            `json:"run_id"`
	Closure        string            `json:"closure"`
	Succeeded      int               `json:"succeeded"`
	EventCount     int               `json:"event_count"`
	EventsPath     string            `json:"events_path,omitempty"`
	ReportPath     string            `json:"report_path,omitempty"`
	HTMLReportPath string            `json:"html_report_path,omitempty"`
	Reviews        []Review          `json:"reviews"`
	Artifacts      map[string]string `json:"artifacts"`
}

func LoadTesseraConfig(path string) (Config, error) {
	cfg, err := tesseraconfig.LoadFile(path)
	if err != nil {
		return Config{}, err
	}
	return ConfigFromTessera(cfg), nil
}

func ConfigFromTessera(cfg tesseraconfig.Config) Config {
	opts := cfg.RunOptions()
	return Config{
		RunID:          opts.RunID,
		Workers:        opts.Workers,
		MaxAttempts:    opts.MaxAttempts,
		RoleLimits:     opts.RoleLimits,
		LeaseTimeout:   opts.LeaseTimeout,
		EventsPath:     cfg.Observe.EventsJSONL,
		ReportPath:     cfg.Observe.ReportJSON,
		HTMLReportPath: cfg.Observe.HTMLReport,
	}
}

func Run(ctx context.Context, cfg Config) (Report, error) {
	cfg = cfg.withDefaults()
	if cfg.Goal == "" {
		return Report{}, ErrBlankGoal
	}

	plan, err := leader.NewNovelTeamPlanner().Plan(ctx, leader.Goal{
		ID:       "linetta-goal",
		Text:     cfg.Goal,
		Template: "novel-team",
	})
	if err != nil {
		return Report{}, err
	}

	councilReport, err := council.DefaultCouncil().Review(ctx, plan)
	if err != nil {
		return Report{}, err
	}

	m, err := mandate.New("linetta-mandate", plan.Goal.Text, plan.Summary).
		WithReviews(councilReport.MandateReviews()).
		Approve(cfg.ApprovedBy, cfg.ApprovedAt)
	if err != nil {
		return Report{}, err
	}

	graph, err := run.NewTaskGraphFromPlan(plan, m)
	if err != nil {
		return Report{}, err
	}

	artifacts := newArtifactStore()
	eventSink, memorySink, closeSink, err := cfg.buildEventSink()
	if err != nil {
		return Report{}, err
	}
	result, err := run.ExecuteTaskGraph(ctx, run.ExecutionConfig{
		RunID:       cfg.RunID,
		Mandate:     m,
		Graph:       graph,
		Queue:       queue.NewInMemory(queue.WithLeaseTimeout(cfg.LeaseTimeout)),
		EventSink:   eventSink,
		Workers:     cfg.Workers,
		MaxAttempts: cfg.MaxAttempts,
		RoleLimits:  cfg.RoleLimits,
		Executor:    newNovelExecutor(cfg, artifacts),
	})
	if closeErr := closeSink(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return Report{}, err
	}

	report := Report{
		App:            appName,
		Title:          cfg.Title,
		Goal:           cfg.Goal,
		PlanID:         plan.ID,
		RunID:          result.Report.RunID,
		Closure:        string(result.Report.Closure),
		Succeeded:      result.Report.Succeeded,
		EventCount:     len(result.Report.Events),
		EventsPath:     cfg.EventsPath,
		ReportPath:     cfg.ReportPath,
		HTMLReportPath: cfg.HTMLReportPath,
		Reviews:        reviewsFromCouncil(councilReport),
		Artifacts:      artifacts.snapshot(),
	}
	if cfg.HTMLReportPath != "" {
		if err := writeHTMLReportFile(cfg.HTMLReportPath, memorySink.Events()); err != nil {
			return Report{}, err
		}
	}
	if cfg.ReportPath != "" {
		if err := writeJSONFile(cfg.ReportPath, report); err != nil {
			return Report{}, err
		}
	}
	return report, nil
}

func (c Config) withDefaults() Config {
	c.Goal = strings.TrimSpace(c.Goal)
	c.Title = strings.TrimSpace(c.Title)
	c.ApprovedBy = strings.TrimSpace(c.ApprovedBy)
	if c.Title == "" {
		c.Title = "Linetta Novel"
	}
	if c.RunID == "" {
		c.RunID = "linetta-run"
	}
	if c.ApprovedBy == "" {
		c.ApprovedBy = "operator"
	}
	if c.ApprovedAt.IsZero() {
		c.ApprovedAt = time.Now().UTC()
	}
	if c.Workers <= 0 {
		c.Workers = 4
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 2
	}
	if len(c.RoleLimits) == 0 {
		c.RoleLimits = map[string]int{
			"researcher": 1,
			"leader":     1,
			"writer":     1,
			"editor":     1,
		}
	} else {
		c.RoleLimits = cloneRoleLimits(c.RoleLimits)
	}
	if c.LeaseTimeout <= 0 {
		c.LeaseTimeout = 30 * time.Second
	}
	return c
}

func (c Config) buildEventSink() (run.EventSink, *observe.MemorySink, func() error, error) {
	var sinks []observe.Sink
	memorySink := &observe.MemorySink{}
	if c.HTMLReportPath != "" {
		sinks = append(sinks, memorySink)
	}

	var eventsFile *observe.JSONLinesSink
	if c.EventsPath != "" {
		fileSink, err := observe.NewJSONLinesFile(c.EventsPath)
		if err != nil {
			return nil, memorySink, func() error { return nil }, err
		}
		eventsFile = fileSink
		sinks = append(sinks, fileSink)
	}
	if c.EventSink != nil {
		sinks = append(sinks, c.EventSink)
	}

	closeSink := func() error {
		if eventsFile == nil {
			return nil
		}
		return eventsFile.Close()
	}
	switch len(sinks) {
	case 0:
		return nil, memorySink, closeSink, nil
	case 1:
		return sinks[0], memorySink, closeSink, nil
	default:
		return observe.MultiSink(sinks), memorySink, closeSink, nil
	}
}

func reviewsFromCouncil(report council.Report) []Review {
	reviews := make([]Review, len(report.Reviews))
	for i, review := range report.Reviews {
		reviews[i] = Review{
			Role:    string(review.Role),
			Verdict: string(review.Verdict),
			Notes:   review.Notes,
		}
	}
	return reviews
}

func cloneRoleLimits(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func newNovelExecutor(cfg Config, artifacts *artifactStore) executor.TaskExecutor {
	return executor.TaskHandler(func(_ context.Context, task queue.Task) (executor.Result, error) {
		var output string
		switch task.ID {
		case "research-world":
			output = buildWorldNotes(cfg)
		case "outline-plot":
			output = buildOutline(cfg, artifacts.get("research-world"))
		case "draft-chapter":
			output = buildChapterDraft(cfg, artifacts.get("outline-plot"))
		case "review-draft":
			output = buildEditorialReview(cfg, artifacts.get("draft-chapter"))
		default:
			output = fmt.Sprintf("%s completed %s: %s", task.Role, task.ID, task.Payload)
		}
		artifacts.set(task.ID, output)
		return executor.Result{Output: output}, nil
	})
}

func buildWorldNotes(cfg Config) string {
	return strings.Join([]string{
		fmt.Sprintf("Title: %s", cfg.Title),
		fmt.Sprintf("Premise: %s", cfg.Goal),
		"World: A near-future coastal city where public memory, climate recovery, and quiet civic technology shape daily life.",
		"Protagonist: An observant caretaker who notices a small anomaly before anyone else does.",
		"Promise: Keep the first chapter intimate, sensory, and hopeful while leaving one unresolved question.",
	}, "\n")
}

func buildOutline(cfg Config, worldNotes string) string {
	lines := []string{
		fmt.Sprintf("Novel: %s", cfg.Title),
		"Act 1: Establish the city, the protagonist's ordinary ritual, and the first sign that the system is changing.",
		"Act 2: Let the protagonist follow a personal clue into a wider civic mystery.",
		"Act 3: End the opening movement with a choice that protects one person but complicates the larger mission.",
	}
	if worldNotes != "" {
		lines = append(lines, "Source world note: "+firstLine(worldNotes))
	}
	return strings.Join(lines, "\n")
}

func buildChapterDraft(cfg Config, outline string) string {
	opening := fmt.Sprintf("# %s\n\n", cfg.Title)
	body := []string{
		"The harbor lights came on before sunset, one by one, as if the city were counting its breaths.",
		"Every evening, Mira walked the service path above the tide gardens and listened for the pumps below the stone.",
		"Tonight one pump answered in a different rhythm: three soft clicks, a pause, then a tone like a glass rim singing.",
		"She should have logged it and gone home, but the sound matched the lullaby her grandmother used when the black-water years were still a story people told carefully.",
		"By the time the rain began, Mira had opened the maintenance hatch and found a message waiting in the condensation.",
	}
	if outline != "" {
		body = append(body, "", "Working outline signal: "+firstLine(outline))
	}
	return opening + strings.Join(body, "\n\n")
}

func buildEditorialReview(cfg Config, draft string) string {
	notes := []string{
		fmt.Sprintf("Review target: %s", cfg.Title),
		"Strength: The opening has a concrete ritual, a sensory anomaly, and a personal reason to investigate.",
		"Revision: Add one sharper detail about what the city has recovered from.",
		"Next pass: Decide whether the message is a warning, an invitation, or a memory from the old infrastructure.",
	}
	if draft != "" {
		notes = append(notes, fmt.Sprintf("Draft length: %d characters", len([]rune(draft))))
	}
	return strings.Join(notes, "\n")
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

func RenderMarkdown(report Report) string {
	var b strings.Builder
	title := strings.TrimSpace(report.Title)
	if title == "" {
		title = "Linetta Novel"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if report.Goal != "" {
		fmt.Fprintf(&b, "## Goal\n\n%s\n\n", report.Goal)
	}

	sections := []struct {
		Title string
		ID    string
	}{
		{Title: "World Notes", ID: "research-world"},
		{Title: "Plot Outline", ID: "outline-plot"},
		{Title: "Chapter Draft", ID: "draft-chapter"},
		{Title: "Editorial Review", ID: "review-draft"},
	}
	for _, section := range sections {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", section.Title, strings.TrimSpace(report.Artifacts[section.ID]))
	}

	if len(report.Reviews) > 0 {
		fmt.Fprintln(&b, "## Council Reviews")
		fmt.Fprintln(&b)
		for _, review := range report.Reviews {
			fmt.Fprintf(&b, "- %s/%s: %s\n", review.Role, review.Verdict, review.Notes)
		}
		fmt.Fprintln(&b)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := ensureParentDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeHTMLReportFile(path string, events []run.Event) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return visualize.WriteHTMLReport(file, visualize.Project(events))
}

func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

type artifactStore struct {
	mu    sync.Mutex
	items map[string]string
}

func newArtifactStore() *artifactStore {
	return &artifactStore{items: make(map[string]string)}
}

func (s *artifactStore) set(id, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = value
}

func (s *artifactStore) get(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[id]
}

func (s *artifactStore) snapshot() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.items))
	for key := range s.items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make(map[string]string, len(s.items))
	for _, key := range keys {
		out[key] = s.items[key]
	}
	return out
}
