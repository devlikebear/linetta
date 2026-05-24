package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/linetta/internal/novel"
	"github.com/devlikebear/linetta/internal/server"
	"github.com/devlikebear/linetta/internal/store"
	"github.com/devlikebear/linetta/internal/work"
	"github.com/devlikebear/tessera/pkg/visualize"
)

func main() {
	if err := runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "linetta:", err)
		os.Exit(1)
	}
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "visualize" {
		return runVisualize(args[1:])
	}
	if len(args) > 0 && args[0] == "serve" {
		opts, err := parseServeOptions(args[1:], stderr)
		if err != nil {
			return err
		}
		return runServe(ctx, opts, stderr)
	}

	var format string
	var outputPath string
	var configPath string
	var goal string
	var title string
	var approvedBy string

	flags := flag.NewFlagSet("linetta", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&configPath, "config", "", "Tessera YAML or JSON config path")
	flags.StringVar(&goal, "goal", "", "novel-writing goal")
	flags.StringVar(&title, "title", "", "novel title")
	flags.StringVar(&approvedBy, "approved-by", "operator", "mandate approver")
	flags.StringVar(&format, "format", "markdown", "output format: markdown or json")
	flags.StringVar(&outputPath, "output", "", "write output to a file")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg := novel.Config{}
	if configPath != "" {
		loaded, err := novel.LoadTesseraConfig(configPath)
		if err != nil {
			return err
		}
		cfg = loaded
	}
	cfg.Goal = goal
	cfg.Title = title
	cfg.ApprovedBy = approvedBy
	if strings.TrimSpace(cfg.Goal) == "" && flags.NArg() > 0 {
		cfg.Goal = strings.Join(flags.Args(), " ")
	}
	if strings.TrimSpace(cfg.Goal) == "" {
		return errors.New("missing --goal")
	}
	cfg.ApprovedAt = time.Now().UTC()

	report, err := novel.Run(ctx, cfg)
	if err != nil {
		return err
	}

	data, err := encodeReport(report, format)
	if err != nil {
		return err
	}
	if outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0o644); err != nil {
			return err
		}
		return nil
	}
	_, err = stdout.Write(data)
	return err
}

type serveOptions struct {
	DBPath string
	Addr   string
	ready  chan<- string
}

func parseServeOptions(args []string, stderr io.Writer) (serveOptions, error) {
	opts := serveOptions{
		DBPath: defaultDBPath(),
		Addr:   "127.0.0.1:43190",
	}
	flags := flag.NewFlagSet("linetta serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.DBPath, "db", opts.DBPath, "SQLite database path")
	flags.StringVar(&opts.Addr, "addr", opts.Addr, "HTTP listen address")
	if err := flags.Parse(args); err != nil {
		return serveOptions{}, err
	}
	if strings.TrimSpace(opts.DBPath) == "" {
		return serveOptions{}, errors.New("missing --db")
	}
	if strings.TrimSpace(opts.Addr) == "" {
		return serveOptions{}, errors.New("missing --addr")
	}
	return opts, nil
}

func runServe(ctx context.Context, opts serveOptions, stderr io.Writer) error {
	db, err := store.Open(opts.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Handler: server.New(work.NewRepository(db), server.Options{}),
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	addr := listener.Addr().String()
	if opts.ready != nil {
		opts.ready <- addr
	}
	fmt.Fprintf(stderr, "linetta serve listening on http://%s\n", addr)
	err = httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".linetta", "linetta.db")
	}
	return filepath.Join(home, ".linetta", "linetta.db")
}

func runVisualize(args []string) error {
	eventsPath, outPath, err := parseVisualizeArgs(args)
	if err != nil {
		return err
	}
	return visualize.WriteHTMLReportFile(eventsPath, outPath)
}

func parseVisualizeArgs(args []string) (string, string, error) {
	var eventsPath string
	var outPath string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--out":
			i++
			if i >= len(args) {
				return "", "", errors.New("missing value for --out")
			}
			outPath = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", fmt.Errorf("unsupported visualize flag: %s", arg)
			}
			if eventsPath != "" {
				return "", "", fmt.Errorf("unexpected visualize argument: %s", arg)
			}
			eventsPath = arg
		}
	}
	if eventsPath == "" {
		return "", "", errors.New("visualize requires an events JSONL path")
	}
	if outPath == "" {
		return "", "", errors.New("visualize requires --out <report.html>")
	}
	return eventsPath, outPath, nil
}

func encodeReport(report novel.Report, format string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "markdown", "md":
		return []byte(novel.RenderMarkdown(report)), nil
	case "json":
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}
