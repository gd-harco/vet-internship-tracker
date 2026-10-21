package migrations

import "embed"

// FS exposes embedded goose SQL migration files.
//
//go:embed *.sql
var FS embed.FS
