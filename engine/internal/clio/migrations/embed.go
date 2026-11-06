// Package migrations holds Clio's own schema chain. It is separate from the
// Linetta chain so the Clio database never gets Linetta's tables.
package migrations

import "embed"

// FS holds the NNNN_name.sql files at its root, the layout store.OpenWith expects.
//
//go:embed *.sql
var FS embed.FS
