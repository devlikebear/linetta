package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/devlikebear/linetta/internal/novel"
)

func main() {
	if err := runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "linetta:", err)
		os.Exit(1)
	}
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var cfg novel.Config
	var format string
	var outputPath string

	flags := flag.NewFlagSet("linetta", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.Goal, "goal", "", "novel-writing goal")
	flags.StringVar(&cfg.Title, "title", "", "novel title")
	flags.StringVar(&cfg.ApprovedBy, "approved-by", "operator", "mandate approver")
	flags.StringVar(&format, "format", "markdown", "output format: markdown or json")
	flags.StringVar(&outputPath, "output", "", "write output to a file")
	if err := flags.Parse(args); err != nil {
		return err
	}

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
