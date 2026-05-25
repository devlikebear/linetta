// Package library packages and unpacks Linetta library backups (SQLite + Tessera config)
// as portable zip archives. Used by both the CLI (cmd/linetta) and the HTTP server
// (internal/server) so the two share a single implementation.
package library

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExportOptions captures the inputs to Export.
type ExportOptions struct {
	DBPath     string // required: path to the SQLite database to archive
	ConfigPath string // optional: include this Tessera config in the archive
	OutPath    string // required: where to write the zip
}

// ImportOptions captures the inputs to Import.
type ImportOptions struct {
	InPath    string // required: path to the source zip
	DBPath    string // required: where to write the restored database
	ConfigOut string // optional: where to restore the Tessera config (if present in the zip)
	Force     bool   // overwrite DBPath if it already exists
}

// Export writes a zip containing library.db and (optionally) tessera-config.yaml.
// Returns an error if the source DB is missing or the destination cannot be created.
func Export(opts ExportOptions) error {
	if strings.TrimSpace(opts.DBPath) == "" {
		return errors.New("library.Export: missing DBPath")
	}
	if strings.TrimSpace(opts.OutPath) == "" {
		return errors.New("library.Export: missing OutPath")
	}
	if _, err := os.Stat(opts.DBPath); err != nil {
		return fmt.Errorf("library.Export: stat %s: %w", opts.DBPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutPath), 0o755); err != nil {
		return fmt.Errorf("library.Export: mkdir parent of %s: %w", opts.OutPath, err)
	}
	file, err := os.Create(opts.OutPath)
	if err != nil {
		return fmt.Errorf("library.Export: create %s: %w", opts.OutPath, err)
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	if err := addZipFile(archive, "library.db", opts.DBPath); err != nil {
		_ = archive.Close()
		return err
	}
	if strings.TrimSpace(opts.ConfigPath) != "" {
		if err := addZipFile(archive, "tessera-config.yaml", opts.ConfigPath); err != nil {
			_ = archive.Close()
			return err
		}
	}
	return archive.Close()
}

// Import extracts library.db (and optionally tessera-config.yaml) from a zip.
// If opts.Force is false and opts.DBPath already exists, Import refuses to overwrite.
func Import(opts ImportOptions) error {
	if strings.TrimSpace(opts.InPath) == "" {
		return errors.New("library.Import: missing InPath")
	}
	if strings.TrimSpace(opts.DBPath) == "" {
		return errors.New("library.Import: missing DBPath")
	}
	if !opts.Force {
		if _, err := os.Stat(opts.DBPath); err == nil {
			return fmt.Errorf("database already exists at %s; pass force=true to overwrite", opts.DBPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	reader, err := zip.OpenReader(opts.InPath)
	if err != nil {
		return fmt.Errorf("library.Import: open %s: %w", opts.InPath, err)
	}
	defer reader.Close()
	if err := extractZipFile(reader.File, "library.db", opts.DBPath); err != nil {
		return err
	}
	if strings.TrimSpace(opts.ConfigOut) != "" {
		if err := extractZipFile(reader.File, "tessera-config.yaml", opts.ConfigOut); err != nil {
			return err
		}
	}
	return nil
}

func addZipFile(archive *zip.Writer, name, path string) error {
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("library: open %s: %w", path, err)
	}
	defer source.Close()
	target, err := archive.Create(name)
	if err != nil {
		return fmt.Errorf("library: create entry %s: %w", name, err)
	}
	_, err = io.Copy(target, source)
	return err
}

func extractZipFile(files []*zip.File, name, outPath string) error {
	for _, file := range files {
		if file.Name != name {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return fmt.Errorf("library: open entry %s: %w", name, err)
		}
		defer source.Close()
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("library: mkdir parent of %s: %w", outPath, err)
		}
		target, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("library: create %s: %w", outPath, err)
		}
		defer target.Close()
		_, err = io.Copy(target, source)
		return err
	}
	return fmt.Errorf("backup missing %s", name)
}
