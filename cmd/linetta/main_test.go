package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunCLIWritesMarkdownByDefault(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runCLI(context.Background(), []string{
		"--goal", "Draft a solarpunk opening",
		"--title", "Harbor of Glass",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runCLI() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "# Harbor of Glass") {
		t.Fatalf("stdout missing markdown title:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCLIWritesJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runCLI(context.Background(), []string{
		"--goal", "Draft a mystery opening",
		"--title", "Signal Rain",
		"--format", "json",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runCLI() error = %v", err)
	}

	var report struct {
		App     string `json:"app"`
		Closure string `json:"closure"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, stdout.String())
	}
	if report.App != "linetta" || report.Closure != "normal" {
		t.Fatalf("report = %+v, want linetta normal", report)
	}
}

func TestRunCLIRejectsMissingGoal(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runCLI(context.Background(), []string{"--title", "No Goal"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("runCLI() error = nil, want non-nil")
	}
}

func TestRunCLIUsesTesseraConfigAndWritesObservabilityFiles(t *testing.T) {
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "runs", "events.jsonl")
	reportPath := filepath.Join(dir, "runs", "report.json")
	htmlPath := filepath.Join(dir, "runs", "report.html")
	configPath := writeConfig(t, dir, `
run:
  id: configured-linetta-run
  workers: 2
  max_attempts: 3
  role_limits:
    researcher: 1
    leader: 1
    writer: 1
    editor: 1
queue:
  type: inmemory
  lease_timeout: 45s
observe:
  events_jsonl: `+eventsPath+`
  report_json: `+reportPath+`
  html_report: `+htmlPath+`
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runCLI(context.Background(), []string{
		"--config", configPath,
		"--goal", "Draft a configured run",
		"--title", "Configured Harbor",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runCLI() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "# Configured Harbor") {
		t.Fatalf("stdout missing markdown title:\n%s", stdout.String())
	}

	events := readFile(t, eventsPath)
	for _, want := range []string{`"run_id":"configured-linetta-run"`, `"type":"task.succeeded"`, `"role":"writer"`} {
		if !strings.Contains(events, want) {
			t.Fatalf("events missing %q:\n%s", want, events)
		}
	}

	report := readFile(t, reportPath)
	for _, want := range []string{`"run_id": "configured-linetta-run"`, `"events_path": "` + eventsPath + `"`, `"html_report_path": "` + htmlPath + `"`} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}

	html := readFile(t, htmlPath)
	for _, want := range []string{"configured-linetta-run", "draft-chapter", "writer", "succeeded"} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q:\n%s", want, html)
		}
	}
}

func TestRunCLIVisualizesExistingEventsFile(t *testing.T) {
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "events.jsonl")
	htmlPath := filepath.Join(dir, "report.html")
	configPath := writeConfig(t, dir, `
observe:
  events_jsonl: `+eventsPath+`
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runCLI(context.Background(), []string{
		"--config", configPath,
		"--goal", "Draft a visualized run",
		"--title", "Signal Garden",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("runCLI() run error = %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := runCLI(context.Background(), []string{"visualize", eventsPath, "--out", htmlPath}, &stdout, &stderr); err != nil {
		t.Fatalf("runCLI() visualize error = %v", err)
	}

	html := readFile(t, htmlPath)
	for _, want := range []string{"linetta-run", "draft-chapter", "writer"} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q:\n%s", want, html)
		}
	}
}

func TestParseServeOptionsUsesCustomValues(t *testing.T) {
	var stderr bytes.Buffer

	opts, err := parseServeOptions([]string{
		"--db", "/tmp/linetta.db",
		"--addr", "127.0.0.1:43191",
	}, &stderr)
	if err != nil {
		t.Fatalf("parseServeOptions() error = %v", err)
	}
	if opts.DBPath != "/tmp/linetta.db" {
		t.Fatalf("DBPath = %q, want custom path", opts.DBPath)
	}
	if opts.Addr != "127.0.0.1:43191" {
		t.Fatalf("Addr = %q, want custom addr", opts.Addr)
	}
}

func TestRunServeMigratesDatabaseAndServesHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan string, 1)
	done := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		done <- runServe(ctx, serveOptions{
			DBPath: filepath.Join(t.TempDir(), "linetta.db"),
			Addr:   "127.0.0.1:0",
			ready:  ready,
		}, &stderr)
	}()

	addr := waitForServeReady(t, ready)
	res, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("GET /health error = %v; stderr=%s", err, stderr.String())
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServe() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runServe() did not stop after context cancellation")
	}
}

func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "tessera.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	return path
}

func waitForServeReady(t *testing.T, ready <-chan string) string {
	t.Helper()
	select {
	case addr := <-ready:
		return addr
	case <-time.After(2 * time.Second):
		t.Fatal("server did not become ready")
		return ""
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
