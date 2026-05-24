package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
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
