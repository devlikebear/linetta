package store

import "embed"

// MigrationsFS holds the SQL migration files. They are embedded at build time
// so the binary is self-contained.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
